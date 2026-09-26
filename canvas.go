package place

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Canvas holds the pixels and, for each pixel, the id of the player who placed it.
// Owner 0 means the pixel predates accounts ("œuvre d'origine").
type Canvas struct {
	mu      sync.RWMutex
	img     *image.NRGBA
	owners  []uint32
	version uint64
	lastTS  int64
	imgPNG  []byte // cached encodings, nil when stale
	ownPNG  []byte
	now     func() time.Time
}

func NewCanvas(img *image.NRGBA) *Canvas {
	b := img.Bounds()
	if b.Min != (image.Point{}) {
		m := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		copy(m.Pix, img.Pix)
		img = m
	}
	return &Canvas{
		img:    img,
		owners: make([]uint32, b.Dx()*b.Dy()),
		now:    time.Now,
	}
}

// NewBlankCanvas creates a white canvas.
func NewBlankCanvas(w, h int) *Canvas {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	return NewCanvas(img)
}

func (c *Canvas) Size() (w, h int) {
	b := c.img.Bounds()
	return b.Dx(), b.Dy()
}

func (c *Canvas) inBounds(x, y int) bool {
	w, h := c.Size()
	return x >= 0 && y >= 0 && x < w && y < h
}

// Set paints a pixel for a player and returns what it replaced along with the
// time of the change in milliseconds. Timestamps are strictly increasing so that
// events can be replayed in order.
func (c *Canvas) Set(x, y int, col color.NRGBA, uid uint32) (prev color.NRGBA, prevOwner uint32, ts int64, ok bool) {
	if !c.inBounds(x, y) {
		return prev, 0, 0, false
	}
	col.A = 255
	c.mu.Lock()
	defer c.mu.Unlock()
	prev = c.img.NRGBAAt(x, y)
	i := y*c.img.Rect.Dx() + x
	prevOwner = c.owners[i]
	c.img.SetNRGBA(x, y, col)
	c.owners[i] = uid
	c.version++
	c.imgPNG, c.ownPNG = nil, nil
	ts = c.now().UnixMilli()
	if ts <= c.lastTS {
		ts = c.lastTS + 1
	}
	c.lastTS = ts
	return prev, prevOwner, ts, true
}

// At returns the colour and owner of a pixel.
func (c *Canvas) At(x, y int) (color.NRGBA, uint32, bool) {
	if !c.inBounds(x, y) {
		return color.NRGBA{}, 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.img.NRGBAAt(x, y), c.owners[y*c.img.Rect.Dx()+x], true
}

// ImagePNG returns the canvas encoded as PNG, cached until the next change.
func (c *Canvas) ImagePNG() []byte {
	return c.cachedPNG(&c.imgPNG, func() *image.NRGBA {
		return &image.NRGBA{Pix: append([]byte(nil), c.img.Pix...), Stride: c.img.Stride, Rect: c.img.Rect}
	})
}

// OwnersPNG returns the owner map as an RGB PNG where each pixel holds a 24-bit player id.
func (c *Canvas) OwnersPNG() []byte {
	return c.cachedPNG(&c.ownPNG, c.ownersImage)
}

func (c *Canvas) ownersImage() *image.NRGBA {
	m := image.NewNRGBA(c.img.Rect)
	for i, id := range c.owners {
		m.Pix[i*4] = uint8(id >> 16)
		m.Pix[i*4+1] = uint8(id >> 8)
		m.Pix[i*4+2] = uint8(id)
		m.Pix[i*4+3] = 255
	}
	return m
}

// cachedPNG copies the data under the read lock, then encodes it without blocking writers.
func (c *Canvas) cachedPNG(cache *[]byte, snapshot func() *image.NRGBA) []byte {
	c.mu.RLock()
	if b := *cache; b != nil {
		c.mu.RUnlock()
		return b
	}
	ver := c.version
	m := snapshot()
	c.mu.RUnlock()

	b := encodePNG(m)
	c.mu.Lock()
	if c.version == ver {
		*cache = b
	}
	c.mu.Unlock()
	return b
}

func encodePNG(m image.Image) []byte {
	buf := bytes.NewBuffer(nil)
	if err := png.Encode(buf, m); err != nil {
		panic(err) // encoding an in-memory NRGBA image cannot fail
	}
	return buf.Bytes()
}

// ------------------------------------------------------------------
// owners.bin: "BPOW", version, width, height, saved-at (ms), image CRC32, then
// width*height little-endian uint32 owners.

var ownersMagic = [4]byte{'B', 'P', 'O', 'W'}

const ownersVersion = 1

type ownersHeader struct {
	Magic   [4]byte
	Version uint32
	Width   uint32
	Height  uint32
	SavedAt int64
	ImgCRC  uint32
}

// OwnersSnapshot describes the owners file that was loaded.
type OwnersSnapshot struct {
	SavedAt int64
	// Matches is true when the loaded image is exactly the one the owners were saved with.
	Matches bool
}

var ErrBadOwners = errors.New("owners file: bad header")

func (c *Canvas) imageCRC() uint32 {
	return crc32.ChecksumIEEE(c.img.Pix)
}

func writeOwners(w io.Writer, img *image.NRGBA, owners []uint32, savedAt int64) error {
	b := img.Bounds()
	h := ownersHeader{ownersMagic, ownersVersion, uint32(b.Dx()), uint32(b.Dy()), savedAt, crc32.ChecksumIEEE(img.Pix)}
	bw := bufio.NewWriter(w)
	if err := binary.Write(bw, binary.LittleEndian, h); err != nil {
		return err
	}
	if err := binary.Write(bw, binary.LittleEndian, owners); err != nil {
		return err
	}
	return bw.Flush()
}

// ReadOwners loads an owners file. If the saved canvas was smaller (the canvas grew),
// owners are copied to the top-left corner.
func (c *Canvas) ReadOwners(r io.Reader) (OwnersSnapshot, error) {
	br := bufio.NewReader(r)
	var h ownersHeader
	if err := binary.Read(br, binary.LittleEndian, &h); err != nil {
		return OwnersSnapshot{}, err
	}
	if h.Magic != ownersMagic || h.Version != ownersVersion || h.Width == 0 || h.Height == 0 || h.Width > 1<<14 || h.Height > 1<<14 {
		return OwnersSnapshot{}, ErrBadOwners
	}
	saved := make([]uint32, int(h.Width)*int(h.Height))
	if err := binary.Read(br, binary.LittleEndian, saved); err != nil {
		return OwnersSnapshot{}, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	w, hh := c.img.Rect.Dx(), c.img.Rect.Dy()
	for i := range c.owners {
		c.owners[i] = 0
	}
	for y := 0; y < hh && y < int(h.Height); y++ {
		for x := 0; x < w && x < int(h.Width); x++ {
			c.owners[y*w+x] = saved[y*int(h.Width)+x]
		}
	}
	c.version++
	c.ownPNG = nil
	matches := int(h.Width) == w && int(h.Height) == hh && h.ImgCRC == c.imageCRC()
	return OwnersSnapshot{SavedAt: h.SavedAt, Matches: matches}, nil
}

// SaveFiles writes the image and the owners from the same snapshot.
func (c *Canvas) SaveFiles(pngPath, ownersPath string) error {
	c.mu.RLock()
	img := &image.NRGBA{Pix: append([]byte(nil), c.img.Pix...), Stride: c.img.Stride, Rect: c.img.Rect}
	owners := append([]uint32(nil), c.owners...)
	savedAt := c.now().UnixMilli()
	if savedAt < c.lastTS {
		savedAt = c.lastTS
	}
	c.mu.RUnlock()

	if err := writeFileAtomic(pngPath, encodePNG(img)); err != nil {
		return err
	}
	if ownersPath == "" {
		return nil
	}
	buf := bytes.NewBuffer(make([]byte, 0, 24+len(owners)*4))
	if err := writeOwners(buf, img, owners, savedAt); err != nil {
		return err
	}
	return writeFileAtomic(ownersPath, buf.Bytes())
}

// writeFileAtomic writes to a temporary file then renames it, so backup.sh never
// copies a half-written place.png. If the rename fails (e.g. the target is a
// bind-mounted file), it falls back to writing in place.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp*")
	if err == nil {
		_, err = tmp.Write(data)
		if cerr := tmp.Close(); err == nil {
			err = cerr
		}
		if err == nil {
			if err = os.Rename(tmp.Name(), path); err == nil {
				return nil
			}
		}
		os.Remove(tmp.Name())
	}
	return os.WriteFile(path, data, 0644)
}

// ------------------------------------------------------------------
// Restoring owners at startup.

// apply sets a pixel from a replayed event without touching timestamps.
func (c *Canvas) apply(x, y int, rgb uint32, uid uint32) {
	if !c.inBounds(x, y) {
		return
	}
	c.img.SetNRGBA(x, y, rgbToNRGBA(rgb))
	c.owners[y*c.img.Rect.Dx()+x] = uid
}

// RestoreOwners loads the owners file and reconciles it with the pixel journal:
//   - owners file saved with this very image: events saved after it are replayed
//     (recovers pixels placed between the last save and a crash);
//   - otherwise (another image was loaded, or no owners file): each pixel keeps the
//     author of its last event only if the colour still matches.
//
// It then recomputes every player's count of visible pixels.
func RestoreOwners(c *Canvas, st *Store, ownersPath string) (replayed int, err error) {
	snap := OwnersSnapshot{}
	loaded := false
	if ownersPath != "" {
		if f, ferr := os.Open(ownersPath); ferr == nil {
			snap, err = c.ReadOwners(f)
			f.Close()
			if err != nil {
				return 0, err
			}
			loaded = true
		} else if !errors.Is(ferr, os.ErrNotExist) {
			return 0, ferr
		}
	}

	c.mu.Lock()
	defer func() {
		c.version++
		c.imgPNG, c.ownPNG = nil, nil
		c.mu.Unlock()
	}()

	if loaded && snap.Matches {
		err = st.EachPixelEvent(snap.SavedAt, func(e PixelEvent) error {
			c.apply(e.X, e.Y, e.Color, e.UserID)
			if e.TS > c.lastTS {
				c.lastTS = e.TS
			}
			replayed++
			return nil
		})
	} else {
		w := c.img.Rect.Dx()
		for i := range c.owners {
			c.owners[i] = 0
		}
		err = st.EachPixelEvent(0, func(e PixelEvent) error {
			if !c.inBounds(e.X, e.Y) {
				return nil
			}
			if nrgbaToRGB(c.img.NRGBAAt(e.X, e.Y)) == e.Color {
				c.owners[e.Y*w+e.X] = e.UserID
			} else {
				c.owners[e.Y*w+e.X] = 0
			}
			if e.TS > c.lastTS {
				c.lastTS = e.TS
			}
			return nil
		})
	}
	if err != nil {
		return replayed, err
	}

	counts := map[uint32]int64{}
	for _, id := range c.owners {
		if id != 0 {
			counts[id]++
		}
	}
	return replayed, st.SetVisibleCounts(counts)
}

func rgbToNRGBA(rgb uint32) color.NRGBA {
	return color.NRGBA{R: uint8(rgb >> 16), G: uint8(rgb >> 8), B: uint8(rgb), A: 255}
}

func nrgbaToRGB(c color.NRGBA) uint32 {
	return uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
}
