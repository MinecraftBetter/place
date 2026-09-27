package place

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeCaptures writes PNG captures named like backup.sh does.
func writeCaptures(t *testing.T, dir string, caps []struct {
	at  time.Time
	img *image.NRGBA
}) {
	t.Helper()
	for _, c := range caps {
		sub := filepath.Join(dir, c.at.UTC().Format("2006-01"))
		os.MkdirAll(sub, 0755)
		f, err := os.Create(filepath.Join(sub, c.at.UTC().Format("2006-01-02_15-04-05")+".png"))
		if err != nil {
			t.Fatal(err)
		}
		png.Encode(f, c.img)
		f.Close()
	}
}

func blankImg(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	return img
}

func paint(img *image.NRGBA, x0, y0, w, h int, c [3]uint8) *image.NRGBA {
	out := image.NewNRGBA(img.Rect)
	copy(out.Pix, img.Pix)
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			i := out.PixOffset(x, y)
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = c[0], c[1], c[2], 255
		}
	}
	return out
}

func TestIndexAndAnalyzeZone(t *testing.T) {
	dir := t.TempDir()
	day := func(d, h, m int) time.Time { return time.Date(2022, 9, d, h, m, 0, 0, time.UTC) }
	blue := [3]uint8{0x5e, 0xb3, 0xff}
	black := [3]uint8{0, 0, 0}
	small := blankImg(8, 8) // the canvas started smaller
	f1 := paint(blankImg(16, 8), 0, 0, 2, 2, blue)
	f2 := paint(f1, 0, 0, 4, 4, blue)  // artwork done: 4×4 blue at (0,0)
	f3 := paint(f2, 1, 1, 2, 2, black) // someone adds a joke in it (4 px out of 16)
	f4 := paint(f3, 0, 0, 4, 4, blue)  // restored
	writeCaptures(t, dir, []struct {
		at  time.Time
		img *image.NRGBA
	}{
		{day(10, 20, 0), small}, {day(11, 1, 0), f1}, {day(11, 1, 20), f2}, {day(12, 10, 0), f3}, {day(13, 10, 0), f4},
	})
	os.WriteFile(filepath.Join(dir, "2022-09", "2022-09-12_11-00-00.png"), nil, 0644) // empty capture: skipped

	frames, err := ListFrames([]string{dir})
	if err != nil || len(frames) != 6 {
		t.Fatalf("frames %d %v", len(frames), err)
	}
	idx, err := BuildIndex(frames, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if idx.W != 16 || idx.H != 8 || len(idx.Frames) != 5 || idx.Skipped != 1 {
		t.Fatalf("index %dx%d frames %d skipped %d", idx.W, idx.H, len(idx.Frames), idx.Skipped)
	}
	if idx.First[9] != white { // padded area of the small first capture
		t.Fatal("small capture must be padded white")
	}

	pos, _ := Mask{Type: "rect", Rect: [4]int{0, 0, 4, 4}}.Positions(16, 8)
	now := day(14, 0, 0)
	a := AnalyzeZone(idx, pos, time.Time{}, now)
	if a.ArtPixels != 16 || a.NowPct != 100 || a.AlreadyThere {
		t.Fatalf("basics %+v", a)
	}
	if a.Appeared != day(11, 1, 0).UnixMilli() || a.Finished != day(11, 1, 20).UnixMilli() {
		t.Fatalf("appeared %v finished %v", time.UnixMilli(a.Appeared).UTC(), time.UnixMilli(a.Finished).UTC())
	}
	if a.Placed != 16 || len(a.Sessions) != 1 || a.Sessions[0].Pixels != 16 || a.Sessions[0].Minute != 20 {
		t.Fatalf("construction %d, sessions %+v", a.Placed, a.Sessions)
	}
	if len(a.Days) != 1 || a.Days[0] != "2022-09-11" || a.PerHour[3] != 16 || a.Night != 16 {
		t.Fatalf("days %v perHour %v night %d (1:00 UTC = 3:00 Paris)", a.Days, a.PerHour, a.Night)
	}
	if len(a.Retouches) != 1 || a.Retouches[0].MinPct != 75 || a.Retouches[0].RestoredAt != day(13, 10, 0).UnixMilli() {
		t.Fatalf("retouches %+v", a.Retouches)
	}
	if !contains(a.Badges, "pionnier") || !contains(a.Badges, "veteran") || contains(a.Badges, "batisseur") {
		t.Fatalf("badges %v", a.Badges)
	}
	// After the migration: no pioneer badge.
	if b := AnalyzeZone(idx, pos, day(1, 0, 0), now); contains(b.Badges, "pionnier") {
		t.Fatal("artwork drawn after the migration is not pioneer")
	}

	strip, times := ZoneStrip(idx, pos, a.KeyFrames(), 48)
	cfg, err := png.DecodeConfig(bytes.NewReader(strip))
	if err != nil || len(times) < 3 || cfg.Height < 40 {
		t.Fatalf("strip %v %d frames %dpx", err, len(times), cfg.Height)
	}
	before := ZoneStateAt(idx, pos, 0)
	if before[0] != white {
		t.Fatal("state at the first capture")
	}

	// Incremental update keeps the same result.
	idx2, _ := BuildIndex(frames[:3], nil, nil)
	idx3, err := BuildIndex(frames, idx2, nil)
	if err != nil || len(idx3.EvPos) != len(idx.EvPos) {
		t.Fatalf("incremental: %v %d vs %d", err, len(idx3.EvPos), len(idx.EvPos))
	}
	path := filepath.Join(t.TempDir(), "i.idx")
	if err := idx.Save(path); err != nil {
		t.Fatal(err)
	}
	if loaded, err := LoadBackupIndex(path); err != nil || len(loaded.EvPos) != len(idx.EvPos) {
		t.Fatalf("reload: %v", err)
	}
}

func TestMasks(t *testing.T) {
	rect, _ := Mask{Type: "rect", Rect: [4]int{2, 1, 3, 2}}.Positions(10, 10)
	if len(rect) != 6 || rect[0] != 12 {
		t.Fatalf("rect %v", rect)
	}
	tri, err := Mask{Type: "poly", Points: [][2]int{{0, 0}, {9, 0}, {0, 9}}}.Positions(10, 10)
	if err != nil || len(tri) < 40 || len(tri) > 60 {
		t.Fatalf("triangle %d px %v", len(tri), err)
	}
	bm := MaskFromPositions(rect, 10)
	back, err := bm.Positions(10, 10)
	if err != nil || len(back) != 6 || back[5] != rect[5] {
		t.Fatalf("bitmap round trip %v %v", back, err)
	}
	if Overlap(rect, tri) == 0 || Overlap(rect, []int32{99}) != 0 {
		t.Fatal("overlap")
	}
	for _, m := range []Mask{{Type: "rect"}, {Type: "poly", Points: [][2]int{{0, 0}, {1, 1}}}, {Type: "bitmap", W: 2, H: 2, Bits: "!!"}, {Type: "cercle"}} {
		if _, err := m.Positions(10, 10); err == nil {
			t.Errorf("mask %+v accepted", m)
		}
	}
}
