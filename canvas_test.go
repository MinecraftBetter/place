package place

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var blue = color.NRGBA{0x5e, 0xb3, 0xff, 255}

func TestCanvasSetAndAt(t *testing.T) {
	c := NewBlankCanvas(4, 3)
	prev, prevOwner, ts, ok := c.Set(1, 2, color.NRGBA{0x5e, 0xb3, 0xff, 0}, 7)
	if !ok || prevOwner != 0 || prev != (color.NRGBA{255, 255, 255, 255}) || ts == 0 {
		t.Fatalf("Set = %v %v %v %v", prev, prevOwner, ts, ok)
	}
	col, owner, ok := c.At(1, 2)
	if !ok || owner != 7 || col != blue {
		t.Fatalf("At = %v %v %v (alpha must be forced to 255)", col, owner, ok)
	}
	_, prevOwner, ts2, _ := c.Set(1, 2, blue, 9)
	if prevOwner != 7 || ts2 <= ts {
		t.Fatalf("second Set: prevOwner %d, ts %d after %d", prevOwner, ts2, ts)
	}
}

func TestCanvasOutOfBounds(t *testing.T) {
	c := NewBlankCanvas(4, 3)
	for _, p := range [][2]int{{-1, 0}, {0, -1}, {4, 0}, {0, 3}} {
		if _, _, _, ok := c.Set(p[0], p[1], blue, 1); ok {
			t.Errorf("Set(%v) accepted", p)
		}
		if _, _, ok := c.At(p[0], p[1]); ok {
			t.Errorf("At(%v) accepted", p)
		}
	}
}

func TestCanvasTimestampsIncrease(t *testing.T) {
	c := NewBlankCanvas(2, 2)
	fixed := time.UnixMilli(1000)
	c.now = func() time.Time { return fixed }
	_, _, a, _ := c.Set(0, 0, blue, 1)
	_, _, b, _ := c.Set(1, 0, blue, 1)
	if b <= a {
		t.Fatalf("timestamps not increasing: %d then %d", a, b)
	}
}

func TestCanvasPNGCacheInvalidated(t *testing.T) {
	c := NewBlankCanvas(3, 3)
	first := c.ImagePNG()
	if !bytes.Equal(first, c.ImagePNG()) {
		t.Fatal("cache not reused")
	}
	c.Set(0, 0, blue, 1)
	img, err := png.Decode(bytes.NewReader(c.ImagePNG()))
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(0, 0).RGBA(); r>>8 != 0x5e || g>>8 != 0xb3 || b>>8 != 0xff {
		t.Fatal("PNG not refreshed after Set")
	}
}

func TestOwnersPNG(t *testing.T) {
	c := NewBlankCanvas(3, 2)
	c.Set(2, 1, blue, 0x012345)
	img, err := png.Decode(bytes.NewReader(c.OwnersPNG()))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 3 || img.Bounds().Dy() != 2 {
		t.Fatalf("bounds %v", img.Bounds())
	}
	r, g, b, _ := img.At(2, 1).RGBA()
	if id := (r>>8)<<16 | (g>>8)<<8 | b>>8; id != 0x012345 {
		t.Fatalf("owner id %x", id)
	}
	if r, g, b, _ := img.At(0, 0).RGBA(); r|g|b != 0 {
		t.Fatal("unowned pixel must be 0")
	}
}

func TestOwnersRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := NewBlankCanvas(4, 3)
	c.Set(3, 2, blue, 42)
	if err := c.SaveFiles(filepath.Join(dir, "place.png"), filepath.Join(dir, "owners.bin")); err != nil {
		t.Fatal(err)
	}

	img := loadTestPNG(t, filepath.Join(dir, "place.png"))
	d := NewCanvas(img)
	f, _ := os.Open(filepath.Join(dir, "owners.bin"))
	defer f.Close()
	snap, err := d.ReadOwners(f)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Matches || snap.SavedAt == 0 {
		t.Fatalf("snapshot %+v", snap)
	}
	if _, owner, _ := d.At(3, 2); owner != 42 {
		t.Fatalf("owner %d", owner)
	}
}

func TestOwnersGrowCanvas(t *testing.T) {
	dir := t.TempDir()
	small := NewBlankCanvas(2, 2)
	small.Set(1, 1, blue, 5)
	small.SaveFiles(filepath.Join(dir, "p.png"), filepath.Join(dir, "o.bin"))

	big := NewBlankCanvas(4, 3)
	f, _ := os.Open(filepath.Join(dir, "o.bin"))
	defer f.Close()
	snap, err := big.ReadOwners(f)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Matches {
		t.Fatal("a different size cannot match")
	}
	if _, owner, _ := big.At(1, 1); owner != 5 {
		t.Fatalf("owner copied to top-left: got %d", owner)
	}
	if _, owner, _ := big.At(3, 2); owner != 0 {
		t.Fatal("new area must be unowned")
	}
}

func TestReadOwnersRejectsGarbage(t *testing.T) {
	c := NewBlankCanvas(2, 2)
	if _, err := c.ReadOwners(bytes.NewReader([]byte("not an owners file at all, sorry"))); err == nil {
		t.Fatal("garbage accepted")
	}
}

func loadTestPNG(t *testing.T, path string) *image.NRGBA {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	m := image.NewNRGBA(img.Bounds())
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			m.Set(x, y, img.At(x, y))
		}
	}
	return m
}

// ------------------------------------------------------------------

func TestRestoreReplaysEventsAfterCrash(t *testing.T) {
	dir := t.TempDir()
	st := newTestStore(t)
	u := mustUser(t, st, "Brindille")
	c := NewBlankCanvas(4, 4)
	place := func(x, y int, col color.NRGBA) {
		prev, prevOwner, ts, _ := c.Set(x, y, col, u.ID)
		if err := st.RecordPixel(PixelEvent{X: x, Y: y, Color: nrgbaToRGB(col), UserID: u.ID, PrevColor: nrgbaToRGB(prev), PrevUserID: prevOwner, TS: ts}); err != nil {
			t.Fatal(err)
		}
	}
	place(0, 0, blue)
	c.SaveFiles(filepath.Join(dir, "place.png"), filepath.Join(dir, "owners.bin"))
	time.Sleep(2 * time.Millisecond)
	place(1, 1, blue) // placed after the last save, then "crash"

	restarted := NewCanvas(loadTestPNG(t, filepath.Join(dir, "place.png")))
	n, err := RestoreOwners(restarted, st, filepath.Join(dir, "owners.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("replayed %d events", n)
	}
	col, owner, _ := restarted.At(1, 1)
	if col != blue || owner != u.ID {
		t.Fatalf("pixel after crash not recovered: %v %d", col, owner)
	}
	got, _ := st.UserByID(u.ID)
	if got.PixelsVisible != 2 {
		t.Fatalf("visible = %d, want 2", got.PixelsVisible)
	}
}

func TestRestoreReconcilesWithAnotherImage(t *testing.T) {
	st := newTestStore(t)
	u := mustUser(t, st, "Kaelen")
	st.RecordPixel(PixelEvent{X: 0, Y: 0, Color: 0x5eb3ff, UserID: u.ID, PrevColor: 0xffffff, TS: 10})
	st.RecordPixel(PixelEvent{X: 1, Y: 0, Color: 0x000000, UserID: u.ID, PrevColor: 0xffffff, TS: 11})

	// An operator loads an image where (0,0) still shows Kaelen's colour but (1,0) was rolled back.
	c := NewBlankCanvas(2, 1)
	c.img.SetNRGBA(0, 0, blue)
	if _, err := RestoreOwners(c, st, ""); err != nil {
		t.Fatal(err)
	}
	if _, owner, _ := c.At(0, 0); owner != u.ID {
		t.Fatalf("matching colour must keep its author, got %d", owner)
	}
	if _, owner, _ := c.At(1, 0); owner != 0 {
		t.Fatalf("rolled back pixel must lose its author, got %d", owner)
	}
	got, _ := st.UserByID(u.ID)
	if got.PixelsVisible != 1 {
		t.Fatalf("visible = %d, want 1", got.PixelsVisible)
	}
}
