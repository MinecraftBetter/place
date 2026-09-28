package place

// Artwork pages (d-oeuvre): before/after, construction replay, history.

import (
	"fmt"
	"image"
	"image/color"
	"net/http"
	"sort"
	"strconv"
	"time"
)

func init() {
	OeuvreRoutes["image.png"] = handleImagePNG
	OeuvreRoutes["avant.png"] = handleAvantPNG
	OeuvreRoutes["construction.png"] = handleConstructionPNG
}

// GET /api/oeuvres/:id/image.png?z= — the artwork today, cut along its zone: its exact
// box, transparent outside (the museum frames it on a dark mat).
func handleImagePNG(api *API, w http.ResponseWriter, r *http.Request, o *Oeuvre) {
	cw, ch := api.canvas.Size()
	box := o.Mask.Bounds(cw, ch).Intersect(image.Rect(0, 0, cw, ch))
	if box.Empty() {
		writeError(w, http.StatusNotFound, "Zone invalide.")
		return
	}
	z, _ := strconv.Atoi(r.URL.Query().Get("z"))
	z = max(1, min(z, 16))
	img := image.NewNRGBA(image.Rect(0, 0, box.Dx()*z, box.Dy()*z))
	for _, p := range maskPositions(o.Mask, cw, ch) {
		x, y := int(p)%cw, int(p)/cw
		if !image.Pt(x, y).In(box) {
			continue
		}
		c, _, _ := api.canvas.At(x, y)
		fillCell(img, x-box.Min.X, y-box.Min.Y, z, c)
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(encodePNG(img))
}

// fillCell paints the z×z cell (cx, cy) of img.
func fillCell(img *image.NRGBA, cx, cy, z int, c color.NRGBA) {
	for dy := 0; dy < z; dy++ {
		row := img.Pix[(cy*z+dy)*img.Stride:]
		for dx := 0; dx < z; dx++ {
			o := (cx*z + dx) * 4
			row[o], row[o+1], row[o+2], row[o+3] = c.R, c.G, c.B, 255
		}
	}
}

// beforeFrame picks the capture shown as "avant": when the artwork was finished,
// else when it appeared, else the first capture.
func beforeFrame(idx *BackupIndex, a *ZoneAnalysis) int {
	for _, t := range []int64{a.Finished, a.Appeared} {
		if t != 0 {
			if k := idx.FrameAt(t / 1000); k >= 0 {
				return k
			}
		}
	}
	return 0
}

// GET /api/oeuvres/:id/avant.png?z= — the artwork in the backups (the "before").
func handleAvantPNG(api *API, w http.ResponseWriter, r *http.Request, o *Oeuvre) {
	idx := api.index()
	a := o.Analysis()
	if idx == nil || a == nil {
		writeError(w, http.StatusNotFound, "Pas de sauvegarde pour cette œuvre.")
		return
	}
	pos, err := o.Mask.Positions(idx.W, idx.H)
	if err != nil {
		writeError(w, http.StatusNotFound, "Zone invalide.")
		return
	}
	z, _ := strconv.Atoi(r.URL.Query().Get("z"))
	z = max(1, min(z, 16))
	k := beforeFrame(idx, a)
	if q := r.URL.Query().Get("t"); q != "" {
		if t, err := strconv.ParseInt(q, 10, 64); err == nil {
			k = max(0, idx.FrameAt(t/1000))
		}
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Frame-Time", strconv.FormatInt(idx.Frames[k].T*1000, 10))
	w.Header().Set("Cache-Control", "private, max-age=600")
	w.Write(ZoneImage(idx, nil, pos, k, z))
}

// GET /api/oeuvres/:id/construction.png?n=16&z= — n frames of the construction,
// side by side without gaps (a sprite the page animates). X-Frame-Times lists them.
func handleConstructionPNG(api *API, w http.ResponseWriter, r *http.Request, o *Oeuvre) {
	idx := api.index()
	if idx == nil {
		writeError(w, http.StatusNotFound, "Pas de sauvegarde pour cette œuvre.")
		return
	}
	pos, err := o.Mask.Positions(idx.W, idx.H)
	if err != nil {
		writeError(w, http.StatusNotFound, "Zone invalide.")
		return
	}
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	n = max(4, min(n, 48))
	z, _ := strconv.Atoi(r.URL.Query().Get("z"))
	frames := constructionFrames(idx, pos, n)
	box := o.Mask.Bounds(idx.W, idx.H).Intersect(image.Rect(0, 0, idx.W, idx.H))
	img, times := ZoneSprite(idx, pos, box, frames, max(1, min(z, 8)))
	tj := "["
	for i, t := range times {
		if i > 0 {
			tj += ","
		}
		tj += strconv.FormatInt(t, 10)
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Frame-Times", tj+"]")
	w.Header().Set("Cache-Control", "private, max-age=600")
	w.Write(img)
}

// constructionFrames spreads n frames over the changes of a zone (by change, not by time,
// so a busy evening gets more frames than a quiet year).
func constructionFrames(idx *BackupIndex, pos []int32, n int) []int {
	z := replayZone(idx, pos)
	if len(z.evFrame) == 0 {
		return []int{len(idx.Frames) - 1}
	}
	var frames []int
	if first := int(z.evFrame[0]) - 1; first >= 0 {
		frames = append(frames, first)
	}
	for i := 1; i <= n-2; i++ {
		frames = append(frames, int(z.evFrame[i*(len(z.evFrame)-1)/(n-2)]))
	}
	frames = append(frames, len(idx.Frames)-1)
	return uniqueSorted(frames, len(idx.Frames))
}

// ZoneSprite renders frames of a zone side by side at scale z: each frame is box, the
// pixels outside the zone transparent (the museum lays it over the artwork's image).
func ZoneSprite(idx *BackupIndex, pos []int32, box image.Rectangle, frames []int, z int) ([]byte, []int64) {
	bw, bh := box.Dx(), box.Dy()
	if z <= 1 {
		z = max(1, min(8, 160/max(bw, bh, 1)))
	}
	cells := make([]int, 0, len(pos)) // cell in the box of each position kept
	rank := make(map[int32]int, len(pos))
	var kept []int32
	for _, p := range pos {
		x, y := int(p)%idx.W, int(p)/idx.W
		if !image.Pt(x, y).In(box) {
			continue
		}
		rank[p] = len(kept)
		kept = append(kept, p)
		cells = append(cells, (y-box.Min.Y)*bw+x-box.Min.X)
	}
	state := make([]uint32, len(kept))
	for i, p := range kept {
		state[i] = idx.First[p]
	}
	img := image.NewNRGBA(image.Rect(0, 0, len(frames)*bw*z, bh*z))
	times := make([]int64, len(frames))
	e := 0
	for fi, k := range frames {
		for e < len(idx.EvPos) && int(idx.EvFrame[e]) <= k {
			if r, ok := rank[idx.EvPos[e]]; ok {
				state[r] = idx.EvCol[e]
			}
			e++
		}
		times[fi] = idx.Frames[k].T * 1000
		for i, cell := range cells {
			fillCell(img, fi*bw+cell%bw, cell/bw, z, rgbToNRGBA(state[i]))
		}
	}
	return encodePNG(img), times
}

type contributor struct {
	User    *PublicUser `json:"user"`
	Role    string      `json:"role"` // auteur | co_auteur | retouche
	Pioneer int64       `json:"pixels_pionniers,omitempty"`
	Pixels  int         `json:"pixels,omitempty"`
	Last    int64       `json:"dernier,omitempty"`
}

type historyItem struct {
	TS    int64  `json:"ts"`
	Label string `json:"label"` // e.g. "13 sept. 2022" or "sept. 2022 → 2024"
	Text  string `json:"texte"`
}

// oeuvreStory builds the contributors and the history of an artwork.
func (api *API) oeuvreStory(o *Oeuvre) ([]contributor, []historyItem) {
	contrib := []contributor{}
	authors := map[uint32]bool{}
	for _, a := range o.Auteurs {
		authors[a.userID] = true
		contrib = append(contrib, contributor{User: a.User, Role: a.Role, Pioneer: a.Pioneer})
	}
	// players who changed the zone since accounts opened
	cw, ch := api.canvas.Size()
	counts := map[uint32]int{}
	last := map[uint32]int64{}
	rows, err := api.store.db.Query(`SELECT x, y, user_id, ts FROM pixel_events WHERE x >= ? AND x < ? AND y >= ? AND y < ?`, o.X, o.X+o.W, o.Y, o.Y+o.H)
	if err == nil {
		in := map[int32]bool{}
		for _, p := range maskPositions(o.Mask, cw, ch) {
			in[p] = true
		}
		for rows.Next() {
			var x, y int
			var uid uint32
			var ts int64
			rows.Scan(&x, &y, &uid, &ts)
			if in[int32(y*cw+x)] && !authors[uid] {
				counts[uid]++
				last[uid] = max(last[uid], ts)
			}
		}
		rows.Close()
	}
	var ids []uint32
	for id := range counts {
		ids = append(ids, id)
	}
	users, _ := api.store.UsersByIDs(ids)
	sort.Slice(ids, func(i, j int) bool { return counts[ids[i]] > counts[ids[j]] })
	for _, id := range ids {
		contrib = append(contrib, contributor{User: users[id].Public(), Role: "retouche", Pixels: counts[id], Last: last[id]})
	}

	hist := []historyItem{}
	idx := api.index()
	if a := o.Analysis(); a != nil {
		if a.Appeared != 0 {
			t := time.UnixMilli(a.Appeared)
			text := "Apparition de l'œuvre"
			if a.AlreadyThere {
				text = "Déjà là à la toute première sauvegarde du canvas"
			}
			if idx != nil {
				if k := idx.FrameAt(t.Unix()); k >= 0 {
					f := idx.Frames[k]
					text += fmt.Sprintf(" (canvas de %d × %d", f.W, f.H)
					if n := framesOnDay(idx, t); n > 1 {
						text += fmt.Sprintf(", %d min d'activité ce jour-là", n)
					}
					text += ")"
				}
			}
			hist = append(hist, historyItem{a.Appeared, frenchDate(t), text})
		}
		if a.Finished != 0 && a.Finished != a.Appeared {
			hist = append(hist, historyItem{a.Finished, frenchDate(time.UnixMilli(a.Finished)), fmt.Sprintf("Terminée : ≈ %s pixels posés", frenchNumber(a.Placed))})
		}
		for _, rt := range a.Retouches {
			text := fmt.Sprintf("Retouchée (jusqu'à %d %% de l'œuvre intacte)", int(rt.MinPct))
			if rt.RestoredAt != 0 {
				text += ", restaurée " + frenchDate(time.UnixMilli(rt.RestoredAt))
			}
			hist = append(hist, historyItem{rt.At, frenchDate(time.UnixMilli(rt.At)), text})
		}
		if len(a.Retouches) == 0 && a.Finished != 0 {
			months := int((a.LastCapture - a.Finished) / (30 * 24 * 3600 * 1000))
			if months >= 2 {
				hist = append(hist, historyItem{a.Finished + 1, monthYear(time.UnixMilli(a.Finished)) + " → " + monthYear(time.UnixMilli(a.LastCapture)),
					fmt.Sprintf("Reste identique pendant %d mois", months)})
			}
		}
	}
	for _, c := range contrib {
		if c.Role == "retouche" && c.User != nil {
			hist = append(hist, historyItem{c.Last, monthYear(time.UnixMilli(c.Last)), fmt.Sprintf("%s retouche %d pixel%s", c.User.Pseudo, c.Pixels, plural(c.Pixels))})
		}
	}
	if o.ClaimID != 0 {
		var decided int64
		var confirms int
		api.store.db.QueryRow(`SELECT COALESCE(decide_le, 0) FROM claims WHERE id = ?`, o.ClaimID).Scan(&decided)
		api.store.db.QueryRow(`SELECT COUNT(*) FROM claim_votes WHERE claim_id = ? AND type = 'confirme'`, o.ClaimID).Scan(&confirms)
		var names []string
		for _, a := range o.Auteurs {
			if a.User != nil {
				names = append(names, a.User.Pseudo)
			}
		}
		text := "Revendiquée"
		if len(names) > 0 {
			text += " par " + joinFR(names)
		}
		if confirms > 0 {
			text += fmt.Sprintf(" · %d confirmation%s", confirms, plural(confirms))
		}
		text += " · validée"
		if decided == 0 {
			decided = o.CreeLe
		}
		hist = append(hist, historyItem{decided, frenchDate(time.UnixMilli(decided)), text})
	}
	sort.SliceStable(hist, func(i, j int) bool { return hist[i].TS < hist[j].TS })
	return contrib, hist
}

func framesOnDay(idx *BackupIndex, t time.Time) int {
	day := t.In(Paris).Format("2006-01-02")
	n := 0
	for _, f := range idx.Frames {
		if time.Unix(f.T, 0).In(Paris).Format("2006-01-02") == day {
			n++
		}
	}
	return n
}

func monthYear(t time.Time) string {
	t = t.In(Paris)
	return fmt.Sprintf("%s %d", frenchMonths[t.Month()-1], t.Year())
}

func plural(n int) string {
	if n > 1 {
		return "s"
	}
	return ""
}

func joinFR(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	out := names[0]
	for _, n := range names[1 : len(names)-1] {
		out += ", " + n
	}
	return out + " et " + names[len(names)-1]
}
