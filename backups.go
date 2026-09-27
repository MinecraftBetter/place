package place

// Backups: every capture of the canvas made by backup.sh (bak/AAAA-MM/AAAA-MM-JJ_HH-MM-SS.png,
// one per minute when the canvas changed) or zipped by timelapse.sh (archives/archive-AAAA-MM.zip).
// The index replays them all and keeps every pixel change: it is the only history of the
// canvas before accounts, used to analyse claims, build statistics and the timelapse.
// Port of design/outils/analyse_sauvegardes.py (indexer).

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

const white = 0xFFFFFF

var backupNameRE = regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})_(\d{2})-(\d{2})-(\d{2})\.png$`)

// Frame is one capture. T is the capture time (Unix seconds); file names are in UTC.
type Frame struct {
	T     int64
	W, H  int
	Src   string // directory file or zip archive
	Entry string // name inside the zip, "" for a plain file
	Size  int64
}

// BackupIndex holds every pixel change between consecutive captures, normalised to
// the latest canvas size (smaller captures sit in the top-left corner, padded white).
type BackupIndex struct {
	Version int
	W, H    int
	Frames  []Frame
	First   []uint32 // first capture, 0xRRGGBB
	Last    []uint32 // latest capture
	EvFrame []int32  // change i happened in frame EvFrame[i] (non-decreasing)
	EvPos   []int32  // at pixel y*W+x
	EvCol   []uint32 // new colour
	Skipped int      // unreadable captures (empty files…)
}

const backupIndexVersion = 1

// frameTime parses "AAAA-MM-JJ_HH-MM-SS.png" (UTC).
func frameTime(name string) (int64, bool) {
	m := backupNameRE.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	t, err := time.Parse("2006-01-02 15-04-05", fmt.Sprintf("%s-%s-%s %s-%s-%s", m[1], m[2], m[3], m[4], m[5], m[6]))
	if err != nil {
		return 0, false
	}
	return t.Unix(), true
}

// ListFrames finds captures in directories (recursively), zip archives and PNG files.
// When the same instant appears twice (bak/ and a zip), the first source wins.
func ListFrames(sources []string) ([]Frame, error) {
	var frames []Frame
	seen := map[int64]bool{}
	add := func(f Frame) {
		if !seen[f.T] {
			seen[f.T] = true
			frames = append(frames, f)
		}
	}
	for _, src := range sources {
		st, err := os.Stat(src)
		if err != nil {
			continue
		}
		switch {
		case st.IsDir():
			filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				if info.IsDir() {
					if info.Name() == "corrupted" {
						return filepath.SkipDir
					}
					return nil
				}
				if strings.HasSuffix(path, ".zip") {
					if zf, err := zipFrames(path); err == nil {
						for _, f := range zf {
							add(f)
						}
					}
					return nil
				}
				if t, ok := frameTime(info.Name()); ok {
					add(Frame{T: t, Src: path, Size: info.Size()})
				}
				return nil
			})
		case strings.HasSuffix(src, ".zip"):
			zf, err := zipFrames(src)
			if err != nil {
				return nil, err
			}
			for _, f := range zf {
				add(f)
			}
		default:
			if t, ok := frameTime(filepath.Base(src)); ok {
				add(Frame{T: t, Src: src, Size: st.Size()})
			}
		}
	}
	sort.Slice(frames, func(i, j int) bool { return frames[i].T < frames[j].T })
	return frames, nil
}

func zipFrames(path string) ([]Frame, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	var out []Frame
	for _, f := range z.File {
		if t, ok := frameTime(filepath.Base(f.Name)); ok {
			out = append(out, Frame{T: t, Src: path, Entry: f.Name, Size: int64(f.UncompressedSize64)})
		}
	}
	return out, nil
}

// zipCache keeps archives open: a timelapse reads many entries of the same zip.
type openZip struct {
	rc    *zip.ReadCloser
	files map[string]*zip.File
}

var zipCache = struct {
	sync.Mutex
	m map[string]*openZip
}{m: map[string]*openZip{}}

// ReadFrame returns the PNG bytes of a capture.
func ReadFrame(f Frame) ([]byte, error) {
	if f.Entry == "" {
		return os.ReadFile(f.Src)
	}
	zipCache.Lock()
	z, ok := zipCache.m[f.Src]
	if !ok {
		rc, err := zip.OpenReader(f.Src)
		if err != nil {
			zipCache.Unlock()
			return nil, err
		}
		z = &openZip{rc: rc, files: map[string]*zip.File{}}
		for _, zf := range rc.File {
			z.files[zf.Name] = zf
		}
		zipCache.m[f.Src] = z
	}
	zipCache.Unlock()
	zf, ok := z.files[f.Entry]
	if !ok {
		return nil, os.ErrNotExist
	}
	r, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// decodeFrame decodes a capture to 0xRRGGBB values, padded to w×h (0 = native size).
func decodeFrame(b []byte, w, h int) ([]uint32, int, int, error) {
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, 0, 0, err
	}
	bd := img.Bounds()
	fw, fh := bd.Dx(), bd.Dy()
	if w == 0 {
		w, h = fw, fh
	}
	out := make([]uint32, w*h)
	for i := range out {
		out[i] = white
	}
	switch m := img.(type) {
	case *image.NRGBA:
		for y := 0; y < fh && y < h; y++ {
			row := m.Pix[y*m.Stride:]
			for x := 0; x < fw && x < w; x++ {
				out[y*w+x] = uint32(row[x*4])<<16 | uint32(row[x*4+1])<<8 | uint32(row[x*4+2])
			}
		}
	case *image.RGBA:
		for y := 0; y < fh && y < h; y++ {
			row := m.Pix[y*m.Stride:]
			for x := 0; x < fw && x < w; x++ {
				out[y*w+x] = uint32(row[x*4])<<16 | uint32(row[x*4+1])<<8 | uint32(row[x*4+2])
			}
		}
	default:
		for y := 0; y < fh && y < h; y++ {
			for x := 0; x < fw && x < w; x++ {
				r, g, b, _ := img.At(bd.Min.X+x, bd.Min.Y+y).RGBA()
				out[y*w+x] = (r>>8)<<16 | (g>>8)<<8 | b>>8
			}
		}
	}
	return out, fw, fh, nil
}

// BuildIndex replays the captures. With a previous index of the same size, only the new
// captures are decoded. progress (optional) is called every 100 frames.
func BuildIndex(frames []Frame, prev *BackupIndex, progress func(done, total int)) (*BackupIndex, error) {
	if len(frames) == 0 {
		return nil, errors.New("aucune sauvegarde trouvée")
	}
	// Size of the canvas: the latest readable capture.
	var w, h int
	for i := len(frames) - 1; i >= 0 && w == 0; i-- {
		b, err := ReadFrame(frames[i])
		if err != nil || len(b) == 0 {
			continue
		}
		if cfg, err := png.DecodeConfig(bytes.NewReader(b)); err == nil {
			w, h = cfg.Width, cfg.Height
		}
	}
	if w == 0 {
		return nil, errors.New("aucune sauvegarde lisible")
	}

	idx := &BackupIndex{Version: backupIndexVersion, W: w, H: h}
	start := 0
	var cur []uint32
	if prev != nil && prev.Version == backupIndexVersion && prev.W == w && prev.H == h && len(prev.Frames) > 0 {
		last := prev.Frames[len(prev.Frames)-1].T
		for start < len(frames) && frames[start].T <= last {
			start++
		}
		*idx = *prev
		cur = append([]uint32(nil), prev.Last...)
	}
	for i := start; i < len(frames); i++ {
		if progress != nil && i%100 == 0 {
			progress(i, len(frames))
		}
		f := frames[i]
		b, err := ReadFrame(f)
		if err != nil || len(b) == 0 {
			idx.Skipped++
			continue
		}
		px, fw, fh, err := decodeFrame(b, w, h)
		if err != nil {
			idx.Skipped++
			continue
		}
		f.W, f.H = fw, fh
		k := int32(len(idx.Frames))
		idx.Frames = append(idx.Frames, f)
		if cur == nil {
			idx.First = px
			cur = append([]uint32(nil), px...)
			continue
		}
		for p, c := range px {
			if cur[p] != c {
				idx.EvFrame = append(idx.EvFrame, k)
				idx.EvPos = append(idx.EvPos, int32(p))
				idx.EvCol = append(idx.EvCol, c)
				cur[p] = c
			}
		}
	}
	if progress != nil {
		progress(len(frames), len(frames))
	}
	if len(idx.Frames) == 0 {
		return nil, errors.New("aucune sauvegarde lisible")
	}
	idx.Last = cur
	return idx, nil
}

func (idx *BackupIndex) Save(path string) error {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if err := gob.NewEncoder(gz).Encode(idx); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return writeFileAtomic(path, buf.Bytes())
}

func LoadBackupIndex(path string) (*BackupIndex, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	var idx BackupIndex
	if err := gob.NewDecoder(gz).Decode(&idx); err != nil {
		return nil, err
	}
	if idx.Version != backupIndexVersion {
		return nil, errors.New("index d'une autre version")
	}
	return &idx, nil
}

// FrameAt returns the index of the last frame at or before t (Unix seconds), -1 if none.
func (idx *BackupIndex) FrameAt(t int64) int {
	i := sort.Search(len(idx.Frames), func(i int) bool { return idx.Frames[i].T > t })
	return i - 1
}

// FrameTime returns the time of frame k.
func (idx *BackupIndex) FrameTime(k int) time.Time {
	return time.Unix(idx.Frames[k].T, 0)
}

// ------------------------------------------------------------------
// Backups service: loads or builds the index in the background and keeps it up to date.

type Backups struct {
	sources   []string
	indexPath string

	mu       sync.RWMutex
	idx      *BackupIndex
	status   string // "absent" | "indexing" | "ready" | "error"
	progress [2]int
	err      error
	builtAt  time.Time
}

func NewBackups(sources []string, indexPath string) *Backups {
	return &Backups{sources: sources, indexPath: indexPath, status: "absent"}
}

// Index returns the current index, or nil while it is being built.
func (b *Backups) Index() *BackupIndex {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.idx
}

type BackupStatus struct {
	Status  string `json:"status"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Frames  int    `json:"frames"`
	Changes int    `json:"changes"`
	Skipped int    `json:"skipped"`
	First   int64  `json:"first,omitempty"`
	Last    int64  `json:"last,omitempty"`
	Error   string `json:"error,omitempty"`
	BuiltAt int64  `json:"built_at,omitempty"`
	Sources int    `json:"sources"`
	W, H    int
}

func (b *Backups) Status() BackupStatus {
	b.mu.RLock()
	defer b.mu.RUnlock()
	s := BackupStatus{Status: b.status, Done: b.progress[0], Total: b.progress[1], Sources: len(b.sources)}
	if b.err != nil {
		s.Error = b.err.Error()
	}
	if b.idx != nil {
		s.Frames, s.Changes, s.Skipped = len(b.idx.Frames), len(b.idx.EvPos), b.idx.Skipped
		s.First, s.Last = b.idx.Frames[0].T*1000, b.idx.Frames[len(b.idx.Frames)-1].T*1000
		s.W, s.H = b.idx.W, b.idx.H
	}
	if !b.builtAt.IsZero() {
		s.BuiltAt = b.builtAt.UnixMilli()
	}
	return s
}

// Refresh loads the saved index, then adds the captures made since. Safe to call often.
func (b *Backups) Refresh() error {
	b.mu.Lock()
	if b.status == "indexing" {
		b.mu.Unlock()
		return nil
	}
	prev := b.idx
	b.mu.Unlock()

	if prev == nil && b.indexPath != "" {
		if loaded, err := LoadBackupIndex(b.indexPath); err == nil {
			prev = loaded
			b.mu.Lock()
			b.idx, b.status = loaded, "ready"
			b.mu.Unlock()
		}
	}
	frames, err := ListFrames(b.sources)
	if err != nil || len(frames) == 0 {
		b.mu.Lock()
		if b.idx == nil {
			b.status = "absent"
			b.err = err
		}
		b.mu.Unlock()
		return err
	}
	if prev != nil && len(prev.Frames) > 0 && frames[len(frames)-1].T <= prev.Frames[len(prev.Frames)-1].T {
		return nil // nothing new
	}
	b.mu.Lock()
	if b.idx == nil {
		b.status = "indexing"
	}
	b.mu.Unlock()

	started := time.Now()
	idx, err := BuildIndex(frames, prev, func(done, total int) {
		b.mu.Lock()
		b.progress = [2]int{done, total}
		b.mu.Unlock()
	})
	b.mu.Lock()
	defer b.mu.Unlock()
	if err != nil {
		b.err = err
		if b.idx == nil {
			b.status = "error"
		}
		return err
	}
	b.idx, b.status, b.err, b.builtAt = idx, "ready", nil, time.Now()
	log.WithField("endpoint", "Backups").Infof("Index: %d captures, %d pixel changes (%s)", len(idx.Frames), len(idx.EvPos), time.Since(started).Round(time.Second))
	if b.indexPath != "" {
		if err := idx.Save(b.indexPath); err != nil {
			log.WithField("endpoint", "Backups").Error("Saving index: ", err)
		}
	}
	return nil
}

// Run refreshes the index now and then every interval.
func (b *Backups) Run(interval time.Duration) {
	for {
		if err := b.Refresh(); err != nil {
			log.WithField("endpoint", "Backups").Warning("Index: ", err)
		}
		time.Sleep(interval)
	}
}
