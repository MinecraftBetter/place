package place

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Indexes the real archives when BP_ARCHIVES points to them (slow: every capture).
//   BP_ARCHIVES=~/BetterPlace/archives go test -run TestRealArchives -v
func TestRealArchives(t *testing.T) {
	dir := os.Getenv("BP_ARCHIVES")
	if dir == "" {
		t.Skip("BP_ARCHIVES not set")
	}
	frames, err := ListFrames([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d captures, %s → %s", len(frames), time.Unix(frames[0].T, 0).UTC(), time.Unix(frames[len(frames)-1].T, 0).UTC())
	start := time.Now()
	idx, err := BuildIndex(frames, nil, func(done, total int) {
		if done%2000 == 0 {
			t.Logf("%d/%d (%s)", done, total, time.Since(start).Round(time.Second))
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("indexed %d frames (%d skipped), %d changes, %dx%d in %s", len(idx.Frames), idx.Skipped, len(idx.EvPos), idx.W, idx.H, time.Since(start).Round(time.Second))
	out := filepath.Join(os.TempDir(), "bp-backups-test.idx")
	if err := idx.Save(out); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(out)
	t.Logf("index file %d MB → %s", st.Size()>>20, out)
}
