package place

// Admin — zone tool (a-zone): restore a zone from a backup, erase it, or give it to a
// player directly. Changes are journaled with the previous pixels so they can be undone.

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

type pixelChange struct {
	Pos   int32
	Color uint32
	Owner uint32
}

// ApplyChanges sets pixels and owners at once and returns what they were.
func (c *Canvas) ApplyChanges(changes []pixelChange) []pixelChange {
	c.mu.Lock()
	defer c.mu.Unlock()
	prev := make([]pixelChange, 0, len(changes))
	for _, ch := range changes {
		p := int(ch.Pos)
		if p < 0 || p >= len(c.owners) {
			continue
		}
		px := c.img.Pix[p*4 : p*4+4]
		prev = append(prev, pixelChange{ch.Pos, uint32(px[0])<<16 | uint32(px[1])<<8 | uint32(px[2]), c.owners[p]})
		px[0], px[1], px[2], px[3] = uint8(ch.Color>>16), uint8(ch.Color>>8), uint8(ch.Color), 255
		c.owners[p] = ch.Owner
	}
	c.version++
	c.imgPNG, c.ownPNG = nil, nil
	return prev
}

func encodeChanges(ch []pixelChange) string {
	var raw bytes.Buffer
	gz := gzip.NewWriter(&raw)
	binary.Write(gz, binary.LittleEndian, uint32(len(ch)))
	for _, c := range ch {
		binary.Write(gz, binary.LittleEndian, [3]uint32{uint32(c.Pos), c.Color, c.Owner})
	}
	gz.Close()
	return base64.StdEncoding.EncodeToString(raw.Bytes())
}

func decodeChanges(s string) ([]pixelChange, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	gz, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(gz)
	if err != nil || len(data) < 4 {
		return nil, fmt.Errorf("zone: bad record")
	}
	n := binary.LittleEndian.Uint32(data)
	if int(n)*12+4 > len(data) {
		return nil, fmt.Errorf("zone: truncated record")
	}
	out := make([]pixelChange, n)
	for i := range out {
		o := 4 + i*12
		out[i] = pixelChange{int32(binary.LittleEndian.Uint32(data[o:])), binary.LittleEndian.Uint32(data[o+4:]), binary.LittleEndian.Uint32(data[o+8:])}
	}
	return out, nil
}

type zoneRecord struct {
	Kind   string          `json:"kind"`
	Before string          `json:"avant,omitempty"` // encoded pixels before the action
	X      int             `json:"x"`
	Y      int             `json:"y"`
	W      int             `json:"w"`
	H      int             `json:"h"`
	Oeuvre *decisionRecord `json:"oeuvre,omitempty"` // zone.assign
}

func (api *API) mountZone() {
	api.mux.HandleFunc("/api/admin/zones/preview", api.admin(api.handleZonePreview))
	api.mux.HandleFunc("/api/admin/zones/backups", api.admin(api.handleZoneBackups))
	api.mux.HandleFunc("/api/admin/zones/apply", api.admin(api.handleZoneApply))
}

type zoneRequest struct {
	Mask   Mask     `json:"masque"`
	Action string   `json:"action"` // restore | erase | assign
	Frame  int      `json:"frame"`  // restore: backup capture
	Color  string   `json:"color"`  // erase: fill colour (default white)
	User   string   `json:"user"`   // assign: slug
	Titre  string   `json:"titre"`  // assign
	Badges []string `json:"badges"` // assign
	Motif  string   `json:"motif"`
}

// zoneChanges computes the pixels an action would change.
func (api *API) zoneChanges(req zoneRequest, pos []int32) ([]pixelChange, error) {
	cw, _ := api.canvas.Size()
	switch req.Action {
	case "erase":
		col := uint32(white)
		if c, ok := parseHexColor(req.Color); ok {
			col = nrgbaToRGB(c)
		}
		out := make([]pixelChange, len(pos))
		for i, p := range pos {
			out[i] = pixelChange{p, col, 0}
		}
		return out, nil
	case "restore":
		idx := api.index()
		if idx == nil {
			return nil, &FieldError{"frame", "Les sauvegardes ne sont pas disponibles."}
		}
		if req.Frame < 0 || req.Frame >= len(idx.Frames) {
			return nil, &FieldError{"frame", "Choisis une sauvegarde."}
		}
		// zone pixels in the backups' layout
		bpos := make([]int32, 0, len(pos))
		keep := make([]int32, 0, len(pos))
		for _, p := range pos {
			x, y := int(p)%cw, int(p)/cw
			if x < idx.W && y < idx.H {
				bpos = append(bpos, int32(y*idx.W+x))
				keep = append(keep, p)
			}
		}
		state := ZoneStateAt(idx, bpos, req.Frame)
		t := idx.Frames[req.Frame].T * 1000
		owners := api.store.ownersAt(keep, cw, t)
		out := make([]pixelChange, len(keep))
		for i, p := range keep {
			out[i] = pixelChange{p, state[i], owners[p]}
		}
		return out, nil
	}
	return nil, &FieldError{"action", "Action inconnue."}
}

// ownersAt returns who owned each pixel at time t (last event at or before t; 0 before accounts).
func (s *Store) ownersAt(pos []int32, cw int, t int64) map[int32]uint32 {
	res := map[int32]uint32{}
	if len(pos) == 0 {
		return res
	}
	minX, minY, maxX, maxY := cw, 1<<30, 0, 0
	in := map[int32]bool{}
	for _, p := range pos {
		x, y := int(p)%cw, int(p)/cw
		minX, maxX, minY, maxY = min(minX, x), max(maxX, x), min(minY, y), max(maxY, y)
		in[p] = true
	}
	rows, err := s.db.Query(`SELECT x, y, user_id FROM pixel_events WHERE ts <= ? AND x BETWEEN ? AND ? AND y BETWEEN ? AND ? ORDER BY ts, id`, t, minX, maxX, minY, maxY)
	if err != nil {
		return res
	}
	defer rows.Close()
	for rows.Next() {
		var x, y int
		var uid uint32
		rows.Scan(&x, &y, &uid)
		if p := int32(y*cw + x); in[p] {
			res[p] = uid
		}
	}
	return res
}

func zoneImageURL(api *API, pos []int32, changes []pixelChange) string {
	cw, ch := api.canvas.Size()
	box := cropBox(pos, cw, ch)
	scale := max(1, min(8, 240/max(box.Dx(), box.Dy())))
	img := image.NewNRGBA(image.Rect(0, 0, box.Dx()*scale, box.Dy()*scale))
	over := map[int32]uint32{}
	for _, c := range changes {
		over[c.Pos] = c.Color
	}
	api.canvas.mu.RLock()
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			p := int32(y*cw + x)
			px := api.canvas.img.Pix[int(p)*4:]
			r, g, b := px[0], px[1], px[2]
			if v, ok := over[p]; ok {
				r, g, b = uint8(v>>16), uint8(v>>8), uint8(v)
			}
			for dy := 0; dy < scale; dy++ {
				row := img.Pix[((y-box.Min.Y)*scale+dy)*img.Stride:]
				for dx := 0; dx < scale; dx++ {
					o := ((x-box.Min.X)*scale + dx) * 4
					row[o], row[o+1], row[o+2], row[o+3] = r, g, b, 255
				}
			}
		}
	}
	api.canvas.mu.RUnlock()
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(encodePNG(img))
}

// POST /api/admin/zones/preview — impact and before/after images of an action.
func (api *API) handleZonePreview(w http.ResponseWriter, r *http.Request, admin *User) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
		return
	}
	var req zoneRequest
	if !readJSON(w, r, &req) {
		return
	}
	cw, ch := api.canvas.Size()
	pos, err := req.Mask.Positions(cw, ch)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Zone invalide.")
		return
	}
	res := map[string]any{"pixels": len(pos), "maintenant": zoneImageURL(api, pos, nil)}
	b := req.Mask.Bounds(cw, ch)
	res["x"], res["y"], res["w"], res["h"] = b.Min.X, b.Min.Y, b.Dx(), b.Dy()
	// players owning pixels of the zone now
	counts := map[uint32]int{}
	api.canvas.mu.RLock()
	for _, p := range pos {
		if o := api.canvas.owners[p]; o != 0 {
			counts[o]++
		}
	}
	api.canvas.mu.RUnlock()
	var ids []uint32
	for id := range counts {
		ids = append(ids, id)
	}
	users, _ := api.store.UsersByIDs(ids)
	sort.Slice(ids, func(i, j int) bool { return counts[ids[i]] > counts[ids[j]] })
	players := []map[string]any{}
	for _, id := range ids {
		players = append(players, map[string]any{"user": users[id].Public(), "pixels": counts[id]})
	}
	res["joueurs"] = players
	if req.Action == "assign" {
		if a, err := api.analyse(req.Mask); err == nil && a != nil {
			res["analyse"] = a
		}
		writeJSON(w, res)
		return
	}
	changes, err := api.zoneChanges(req, pos)
	if writeFieldError(w, err) {
		return
	}
	changed := 0
	api.canvas.mu.RLock()
	for _, c := range changes {
		px := api.canvas.img.Pix[int(c.Pos)*4:]
		if uint32(px[0])<<16|uint32(px[1])<<8|uint32(px[2]) != c.Color {
			changed++
		}
	}
	api.canvas.mu.RUnlock()
	res["modifies"] = changed
	res["apres"] = zoneImageURL(api, pos, changes)
	writeJSON(w, res)
}

// POST /api/admin/zones/backups — captures worth restoring for a zone.
func (api *API) handleZoneBackups(w http.ResponseWriter, r *http.Request, admin *User) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
		return
	}
	var req zoneRequest
	if !readJSON(w, r, &req) {
		return
	}
	idx := api.index()
	if idx == nil {
		writeJSON(w, map[string]any{"backups": []any{}, "sauvegardes": api.backupStatus()})
		return
	}
	pos, err := req.Mask.Positions(idx.W, idx.H)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Zone invalide.")
		return
	}
	a := AnalyzeZone(idx, pos, api.store.MigrationTime(), api.store.now())
	frames := append([]int{}, a.KeyFrames()...)
	// the last capture of the recent days where the zone changed
	z := replayZone(idx, pos)
	seen := map[string]bool{}
	for i := len(z.evFrame) - 1; i >= 0 && len(seen) < 6; i-- {
		k := int(z.evFrame[i])
		d := time.Unix(idx.Frames[k].T, 0).In(Paris).Format("2006-01-02")
		if !seen[d] {
			seen[d] = true
			if k > 0 {
				frames = append(frames, k-1) // just before that day's last change
			}
		}
	}
	frames = uniqueSorted(frames, len(idx.Frames))
	cw, _ := api.canvas.Size()
	live := make([]uint32, len(pos))
	api.canvas.mu.RLock()
	for i, p := range pos {
		x, y := int(p)%idx.W, int(p)/idx.W
		px := api.canvas.img.Pix[(y*cw+x)*4:]
		live[i] = uint32(px[0])<<16 | uint32(px[1])<<8 | uint32(px[2])
	}
	api.canvas.mu.RUnlock()
	out := []map[string]any{}
	for i := len(frames) - 1; i >= 0; i-- {
		k := frames[i]
		st := ZoneStateAt(idx, pos, k)
		diff := 0
		for j := range st {
			if st[j] != live[j] {
				diff++
			}
		}
		out = append(out, map[string]any{"frame": k, "t": idx.Frames[k].T * 1000, "differents": diff})
	}
	writeJSON(w, map[string]any{"backups": out})
}

// POST /api/admin/zones/apply
func (api *API) handleZoneApply(w http.ResponseWriter, r *http.Request, admin *User) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
		return
	}
	var req zoneRequest
	if !readJSON(w, r, &req) {
		return
	}
	cw, ch := api.canvas.Size()
	pos, err := req.Mask.Positions(cw, ch)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Zone invalide.")
		return
	}
	b := req.Mask.Bounds(cw, ch)
	rec := zoneRecord{Kind: req.Action, X: b.Min.X, Y: b.Min.Y, W: b.Dx(), H: b.Dy()}
	var detail string
	switch req.Action {
	case "restore", "erase":
		changes, err := api.zoneChanges(req, pos)
		if writeFieldError(w, err) {
			return
		}
		prev := api.canvas.ApplyChanges(changes)
		rec.Before = encodeChanges(prev)
		api.afterZoneChange(b)
		if req.Action == "restore" {
			t := time.Unix(api.index().Frames[req.Frame].T, 0)
			detail = fmt.Sprintf("%s px restaurés depuis la sauvegarde du %s %s", frenchNumber(len(changes)), frenchDate(t), t.In(Paris).Format("15:04"))
		} else {
			detail = frenchNumber(len(changes)) + " px effacés"
		}
	case "assign":
		target, _ := api.store.UserBySlug(req.User)
		if target == nil {
			writeFieldError(w, &FieldError{"user", "Joueur introuvable."})
			return
		}
		titre := strings.Join(strings.Fields(req.Titre), " ")
		if titre == "" {
			writeFieldError(w, &FieldError{"titre", "Donne un titre à l'œuvre."})
			return
		}
		a, _ := api.analyse(req.Mask)
		dr, oid, err := api.store.CreateOeuvreDirect(target, req.Mask, titre, len(pos), a, req.Badges)
		if err != nil {
			api.serverError(w, "Assign", err)
			return
		}
		rec.Oeuvre = dr
		api.RefreshOeuvres()
		if o, _ := api.store.Oeuvre(oid); o != nil {
			api.setReference(o)
			api.community.alert(target.ID, "claim_validee", map[string]any{"oeuvre": oid, "titre": titre, "badges": req.Badges, "pionniers": dr.Pioneer[target.ID], "direct": true})
			if o.Origine == "avant_migration" {
				api.community.Emit("claim_valid", target, o.X+o.W/2, o.Y+o.H/2, int(dr.Pioneer[target.ID]), fmt.Sprintf("oeuvre:%d", oid), titre)
			}
		}
		detail = fmt.Sprintf("« %s » attribuée à %s", titre, target.Pseudo)
	default:
		writeFieldError(w, &FieldError{"action", "Action inconnue."})
		return
	}
	if m := strings.TrimSpace(req.Motif); m != "" {
		detail += " · " + m
	}
	rj, _ := json.Marshal(rec)
	id := api.logAction(admin, "zone."+req.Action, fmt.Sprintf("zone:%d,%d,%d,%d", rec.X, rec.Y, rec.W, rec.H), string(rj), "", detail)
	writeJSON(w, map[string]any{"ok": true, "action_id": id})
}

// afterZoneChange recounts visible pixels and tells the clients to reload the area.
func (api *API) afterZoneChange(b image.Rectangle) {
	counts := map[uint32]int64{}
	api.canvas.mu.RLock()
	for _, id := range api.canvas.owners {
		if id != 0 {
			counts[id]++
		}
	}
	api.canvas.mu.RUnlock()
	api.store.SetVisibleCounts(counts)
	api.hub.Broadcast(mustJSON(map[string]any{"type": "zone", "x": b.Min.X, "y": b.Min.Y, "w": b.Dx(), "h": b.Dy()}))
}

func (api *API) undoZone(raw string) error {
	var rec zoneRecord
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		return err
	}
	if rec.Oeuvre != nil {
		if err := api.store.undoRecord(*rec.Oeuvre); err != nil {
			return err
		}
		api.RefreshOeuvres()
		return nil
	}
	prev, err := decodeChanges(rec.Before)
	if err != nil {
		return err
	}
	api.canvas.ApplyChanges(prev)
	api.afterZoneChange(image.Rect(rec.X, rec.Y, rec.X+rec.W, rec.Y+rec.H))
	return nil
}

// CreateOeuvreDirect gives a zone to a player without a claim (admin).
func (s *Store) CreateOeuvreDirect(u *User, m Mask, titre string, pixels int, a *ZoneAnalysis, badges []string) (*decisionRecord, int64, error) {
	now := s.now().UnixMilli()
	rec := &decisionRecord{Statuts: map[int64]string{}, Pioneer: map[uint32]int64{}, Badges: map[uint32][]string{}}
	var gold []string
	for _, b := range badges {
		if contains(goldBadges, b) && !contains(gold, b) {
			gold = append(gold, b)
		}
	}
	origine := "compte"
	var apparue, terminee any
	placed := 0
	aJSON := ""
	if a != nil {
		if contains(a.Badges, "pionnier") || contains(gold, "pionnier") {
			origine = "avant_migration"
		}
		if a.Appeared != 0 {
			apparue = a.Appeared
		}
		if a.Finished != 0 {
			terminee = a.Finished
		}
		placed = a.Placed
		b, _ := json.Marshal(a)
		aJSON = string(b)
	} else if contains(gold, "pionnier") {
		origine = "avant_migration"
	}
	bounds := m.Bounds(1<<14, 1<<14)
	maskJSON, _ := json.Marshal(m)
	tx, err := s.db.Begin()
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO oeuvres (titre, masque, x, y, w, h, pixels, origine, apparue_le, terminee_le, analyse_json, creee_le)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, titre, string(maskJSON), bounds.Min.X, bounds.Min.Y, bounds.Dx(), bounds.Dy(), pixels, origine, apparue, terminee, aJSON, now)
	if err != nil {
		return nil, 0, err
	}
	rec.Oeuvre, _ = res.LastInsertId()
	share := int64(0)
	if origine == "avant_migration" {
		share = int64(math.Round(float64(placed)))
	}
	if _, err := tx.Exec(`INSERT INTO oeuvre_auteurs (oeuvre_id, user_id, role, pixels_pionniers) VALUES (?, ?, 'auteur', ?)`, rec.Oeuvre, u.ID, share); err != nil {
		return nil, 0, err
	}
	if share > 0 {
		tx.Exec(`UPDATE users SET pixels_pionniers = pixels_pionniers + ? WHERE id = ?`, share, u.ID)
		rec.Pioneer[u.ID] = share
	}
	for _, b := range gold {
		isNew, err := grant(tx, u.ID, b, fmt.Sprintf("oeuvre:%d", rec.Oeuvre), titre, now)
		if err != nil {
			return nil, 0, err
		}
		if isNew {
			rec.Badges[u.ID] = append(rec.Badges[u.ID], b)
		}
	}
	earned, err := awardCounterBadges(tx, u.ID, now)
	if err != nil {
		return nil, 0, err
	}
	rec.Badges[u.ID] = append(rec.Badges[u.ID], earned...)
	return rec, rec.Oeuvre, tx.Commit()
}
