package place

// Phase 4 — community: live activity (pixels grouped by player and area), alerts when an
// artwork is retouched, likes (coups de cœur), exhibitions in the museum, leaderboards.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	log "github.com/sirupsen/logrus"
)

const (
	groupCell        = 32               // pixels in the same 32 px cell…
	groupWindow      = 60 * time.Second // …within a minute are one activity line
	retouchThreshold = 5                // pixels changed in someone else's artwork before an alert
	retouchWindow    = 30 * time.Minute
)

// Museum rooms (m-exposer). "coups-de-coeur" and "pionnieres" are computed.
var Salles = []struct{ ID, Label string }{
	{"coups-de-coeur", "Coups de cœur"}, {"pionnieres", "Pionnières"}, {"paysages", "Paysages"},
	{"personnages", "Personnages"}, {"clins-d-oeil", "Clins d'œil"}, {"drapeaux-logos", "Drapeaux & logos"},
	{"motifs", "Motifs"}, {"monuments", "Monuments"},
}

func salleOK(id string) bool {
	for _, s := range Salles {
		if s.ID == id && id != "coups-de-coeur" && id != "pionnieres" {
			return true
		}
	}
	return false
}

// Activity is one line of the live feed.
type Activity struct {
	ID    int64       `json:"id"`
	Kind  string      `json:"kind"` // pixels | retouche | claim | claim_valid | badge | exposition | musee | livre_or
	Who   *PublicUser `json:"who"`
	X     int         `json:"x"`
	Y     int         `json:"y"`
	N     int         `json:"n"`
	Ref   string      `json:"ref"`
	Texte string      `json:"texte"`
	TS    int64       `json:"ts"`
	uid   uint32
}

type Alert struct {
	ID      int64          `json:"id"`
	Kind    string         `json:"kind"` // retouche | claim_validee | claim_refusee | coauteur | musee
	Payload map[string]any `json:"payload"`
	Read    bool           `json:"lu"`
	TS      int64          `json:"ts"`
}

// Community keeps the in-memory state behind the feed and the alerts.
type Community struct {
	api *API

	mu       sync.Mutex
	groups   map[uint32]*pixelGroup
	retouch  map[[2]int64]*retouchCount // (artwork, player)
	authors  map[int64][]uint32         // artwork → authors
	dirty    map[int64]*Activity
	lastPush map[int64]time.Time
}

type pixelGroup struct {
	act    *Activity
	cx, cy int
	last   time.Time
}

type retouchCount struct {
	n        int
	last     time.Time
	alertIDs []int64
	actID    int64
}

func newCommunity(api *API) *Community {
	c := &Community{api: api, groups: map[uint32]*pixelGroup{}, retouch: map[[2]int64]*retouchCount{},
		authors: map[int64][]uint32{}, dirty: map[int64]*Activity{}, lastPush: map[int64]time.Time{}}
	return c
}

func (c *Community) refreshAuthors(os []*Oeuvre) {
	m := map[int64][]uint32{}
	for _, o := range os {
		for _, a := range o.Auteurs {
			m[o.ID] = append(m[o.ID], a.userID)
		}
	}
	c.mu.Lock()
	c.authors = m
	c.mu.Unlock()
}

func (s *Store) AddActivity(a *Activity) error {
	res, err := s.db.Exec(`INSERT INTO activity (kind, user_id, x, y, n, ref, texte, ts) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Kind, a.uid, a.X, a.Y, a.N, a.Ref, a.Texte, a.TS)
	if err == nil {
		a.ID, _ = res.LastInsertId()
	}
	return err
}

func (s *Store) Activities(since int64, before int64, kinds []string, limit int) ([]*Activity, error) {
	q := `SELECT id, kind, COALESCE(user_id, 0), COALESCE(x, 0), COALESCE(y, 0), n, ref, texte, ts FROM activity WHERE ts >= ?`
	args := []any{since}
	if before > 0 {
		q += ` AND id < ?`
		args = append(args, before)
	}
	if len(kinds) > 0 {
		q += ` AND kind IN (?` + strings.Repeat(",?", len(kinds)-1) + `)`
		for _, k := range kinds {
			args = append(args, k)
		}
	}
	rows, err := s.db.Query(q+` ORDER BY id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	var out []*Activity
	var uids []uint32
	for rows.Next() {
		a := &Activity{}
		if err := rows.Scan(&a.ID, &a.Kind, &a.uid, &a.X, &a.Y, &a.N, &a.Ref, &a.Texte, &a.TS); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, a)
		uids = append(uids, a.uid)
	}
	rows.Close()
	users, err := s.UsersByIDs(uids)
	for _, a := range out {
		a.Who = users[a.uid].Public()
	}
	return out, err
}

func (s *Store) AddAlert(uid uint32, kind string, payload map[string]any) (int64, error) {
	b, _ := json.Marshal(payload)
	res, err := s.db.Exec(`INSERT INTO alerts (user_id, kind, payload, ts) VALUES (?, ?, ?, ?)`, uid, kind, string(b), s.now().UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateAlertPayload(id int64, payload map[string]any) error {
	b, _ := json.Marshal(payload)
	_, err := s.db.Exec(`UPDATE alerts SET payload = ?, ts = ?, lu = 0 WHERE id = ?`, string(b), s.now().UnixMilli(), id)
	return err
}

func (s *Store) Alerts(uid uint32, limit int) ([]*Alert, error) {
	rows, err := s.db.Query(`SELECT id, kind, payload, lu, ts FROM alerts WHERE user_id = ? ORDER BY id DESC LIMIT ?`, uid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Alert{}
	for rows.Next() {
		a := &Alert{}
		var p string
		if err := rows.Scan(&a.ID, &a.Kind, &p, &a.Read, &a.TS); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(p), &a.Payload)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) UnreadAlerts(uid uint32) int {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE user_id = ? AND lu = 0`, uid).Scan(&n)
	return n
}

// push sends an activity line to everyone.
func (c *Community) push(a *Activity) {
	c.api.hub.Broadcast(mustJSON(struct {
		Type string `json:"type"`
		*Activity
	}{"activity", a}))
}

// Emit records and broadcasts an activity line.
func (c *Community) Emit(kind string, u *User, x, y, n int, ref, texte string) {
	a := &Activity{Kind: kind, X: x, Y: y, N: n, Ref: ref, Texte: texte, TS: c.api.store.now().UnixMilli()}
	if u != nil {
		a.uid, a.Who = u.ID, u.Public()
	}
	if err := c.api.store.AddActivity(a); err != nil {
		log.WithField("endpoint", "Activity").Error(err)
		return
	}
	c.push(a)
}

// alert stores an alert and pushes it to the player's sockets.
func (c *Community) alert(uid uint32, kind string, payload map[string]any) int64 {
	id, err := c.api.store.AddAlert(uid, kind, payload)
	if err != nil {
		log.WithField("endpoint", "Alerts").Error(err)
		return 0
	}
	msg := map[string]any{"type": "alert", "kind": kind, "id": id}
	for k, v := range payload {
		msg[k] = v
	}
	c.api.hub.SendToUser(uid, mustJSON(msg))
	return id
}

// onPixel groups pixels into activity lines and watches artworks being retouched.
func (c *Community) onPixel(u *User, e PixelEvent) {
	now := time.UnixMilli(e.TS)
	cx, cy := e.X/groupCell, e.Y/groupCell
	c.mu.Lock()
	g := c.groups[u.ID]
	if g != nil && g.cx == cx && g.cy == cy && now.Sub(g.last) < groupWindow {
		g.act.N++
		g.act.X, g.act.Y, g.act.TS = e.X, e.Y, e.TS
		g.last = now
		c.dirty[g.act.ID] = g.act
		c.mu.Unlock()
	} else {
		c.mu.Unlock()
		a := &Activity{Kind: "pixels", uid: u.ID, Who: u.Public(), X: e.X, Y: e.Y, N: 1, TS: e.TS}
		if err := c.api.store.AddActivity(a); err == nil {
			c.mu.Lock()
			c.groups[u.ID] = &pixelGroup{act: a, cx: cx, cy: cy, last: now}
			c.lastPush[a.ID] = now
			c.mu.Unlock()
			c.push(a)
		}
	}

	// Someone else's artwork?
	cw, _ := c.api.canvas.Size()
	oid := c.api.oeuvres.At(e.X, e.Y, cw)
	if oid == 0 {
		return
	}
	c.mu.Lock()
	authors := c.authors[oid]
	for _, a := range authors {
		if a == u.ID {
			c.mu.Unlock()
			return
		}
	}
	key := [2]int64{oid, int64(u.ID)}
	r := c.retouch[key]
	if r == nil || now.Sub(r.last) > retouchWindow {
		r = &retouchCount{}
		c.retouch[key] = r
	}
	r.n++
	r.last = now
	n := r.n
	first := n == retouchThreshold
	ids := append([]int64(nil), r.alertIDs...)
	c.mu.Unlock()
	if n < retouchThreshold {
		return
	}
	o, err := c.api.store.Oeuvre(oid)
	if err != nil || o == nil {
		return
	}
	payload := map[string]any{"oeuvre": oid, "titre": o.Titre, "by": u.Public(), "count": n, "x": o.X, "y": o.Y, "w": o.W, "h": o.H}
	if first {
		var newIDs []int64
		for _, a := range authors {
			au, _ := c.api.store.UserByID(a)
			if au != nil && au.RetouchAlerts {
				newIDs = append(newIDs, c.alert(a, "retouche", payload))
			}
		}
		act := &Activity{Kind: "retouche", uid: u.ID, Who: u.Public(), X: e.X, Y: e.Y, N: n, Ref: fmt.Sprintf("oeuvre:%d", oid), Texte: o.Titre, TS: e.TS}
		c.api.store.AddActivity(act)
		c.push(act)
		c.mu.Lock()
		r.alertIDs, r.actID = newIDs, act.ID
		c.mu.Unlock()
	} else if n%5 == 0 {
		for _, id := range ids {
			c.api.store.UpdateAlertPayload(id, payload)
		}
		c.mu.Lock()
		if r.actID != 0 {
			c.dirty[r.actID] = &Activity{ID: r.actID, Kind: "retouche", uid: u.ID, Who: u.Public(), X: e.X, Y: e.Y, N: n, Ref: fmt.Sprintf("oeuvre:%d", oid), Texte: o.Titre, TS: e.TS}
		}
		c.mu.Unlock()
	}
}

// flushLoop writes grouped counts and pushes updates every few seconds.
func (c *Community) flushLoop() {
	for range time.Tick(3 * time.Second) {
		c.mu.Lock()
		dirty := c.dirty
		c.dirty = map[int64]*Activity{}
		now := time.Now()
		for uid, g := range c.groups {
			if now.Sub(g.last) > groupWindow {
				delete(c.groups, uid)
				delete(c.lastPush, g.act.ID)
			}
		}
		for k, r := range c.retouch {
			if now.Sub(r.last) > retouchWindow {
				delete(c.retouch, k)
			}
		}
		c.mu.Unlock()
		for _, a := range dirty {
			c.api.store.db.Exec(`UPDATE activity SET n = ?, x = ?, y = ?, ts = ? WHERE id = ?`, a.N, a.X, a.Y, a.TS, a.ID)
			c.push(a)
		}
	}
}

// ------------------------------------------------------------------
// Likes, exhibitions, references

type Exposition struct {
	Salle     string `json:"salle"`
	Marge     int    `json:"marge"`
	Cadre     string `json:"cadre"`
	Mot       string `json:"mot"`
	Timelapse bool   `json:"timelapse"`
	Visite    bool   `json:"visite"`
	Par       uint32 `json:"-"`
	TS        int64  `json:"ts"`
}

func (s *Store) Expositions() (map[int64]*Exposition, error) {
	rows, err := s.db.Query(`SELECT oeuvre_id, salle, marge, cadre, mot, timelapse, visite, par, ts FROM expositions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[int64]*Exposition{}
	for rows.Next() {
		var id int64
		e := &Exposition{}
		if err := rows.Scan(&id, &e.Salle, &e.Marge, &e.Cadre, &e.Mot, &e.Timelapse, &e.Visite, &e.Par, &e.TS); err != nil {
			return nil, err
		}
		m[id] = e
	}
	return m, rows.Err()
}

// LikeCounts returns likes per artwork since a time (0 = all time).
func (s *Store) LikeCounts(since int64) (map[int64]int, error) {
	rows, err := s.db.Query(`SELECT oeuvre_id, COUNT(*) FROM likes WHERE ts >= ? GROUP BY oeuvre_id`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[int64]int{}
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		m[id] = n
	}
	return m, rows.Err()
}

func (s *Store) Liked(uid uint32) map[int64]bool {
	m := map[int64]bool{}
	rows, err := s.db.Query(`SELECT oeuvre_id FROM likes WHERE user_id = ?`, uid)
	if err != nil {
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		m[id] = true
	}
	return m
}

// setReference stores the artwork's current look (RGB of its bounding box): alerts and
// « Retoucher » compare the canvas with it.
func (api *API) setReference(o *Oeuvre) {
	cw, ch := api.canvas.Size()
	b := o.Mask.Bounds(cw, ch)
	buf := make([]byte, 0, b.Dx()*b.Dy()*3)
	api.canvas.mu.RLock()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			p := api.canvas.img.Pix[(y*cw+x)*4:]
			buf = append(buf, p[0], p[1], p[2])
		}
	}
	api.canvas.mu.RUnlock()
	api.store.db.Exec(`UPDATE oeuvres SET reference = ?, reference_le = ? WHERE id = ?`, buf, api.store.now().UnixMilli(), o.ID)
}

// intactPct compares an artwork with its reference.
func (api *API) intactPct(o *Oeuvre) (float64, bool) {
	var ref []byte
	if err := api.store.db.QueryRow(`SELECT reference FROM oeuvres WHERE id = ?`, o.ID).Scan(&ref); err != nil || len(ref) == 0 {
		return 100, false
	}
	cw, ch := api.canvas.Size()
	b := o.Mask.Bounds(cw, ch)
	if len(ref) != b.Dx()*b.Dy()*3 {
		return 100, false
	}
	pos := maskPositions(o.Mask, cw, ch)
	if len(pos) == 0 {
		return 100, false
	}
	same := 0
	api.canvas.mu.RLock()
	for _, p := range pos {
		x, y := int(p)%cw, int(p)/cw
		i := ((y-b.Min.Y)*b.Dx() + x - b.Min.X) * 3
		px := api.canvas.img.Pix[int(p)*4:]
		if px[0] == ref[i] && px[1] == ref[i+1] && px[2] == ref[i+2] {
			same++
		}
	}
	api.canvas.mu.RUnlock()
	return round1(float64(same) / float64(len(pos))), true
}

func (api *API) decorateOeuvres(os []*Oeuvre, viewer *User) {
	likes, _ := api.store.LikeCounts(0)
	expos, _ := api.store.Expositions()
	var liked map[int64]bool
	if viewer != nil {
		liked = api.store.Liked(viewer.ID)
	}
	for _, o := range os {
		if o.Extra == nil {
			o.Extra = map[string]any{}
		}
		o.Extra["likes"] = likes[o.ID]
		o.Extra["liked"] = liked[o.ID]
		if e, ok := expos[o.ID]; ok {
			o.Extra["exposition"] = e
		}
		pct, ok := api.intactPct(o)
		o.Extra["intact_pct"] = pct
		o.Extra["retouchee"] = ok && pct < 95
		o.Extra["thumb"] = cropURL(o.X, o.Y, o.W, o.H, 240)
	}
}

// ------------------------------------------------------------------
// API

func init() {
	OeuvreDecorators = append(OeuvreDecorators, func(api *API, os []*Oeuvre, viewer *User) { api.decorateOeuvres(os, viewer) })
	ClaimHooks = append(ClaimHooks, func(api *API, c *Claim, event string, by *User) {
		if api.community != nil {
			api.community.onClaim(api, c, event, by)
		}
	})
}

func (api *API) mountCommunity() {
	api.community = newCommunity(api)
	api.hub.OnPixel(api.community.onPixel)
	go api.community.flushLoop()
	api.hub.OnBadge(func(u *User, badge string) {
		if b := BadgeByID(badge); b != nil {
			api.community.Emit("badge", u, 0, 0, 1, "badge:"+badge, b.Name)
		}
	})
	api.mux.HandleFunc("/api/activity", api.handleActivity)
	api.mux.HandleFunc("/api/alerts", api.handleAlerts)
	api.mux.HandleFunc("/api/leaderboard", api.handleLeaderboard)
	api.mux.HandleFunc("/api/musee", api.handleMusee)
	api.mux.HandleFunc("/api/musee/visite", api.handleVisite)
	OeuvreRoutes["like"] = handleLike
	OeuvreRoutes["exposition"] = handleExposition
	OeuvreRoutes["keep"] = handleKeep
}

func (c *Community) onClaim(api *API, cl *Claim, event string, by *User) {
	if cl == nil {
		return
	}
	switch {
	case event == "created":
		c.Emit("claim", by, cl.X+cl.W/2, cl.Y+cl.H/2, cl.Pixels, fmt.Sprintf("claim:%d", cl.ID), cl.Titre)
		for _, ca := range cl.Coauthors {
			c.alert(ca.userID, "coauteur", map[string]any{"claim": cl.ID, "titre": cl.Titre, "by": by.Public()})
		}
	case event == "decided:valider" || event == "decided:fusionner" || event == "decided:attribuer":
		o, err := api.store.Oeuvre(cl.OeuvreID)
		if err != nil || o == nil {
			return
		}
		api.setReference(o)
		c.refreshAuthors([]*Oeuvre{o})
		os, _ := api.store.Oeuvres(0, false)
		c.refreshAuthors(os)
		for _, a := range o.Auteurs {
			u, _ := api.store.UserByID(a.userID)
			if u == nil {
				continue
			}
			kind := "claim_validee"
			c.alert(u.ID, kind, map[string]any{"claim": cl.ID, "oeuvre": o.ID, "titre": o.Titre, "badges": cl.Badges, "pionniers": a.Pioneer})
			if o.Origine == "avant_migration" {
				c.Emit("claim_valid", u, o.X+o.W/2, o.Y+o.H/2, int(a.Pioneer), fmt.Sprintf("oeuvre:%d", o.ID), o.Titre)
			}
		}
		if event == "decided:attribuer" {
			c.alert(cl.requesterID, "claim_refusee", map[string]any{"claim": cl.ID, "titre": cl.Titre, "motif": cl.Motif})
		}
	case event == "decided:refuser":
		c.alert(cl.requesterID, "claim_refusee", map[string]any{"claim": cl.ID, "titre": cl.Titre, "motif": cl.Motif})
	}
}

// GET /api/activity?kind=pixels,claim&before=<id>
func (api *API) handleActivity(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	var kinds []string
	if k := r.URL.Query().Get("kind"); k != "" {
		kinds = strings.Split(k, ",")
	}
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	items, err := api.store.Activities(0, before, kinds, 60)
	if err != nil {
		api.serverError(w, "Activity", err)
		return
	}
	if items == nil {
		items = []*Activity{}
	}
	// Where people draw right now: the pixel lines of the last 10 minutes.
	recent, _ := api.store.Activities(api.store.now().Add(-10*time.Minute).UnixMilli(), 0, []string{"pixels"}, 20)
	hot := []map[string]any{}
	for _, a := range recent {
		hot = append(hot, map[string]any{"x": a.X, "y": a.Y, "n": a.N, "who": a.Who})
	}
	writeJSON(w, map[string]any{"items": items, "hot": hot})
}

// GET /api/alerts, POST /api/alerts (mark all as read)
func (api *API) handleAlerts(w http.ResponseWriter, r *http.Request) {
	u := api.auth.User(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "Connecte-toi.")
		return
	}
	switch r.Method {
	case http.MethodGet:
		alerts, err := api.store.Alerts(u.ID, 50)
		if err != nil {
			api.serverError(w, "Alerts", err)
			return
		}
		writeJSON(w, map[string]any{"alerts": alerts, "unread": api.store.UnreadAlerts(u.ID), "settings": map[string]bool{"retouche": u.RetouchAlerts}})
	case http.MethodPost:
		api.store.db.Exec(`UPDATE alerts SET lu = 1 WHERE user_id = ?`, u.ID)
		writeJSON(w, map[string]any{"unread": 0})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
	}
}

type boardEntry struct {
	Rank   int         `json:"rank"`
	User   *PublicUser `json:"user,omitempty"`
	Oeuvre *Oeuvre     `json:"oeuvre,omitempty"`
	Value  int64       `json:"value"`
}

// GET /api/leaderboard?kind=joueurs|oeuvres&period=jour|semaine|tout
func (api *API) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	q := r.URL.Query()
	now := api.store.now()
	var from int64
	switch q.Get("period") {
	case "jour":
		from = DayStart(now).UnixMilli()
	case "semaine":
		from = WeekStart(now).UnixMilli()
	}
	viewer := api.auth.User(r)
	res := map[string]any{"entries": []boardEntry{}}
	if q.Get("kind") == "oeuvres" {
		likes, err := api.store.LikeCounts(from)
		if err != nil {
			api.serverError(w, "Leaderboard", err)
			return
		}
		os, _ := api.store.Oeuvres(0, false)
		api.decorateOeuvres(os, viewer)
		sort.SliceStable(os, func(i, j int) bool { return likes[os[i].ID] > likes[os[j].ID] })
		var out []boardEntry
		for i, o := range os {
			if likes[o.ID] == 0 || i >= 30 {
				break
			}
			out = append(out, boardEntry{Rank: i + 1, Oeuvre: o, Value: int64(likes[o.ID])})
		}
		if out != nil {
			res["entries"] = out
		}
		writeJSON(w, res)
		return
	}
	var ranks []RankEntry
	var err error
	if from == 0 {
		ranks, err = api.store.RankAllTime(50)
	} else {
		ranks, err = api.store.RankSince(from, 0, 50)
	}
	if err != nil {
		api.serverError(w, "Leaderboard", err)
		return
	}
	ids := make([]uint32, len(ranks))
	for i, e := range ranks {
		ids[i] = e.UserID
	}
	users, _ := api.store.UsersByIDs(ids)
	var out []boardEntry
	for _, e := range ranks {
		out = append(out, boardEntry{Rank: e.Rank, User: users[e.UserID].Public(), Value: e.Pixels})
	}
	if out != nil {
		res["entries"] = out
	}
	if viewer != nil {
		var rank int
		var mine int64
		if from == 0 {
			rank, _ = api.store.UserRankAllTime(viewer.ID)
			mine = viewer.PixelsPlaced + viewer.PixelsPioneer
		} else {
			rank, _ = api.store.UserRank(viewer.ID, from)
			api.store.db.QueryRow(`SELECT COUNT(*) FROM pixel_events WHERE user_id = ? AND ts >= ?`, viewer.ID, from).Scan(&mine)
		}
		res["moi"] = map[string]any{"rank": rank, "value": mine, "user": viewer.Public()}
	}
	writeJSON(w, res)
}

// GET /api/musee: the artwork on show, the rooms and the exhibited artworks.
func (api *API) handleMusee(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	viewer := api.auth.User(r)
	os, err := api.store.Oeuvres(0, false)
	if err != nil {
		api.serverError(w, "Musee", err)
		return
	}
	api.decorateOeuvres(os, viewer)
	week, _ := api.store.LikeCounts(api.store.now().AddDate(0, 0, -7).UnixMilli())
	var exposed []*Oeuvre
	for _, o := range os {
		if _, ok := o.Extra["exposition"]; ok {
			exposed = append(exposed, o)
		}
	}
	var affiche *Oeuvre
	for _, o := range exposed {
		if affiche == nil || week[o.ID] > week[affiche.ID] ||
			week[o.ID] == week[affiche.ID] && o.Extra["likes"].(int) > affiche.Extra["likes"].(int) {
			affiche = o
		}
	}
	counts := map[string]int{}
	for _, o := range exposed {
		counts[o.Extra["exposition"].(*Exposition).Salle]++
		if o.Origine == "avant_migration" {
			counts["pionnieres"]++
		}
		if o.Extra["likes"].(int) > 0 {
			counts["coups-de-coeur"]++
		}
	}
	salles := []map[string]any{}
	for _, s := range Salles {
		salles = append(salles, map[string]any{"id": s.ID, "label": s.Label, "n": counts[s.ID]})
	}
	var mine []*Oeuvre
	if viewer != nil {
		for _, o := range os {
			for _, a := range o.Auteurs {
				if a.userID == viewer.ID {
					mine = append(mine, o)
				}
			}
		}
	}
	if exposed == nil {
		exposed = []*Oeuvre{}
	}
	if mine == nil {
		mine = []*Oeuvre{}
	}
	res := map[string]any{"affiche": affiche, "salles": salles, "oeuvres": exposed, "miennes": mine}
	for _, f := range MuseumExtras {
		k, v := f(api, viewer)
		res[k] = v
	}
	writeJSON(w, res)
}

// MuseumExtras add sections to /api/musee (players' museums, phase 6).
var MuseumExtras []func(api *API, viewer *User) (string, any)

// GET /api/musee/visite: the artworks of the guided tour.
func (api *API) handleVisite(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	os, err := api.store.Oeuvres(0, false)
	if err != nil {
		api.serverError(w, "Visite", err)
		return
	}
	api.decorateOeuvres(os, api.auth.User(r))
	var tour []*Oeuvre
	for _, o := range os {
		if e, ok := o.Extra["exposition"].(*Exposition); ok && e.Visite {
			tour = append(tour, o)
		}
	}
	sort.SliceStable(tour, func(i, j int) bool { return tour[i].Extra["likes"].(int) > tour[j].Extra["likes"].(int) })
	if tour == nil {
		tour = []*Oeuvre{}
	}
	cw, ch := api.canvas.Size()
	writeJSON(w, map[string]any{"oeuvres": tour, "width": cw, "height": ch})
}

func isAuthor(o *Oeuvre, u *User) bool {
	if u == nil {
		return false
	}
	for _, a := range o.Auteurs {
		if a.userID == u.ID {
			return true
		}
	}
	return false
}

// POST /api/oeuvres/:id/like {on}
func handleLike(api *API, w http.ResponseWriter, r *http.Request, o *Oeuvre) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
		return
	}
	u := api.auth.User(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "Connecte-toi pour donner un coup de cœur.")
		return
	}
	var req struct {
		On bool `json:"on"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.On {
		api.store.db.Exec(`INSERT OR IGNORE INTO likes (user_id, oeuvre_id, ts) VALUES (?, ?, ?)`, u.ID, o.ID, api.store.now().UnixMilli())
	} else {
		api.store.db.Exec(`DELETE FROM likes WHERE user_id = ? AND oeuvre_id = ?`, u.ID, o.ID)
	}
	var n int
	api.store.db.QueryRow(`SELECT COUNT(*) FROM likes WHERE oeuvre_id = ?`, o.ID).Scan(&n)
	writeJSON(w, map[string]any{"likes": n, "liked": req.On})
}

// POST /api/oeuvres/:id/exposition, DELETE to take it down.
func handleExposition(api *API, w http.ResponseWriter, r *http.Request, o *Oeuvre) {
	u := api.auth.User(r)
	if u == nil || !isAuthor(o, u) && u.Role != "admin" {
		writeError(w, http.StatusForbidden, "Seuls les auteurs peuvent exposer cette œuvre.")
		return
	}
	switch r.Method {
	case http.MethodDelete:
		api.store.db.Exec(`DELETE FROM expositions WHERE oeuvre_id = ?`, o.ID)
		writeJSON(w, map[string]any{"ok": true})
	case http.MethodPost:
		var e Exposition
		if !readJSON(w, r, &e) {
			return
		}
		if !salleOK(e.Salle) {
			writeFieldError(w, &FieldError{"salle", "Choisis une salle."})
			return
		}
		if e.Cadre != "or" && e.Cadre != "sans" {
			e.Cadre = "bois"
		}
		e.Marge = max(50, min(100, e.Marge))
		e.Mot = strings.TrimSpace(e.Mot)
		if utf8.RuneCountInString(e.Mot) > 160 {
			writeFieldError(w, &FieldError{"mot", "160 caractères au maximum."})
			return
		}
		var existed int
		api.store.db.QueryRow(`SELECT COUNT(*) FROM expositions WHERE oeuvre_id = ?`, o.ID).Scan(&existed)
		_, err := api.store.db.Exec(`INSERT INTO expositions (oeuvre_id, salle, marge, cadre, mot, timelapse, visite, par, ts) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (oeuvre_id) DO UPDATE SET salle = excluded.salle, marge = excluded.marge, cadre = excluded.cadre, mot = excluded.mot,
			timelapse = excluded.timelapse, visite = excluded.visite`,
			o.ID, e.Salle, e.Marge, e.Cadre, e.Mot, e.Timelapse, e.Visite, u.ID, api.store.now().UnixMilli())
		if err != nil {
			api.serverError(w, "Exposition", err)
			return
		}
		if existed == 0 {
			api.community.Emit("exposition", u, o.X+o.W/2, o.Y+o.H/2, 1, fmt.Sprintf("oeuvre:%d", o.ID), o.Titre)
		}
		for _, f := range ProfileHooks {
			f(api, u, "exposition")
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
	}
}

// POST /api/oeuvres/:id/keep — « Je garde, c'est drôle »: the artwork now includes the change.
func handleKeep(api *API, w http.ResponseWriter, r *http.Request, o *Oeuvre) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
		return
	}
	u := api.auth.User(r)
	if !isAuthor(o, u) {
		writeError(w, http.StatusForbidden, "Seuls les auteurs peuvent faire ce choix.")
		return
	}
	api.setReference(o)
	writeJSON(w, map[string]any{"ok": true})
}

var _ = sql.ErrNoRows
