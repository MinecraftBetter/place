package place

// The modern timelapse (d-timelapse): one capture per active day, activity per day,
// archives per month, the canvas growing — and any day replayed minute by minute
// from every capture of the backups.

import (
	"bytes"
	"encoding/binary"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type snapshotDay struct {
	Date    string `json:"date"` // Paris date
	Frame   int    `json:"frame"`
	T       int64  `json:"t"`
	W       int    `json:"w"`
	H       int    `json:"h"`
	Minutes int    `json:"minutes"` // captures that day (one per minute of activity)
	Changes int    `json:"changes"`
}

func (api *API) mountSnapshots() {
	api.mux.HandleFunc("/api/snapshots", api.handleSnapshots)
	api.mux.HandleFunc("/api/snapshots/", api.handleSnapshot)
}

func snapshotDays(idx *BackupIndex) []snapshotDay {
	var days []snapshotDay
	changes := make([]int, len(idx.Frames))
	for _, k := range idx.EvFrame {
		changes[k]++
	}
	for k, f := range idx.Frames {
		d := time.Unix(f.T, 0).In(Paris).Format("2006-01-02")
		if n := len(days); n > 0 && days[n-1].Date == d {
			days[n-1].Frame, days[n-1].T, days[n-1].W, days[n-1].H = k, f.T*1000, f.W, f.H
			days[n-1].Minutes++
			days[n-1].Changes += changes[k]
			continue
		}
		days = append(days, snapshotDay{Date: d, Frame: k, T: f.T * 1000, W: f.W, H: f.H, Minutes: 1, Changes: changes[k]})
	}
	return days
}

// GET /api/snapshots
func (api *API) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	idx := api.index()
	if idx == nil {
		writeJSON(w, map[string]any{"days": []any{}, "sauvegardes": api.backupStatus()})
		return
	}
	days := snapshotDays(idx)
	months := map[string]int{}
	for _, d := range days {
		months[d.Date[:7]]++
	}
	// every month between the first and the last capture, empty ones included
	var archive []map[string]any
	if len(days) > 0 {
		t, _ := time.Parse("2006-01", days[0].Date[:7])
		end, _ := time.Parse("2006-01", days[len(days)-1].Date[:7])
		for ; !t.After(end); t = t.AddDate(0, 1, 0) {
			m := t.Format("2006-01")
			entry := map[string]any{"month": m, "days": months[m]}
			// a link only when the monthly zip exists (timelapse.sh may not have made it)
			if api.archivesDir == "" || fileExists(filepath.Join(api.archivesDir, "archive-"+m+".zip")) {
				entry["href"] = "/archives/archive-" + m + ".zip"
			}
			archive = append(archive, entry)
		}
	}
	top := append([]snapshotDay(nil), days...)
	sort.SliceStable(top, func(i, j int) bool { return top[i].Minutes > top[j].Minutes })
	if len(top) > 5 {
		top = top[:5]
	}
	var sizes []map[string]any
	for _, d := range days {
		if n := len(sizes); n == 0 || sizes[n-1]["w"] != d.W || sizes[n-1]["h"] != d.H {
			sizes = append(sizes, map[string]any{"w": d.W, "h": d.H, "from": d.Date})
		}
	}
	writeJSON(w, map[string]any{"days": days, "months": archive, "top": top, "sizes": sizes, "width": idx.W, "height": idx.H,
		"frames": len(idx.Frames), "sauvegardes": api.backupStatus()})
}

// GET /api/snapshots/<frame>.png — an original capture.
// GET /api/snapshots/day/<YYYY-MM-DD>.bin — every change of a day, to replay it minute by minute:
//
//	"BPDY", uint32 version, uint32 start frame (the capture before the day), uint32 frame count,
//	frame count × int64 time (ms), uint32 change count, then change count × (uint32 frame, uint32 pos, uint32 rgb).
func (api *API) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	idx := api.index()
	if idx == nil {
		writeError(w, http.StatusServiceUnavailable, "Les sauvegardes ne sont pas prêtes.")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/snapshots/")
	if strings.HasPrefix(rest, "day/") {
		day := strings.TrimSuffix(strings.TrimPrefix(rest, "day/"), ".bin")
		first, last := -1, -1
		for k, f := range idx.Frames {
			if time.Unix(f.T, 0).In(Paris).Format("2006-01-02") == day {
				if first < 0 {
					first = k
				}
				last = k
			}
		}
		if first < 0 {
			writeError(w, http.StatusNotFound, "Pas de capture ce jour-là.")
			return
		}
		start := max(first-1, 0)
		var buf bytes.Buffer
		buf.WriteString("BPDY")
		le := binary.LittleEndian
		binary.Write(&buf, le, uint32(1))
		binary.Write(&buf, le, uint32(start))
		binary.Write(&buf, le, uint32(last-first+1))
		for k := first; k <= last; k++ {
			binary.Write(&buf, le, idx.Frames[k].T*1000)
		}
		lo := sort.Search(len(idx.EvFrame), func(i int) bool { return int(idx.EvFrame[i]) >= first })
		hi := sort.Search(len(idx.EvFrame), func(i int) bool { return int(idx.EvFrame[i]) > last })
		if first == start { // the day starts with the very first capture: nothing before it
			lo = sort.Search(len(idx.EvFrame), func(i int) bool { return int(idx.EvFrame[i]) > first })
		}
		binary.Write(&buf, le, uint32(hi-lo))
		for i := lo; i < hi; i++ {
			binary.Write(&buf, le, [3]uint32{uint32(idx.EvFrame[i]) - uint32(first), uint32(idx.EvPos[i]), idx.EvCol[i]})
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Canvas-Width", strconv.Itoa(idx.W))
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write(buf.Bytes())
		return
	}
	k, err := strconv.Atoi(strings.TrimSuffix(rest, ".png"))
	if err != nil || k < 0 || k >= len(idx.Frames) {
		writeError(w, http.StatusNotFound, "Capture introuvable.")
		return
	}
	b, err := ReadFrame(idx.Frames[k])
	if err != nil {
		writeError(w, http.StatusNotFound, "Capture illisible.")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Write(b)
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// SetArchivesDir tells where the monthly zips served at /archives/ are.
func (api *API) SetArchivesDir(dir string) { api.archivesDir = dir }
