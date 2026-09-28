package place

// Admin dashboard (a-dashboard): live KPIs, pixels per hour, top players, real history
// from the backups, colours of the canvas, heatmap and the most retouched areas.

import (
	"image"
	"image/color"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

func (api *API) mountStats() {
	api.mux.HandleFunc("/api/admin/stats", api.admin(api.handleStats))
	api.mux.HandleFunc("/api/admin/heatmap.png", api.admin(api.handleHeatmap))
}

func periodBounds(period string, now time.Time) (from, prevFrom int64, label string) {
	switch period {
	case "semaine":
		return now.AddDate(0, 0, -7).UnixMilli(), now.AddDate(0, 0, -14).UnixMilli(), "7 jours"
	case "mois":
		return now.AddDate(0, -1, 0).UnixMilli(), now.AddDate(0, -2, 0).UnixMilli(), "30 jours"
	}
	return now.Add(-24 * time.Hour).UnixMilli(), now.Add(-48 * time.Hour).UnixMilli(), "24 h"
}

func (api *API) handleStats(w http.ResponseWriter, r *http.Request, admin *User) {
	if !getOnly(w, r) {
		return
	}
	now := api.store.now()
	period := r.URL.Query().Get("periode")
	from, prev, label := periodBounds(period, now)
	db := api.store.db
	count := func(q string, args ...any) int {
		var n int
		db.QueryRow(q, args...).Scan(&n)
		return n
	}
	online, slots := api.hub.Online()
	claims, _ := api.store.ClaimCounts()
	kpi := map[string]any{
		"connectes": online, "connexions": api.hub.Connections(), "slots": slots,
		"pixels_1h":       count(`SELECT COUNT(*) FROM pixel_events WHERE ts >= ?`, now.Add(-time.Hour).UnixMilli()),
		"pixels":          count(`SELECT COUNT(*) FROM pixel_events WHERE ts >= ?`, from),
		"pixels_prev":     count(`SELECT COUNT(*) FROM pixel_events WHERE ts >= ? AND ts < ?`, prev, from),
		"actifs":          count(`SELECT COUNT(DISTINCT user_id) FROM pixel_events WHERE ts >= ?`, from),
		"actifs_prev":     count(`SELECT COUNT(DISTINCT user_id) FROM pixel_events WHERE ts >= ? AND ts < ?`, prev, from),
		"nouveaux":        count(`SELECT COUNT(*) FROM users WHERE cree_le >= ?`, from),
		"nouveaux_prev":   count(`SELECT COUNT(*) FROM users WHERE cree_le >= ? AND cree_le < ?`, prev, from),
		"a_valider":       claims["attente"] + api.store.PendingReports(),
		"claims_attente":  claims["attente"],
		"reports_attente": api.store.PendingReports(),
		"periode":         label,
	}
	// pixels per hour today (Paris)
	hours := make([]int, 24)
	day := DayStart(now)
	rows, err := db.Query(`SELECT ts FROM pixel_events WHERE ts >= ?`, day.UnixMilli())
	if err == nil {
		for rows.Next() {
			var ts int64
			rows.Scan(&ts)
			hours[time.UnixMilli(ts).In(Paris).Hour()]++
		}
		rows.Close()
	}
	// top players over 7 days
	top, _ := api.store.RankSince(now.AddDate(0, 0, -7).UnixMilli(), 0, 6)
	ids := make([]uint32, len(top))
	for i, e := range top {
		ids[i] = e.UserID
	}
	users, _ := api.store.UsersByIDs(ids)
	topOut := []map[string]any{}
	for _, e := range top {
		topOut = append(topOut, map[string]any{"user": users[e.UserID].Public(), "pixels": e.Pixels})
	}
	res := map[string]any{"kpi": kpi, "heures": hours, "top": topOut, "couleurs": api.canvasColors(), "sauvegardes": api.backupStatus()}
	if idx := api.index(); idx != nil {
		days := snapshotDays(idx)
		months := map[string]int{}
		for _, d := range days {
			months[d.Date[:7]] += d.Minutes
		}
		var hist []map[string]any
		if len(days) > 0 {
			t, _ := time.Parse("2006-01", days[0].Date[:7])
			end, _ := time.Parse("2006-01", days[len(days)-1].Date[:7])
			for ; !t.After(end); t = t.AddDate(0, 1, 0) {
				m := t.Format("2006-01")
				hist = append(hist, map[string]any{"mois": m, "minutes": months[m]})
			}
		}
		top := append([]snapshotDay(nil), days...)
		sort.SliceStable(top, func(i, j int) bool { return top[i].Minutes > top[j].Minutes })
		if len(top) > 5 {
			top = top[:5]
		}
		res["historique"] = hist
		res["jours"] = top
	}
	res["zones"] = api.hotZones(6)
	writeJSON(w, res)
}

// canvasColors counts the colours of the live canvas.
func (api *API) canvasColors() map[string]any {
	counts := map[uint32]int{}
	api.canvas.mu.RLock()
	pix := api.canvas.img.Pix
	for i := 0; i+3 < len(pix); i += 4 {
		counts[uint32(pix[i])<<16|uint32(pix[i+1])<<8|uint32(pix[i+2])]++
	}
	total := len(pix) / 4
	api.canvas.mu.RUnlock()
	type kv struct {
		c uint32
		n int
	}
	var list []kv
	inPalette := 0
	pal := map[uint32]bool{}
	for _, h := range paletteHex {
		if c, ok := parseHexColor(h); ok {
			pal[nrgbaToRGB(c)] = true
		}
	}
	for c, n := range counts {
		list = append(list, kv{c, n})
		if pal[c] {
			inPalette += n
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].n > list[j].n })
	out := []map[string]any{}
	for i, e := range list {
		if i >= 8 {
			break
		}
		out = append(out, map[string]any{"hex": hexColor(e.c), "pixels": e.n, "pct": round1(float64(e.n) / float64(total))})
	}
	return map[string]any{"top": out, "distinctes": len(counts), "palette_pct": round1(float64(inPalette) / float64(total))}
}

// Heat: number of changes per pixel, from the backups and the pixel journal.
var heatCache = struct {
	sync.Mutex
	key  string
	heat []int
	w, h int
}{}

func (api *API) heat() ([]int, int, int) {
	cw, ch := api.canvas.Size()
	var events int
	api.store.db.QueryRow(`SELECT COUNT(*) FROM pixel_events`).Scan(&events)
	idx := api.index()
	frames := 0
	if idx != nil {
		frames = len(idx.Frames)
	}
	key := strconv.Itoa(frames) + ":" + strconv.Itoa(events/100)
	heatCache.Lock()
	defer heatCache.Unlock()
	if heatCache.key == key && heatCache.w == cw {
		return heatCache.heat, heatCache.w, heatCache.h
	}
	heat := make([]int, cw*ch)
	if idx != nil {
		for _, p := range idx.EvPos {
			x, y := int(p)%idx.W, int(p)/idx.W
			if x < cw && y < ch {
				heat[y*cw+x]++
			}
		}
	}
	rows, err := api.store.db.Query(`SELECT x, y FROM pixel_events`)
	if err == nil {
		for rows.Next() {
			var x, y int
			rows.Scan(&x, &y)
			if x < cw && y < ch {
				heat[y*cw+x]++
			}
		}
		rows.Close()
	}
	heatCache.key, heatCache.heat, heatCache.w, heatCache.h = key, heat, cw, ch
	return heat, cw, ch
}

// hotZones returns the 32×32 cells with the most changes.
func (api *API) hotZones(n int) []map[string]any {
	heat, cw, ch := api.heat()
	type cell struct{ x, y, score int }
	var cells []cell
	for cy := 0; cy < ch; cy += 32 {
		for cx := 0; cx < cw; cx += 32 {
			s := 0
			for y := cy; y < min(cy+32, ch); y++ {
				for x := cx; x < min(cx+32, cw); x++ {
					s += heat[y*cw+x]
				}
			}
			if s > 0 {
				cells = append(cells, cell{cx, cy, s})
			}
		}
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i].score > cells[j].score })
	out := []map[string]any{}
	for i, c := range cells {
		if i >= n {
			break
		}
		z := map[string]any{"x": c.x, "y": c.y, "score": c.score}
		if oid := api.oeuvres.At(c.x+16, c.y+16, cw); oid != 0 {
			if o, _ := api.store.Oeuvre(oid); o != nil {
				z["oeuvre"] = o.Titre
			}
		}
		out = append(out, z)
	}
	return out
}

// GET /api/admin/heatmap.png — the canvas in grey, changes from calm to hot.
func (api *API) handleHeatmap(w http.ResponseWriter, r *http.Request, admin *User) {
	heat, cw, ch := api.heat()
	maxV := 1
	for _, v := range heat {
		maxV = max(maxV, v)
	}
	img := image.NewNRGBA(image.Rect(0, 0, cw, ch))
	// one hue (gold), dark to bright: sequential data gets a single-hue ramp
	ramp := []color.NRGBA{{58, 42, 18, 255}, {124, 88, 34, 255}, {196, 142, 58, 255}, {240, 183, 90, 255}, {255, 236, 190, 255}}
	api.canvas.mu.RLock()
	for i, v := range heat {
		p := api.canvas.img.Pix[i*4:]
		g := uint8((int(p[0])*3 + int(p[1])*6 + int(p[2])) / 10 / 5)
		if v == 0 {
			img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = g, g, g, 255
			continue
		}
		// logarithmic scale, so a few very busy pixels do not hide the rest
		t := logScale(v, maxV) * float64(len(ramp)-1)
		k := min(int(t), len(ramp)-2)
		f := t - float64(k)
		a, b := ramp[k], ramp[k+1]
		img.Pix[i*4] = uint8(float64(a.R)*(1-f) + float64(b.R)*f)
		img.Pix[i*4+1] = uint8(float64(a.G)*(1-f) + float64(b.G)*f)
		img.Pix[i*4+2] = uint8(float64(a.B)*(1-f) + float64(b.B)*f)
		img.Pix[i*4+3] = 255
	}
	api.canvas.mu.RUnlock()
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=120")
	w.Write(encodePNG(img))
}

func logScale(v, maxV int) float64 {
	if maxV <= 1 {
		return 1
	}
	l := func(x int) float64 {
		f := 0.0
		for n := x; n > 0; n >>= 1 {
			f++
		}
		return f
	}
	return min(1, l(v)/l(maxV))
}
