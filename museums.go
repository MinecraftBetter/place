package place

// Players' museums (bonus, HANDOFF §4.2): a museum per player with its ambience,
// rooms and artworks, a guestbook, likes, and sounds checked by the team.

import (
	"bytes"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// The choices of the editor; the first one is the default.
var museeChoices = map[string][]string{
	"theme":      {"maree", "saloon", "galerie", "etoiles", "desert"},
	"eclairage":  {"spots", "tamise", "jour"},
	"particules": {"bulles", "aucune", "etoiles", "sable", "neige", "confettis"},
	"cadre":      {"or", "bois", "pixel", "aucun"},
	"musique":    {"vagues", "saloon", "desert", "silence"},
	"entree":     {"rideau", "camera", "fondu"},
	"parcours":   {"libre", "guide"},
	"guide":      {"cowboy", "avatar", "aucun"},
	"visibilite": {"tous", "connectes", "moi"},
	"animation":  {"aucune", "construction", "zoom", "scintille"},
	"son":        {"aucun", "vague", "clochette", "piece", "mouette"},
	"taille":     {"moyenne", "petite", "grande"},
	"cadre_oe":   {"musee", "or", "bois", "pixel", "aucun"},
	"spot":       {"#fff8b8", "#5eb3ff", "#ff63aa", "#7eed38", "#ffa800", "#e4abff"},
}

var themeAccents = map[string]string{"maree": "#5eb3ff", "saloon": "#f0b75a", "galerie": "#13793a", "etoiles": "#e4abff", "desert": "#6d302f"}
var themeWalls = map[string]string{
	"maree": "linear-gradient(180deg, #0a2340, #1c4f7c)", "saloon": "repeating-linear-gradient(90deg, #5a3218 0 14px, #6d3d1e 14px 16px)",
	"galerie": "linear-gradient(180deg, #f4f1ea, #e6e0d3)", "etoiles": "linear-gradient(180deg, #120b2a, #26164f)", "desert": "linear-gradient(180deg, #ff9d5c, #ffd623 70%)",
}
var musicLabels = map[string]string{"vagues": "Vagues 8-bit", "saloon": "Saloon au piano", "desert": "Désert de nuit", "silence": ""}

const (
	maxSalles       = 6
	maxSalleOeuvres = 12
	maxSons         = 20
)

type Musee struct {
	Nom         string        `json:"nom"`
	Theme       string        `json:"theme"`
	Mur         string        `json:"mur"` // "" = the theme's wall, else a palette colour
	Eclairage   string        `json:"eclairage"`
	Particules  string        `json:"particules"`
	Cadre       string        `json:"cadre"`
	Musique     string        `json:"musique"` // built-in tune, or "son:<id>"
	Volume      int           `json:"volume"`
	VoixAuto    bool          `json:"voix_auto"`
	SonsPassage bool          `json:"sons_passage"`
	Pas         bool          `json:"pas"`
	Entree      string        `json:"entree"`
	Parcours    string        `json:"parcours"`
	Guide       string        `json:"guide"`
	PhraseGuide string        `json:"phrase_guide"`
	Accueil     string        `json:"accueil"`
	Visibilite  string        `json:"visibilite"`
	LivreOr     bool          `json:"livre_or"`
	Salles      []*MuseeSalle `json:"salles"`
}

type MuseeSalle struct {
	Titre   string         `json:"titre"`
	Oeuvres []*MuseeOeuvre `json:"oeuvres"`
}

type MuseeOeuvre struct {
	OeuvreID  int64   `json:"oeuvre_id"`
	Animation string  `json:"animation"`
	Son       string  `json:"son"` // built-in sound, or "son:<id>"
	VoixID    int64   `json:"voix_id"`
	Texte     string  `json:"texte"`
	Taille    string  `json:"taille"`
	Cadre     string  `json:"cadre"`
	Spot      string  `json:"spot"`
	Vedette   bool    `json:"vedette"`
	Oeuvre    *Oeuvre `json:"oeuvre,omitempty"` // filled for visits and the editor
	Voix      *Son    `json:"voix,omitempty"`
	SonFile   *Son    `json:"son_fichier,omitempty"`
}

type Son struct {
	ID     int64   `json:"id"`
	URL    string  `json:"url"`
	Nom    string  `json:"nom"`
	Type   string  `json:"type"` // musique | voix | passage
	Duree  float64 `json:"duree"`
	Statut string  `json:"statut"` // attente | ok | refuse
	uid    uint32
}

func defaultMusee(u *User) *Musee {
	return &Musee{
		Nom: "Le musée de " + u.Pseudo, Theme: "maree", Eclairage: "spots", Particules: "bulles", Cadre: "or", Musique: "vagues",
		Volume: 60, VoixAuto: true, SonsPassage: true, Entree: "rideau", Parcours: "libre", Guide: "cowboy",
		PhraseGuide: "Howdy ! Suis-moi, les œuvres c'est par là.", Visibilite: "tous", LivreOr: true,
		Salles: []*MuseeSalle{{Titre: "Salle 1 · Mes œuvres"}},
	}
}

func pick(kind, v string) string {
	for _, c := range museeChoices[kind] {
		if c == v {
			return v
		}
	}
	return museeChoices[kind][0]
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > n {
		s = string([]rune(s)[:n])
	}
	return s
}

var sonRefRE = regexp.MustCompile(`^son:(\d+)$`)

// sanitize keeps the known choices, trims texts and drops what the player doesn't own.
func (api *API) sanitizeMusee(u *User, m *Musee) *Musee {
	d := defaultMusee(u)
	m.Nom = clip(m.Nom, 40)
	if m.Nom == "" {
		m.Nom = d.Nom
	}
	m.Theme = pick("theme", m.Theme)
	if !isPaletteHex(m.Mur) {
		m.Mur = ""
	}
	m.Eclairage = pick("eclairage", m.Eclairage)
	m.Particules = pick("particules", m.Particules)
	m.Cadre = pick("cadre", m.Cadre)
	mySons := api.store.sonsOf(u.ID)
	ownSon := func(ref, typ string) bool {
		mm := sonRefRE.FindStringSubmatch(ref)
		if mm == nil {
			return false
		}
		id, _ := strconv.ParseInt(mm[1], 10, 64)
		s := mySons[id]
		return s != nil && s.Type == typ && s.Statut != "refuse"
	}
	if !ownSon(m.Musique, "musique") {
		m.Musique = pick("musique", m.Musique)
	}
	m.Volume = max(0, min(100, m.Volume))
	m.Entree = pick("entree", m.Entree)
	m.Parcours = pick("parcours", m.Parcours)
	m.Guide = pick("guide", m.Guide)
	m.PhraseGuide = clip(m.PhraseGuide, 120)
	m.Accueil = clip(m.Accueil, 160)
	m.Visibilite = pick("visibilite", m.Visibilite)
	if len(m.Salles) == 0 {
		m.Salles = d.Salles
	}
	if len(m.Salles) > maxSalles {
		m.Salles = m.Salles[:maxSalles]
	}
	for i, s := range m.Salles {
		s.Titre = clip(s.Titre, 32)
		if s.Titre == "" {
			s.Titre = fmt.Sprintf("Salle %d", i+1)
		}
		var keep []*MuseeOeuvre
		seen := map[int64]bool{}
		for _, w := range s.Oeuvres {
			if len(keep) >= maxSalleOeuvres || w == nil || seen[w.OeuvreID] {
				continue
			}
			if o, _ := api.store.Oeuvre(w.OeuvreID); o == nil || o.Statut != "active" {
				continue
			}
			seen[w.OeuvreID] = true
			w.Animation = pick("animation", w.Animation)
			if !ownSon(w.Son, "passage") {
				w.Son = pick("son", w.Son)
			}
			if v := mySons[w.VoixID]; v == nil || v.Type != "voix" || v.Statut == "refuse" {
				w.VoixID = 0
			}
			w.Texte = clip(w.Texte, 200)
			w.Taille = pick("taille", w.Taille)
			w.Cadre = pick("cadre_oe", w.Cadre)
			w.Spot = pick("spot", w.Spot)
			w.Oeuvre, w.Voix, w.SonFile = nil, nil, nil
			keep = append(keep, w)
		}
		if keep == nil {
			keep = []*MuseeOeuvre{}
		}
		s.Oeuvres = keep
	}
	return m
}

func isPaletteHex(h string) bool {
	return h != "" && inList(strings.ToLower(h), paletteHex)
}

// ------------------------------------------------------------------
// Store

type museeRow struct {
	uid      uint32
	m        *Musee
	publie   bool
	visites  int
	creeLe   int64
	publieLe int64
}

func (s *Store) Musee(uid uint32) (*museeRow, error) {
	var raw string
	var pl sql.NullInt64
	r := &museeRow{uid: uid}
	err := s.db.QueryRow(`SELECT config, publie, visites, cree_le, publie_le FROM musees WHERE user_id = ?`, uid).Scan(&raw, &r.publie, &r.visites, &r.creeLe, &pl)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.publieLe = pl.Int64
	r.m = &Musee{}
	if err := json.Unmarshal([]byte(raw), r.m); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Store) SaveMusee(uid uint32, m *Musee, publie bool) (firstPublish bool, err error) {
	raw, _ := json.Marshal(m)
	now := s.now().UnixMilli()
	var was sql.NullInt64
	s.db.QueryRow(`SELECT publie_le FROM musees WHERE user_id = ?`, uid).Scan(&was)
	_, err = s.db.Exec(`INSERT INTO musees (user_id, config, publie, cree_le, maj_le, publie_le) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET config = excluded.config, publie = excluded.publie, maj_le = excluded.maj_le,
		publie_le = COALESCE(musees.publie_le, excluded.publie_le)`, uid, string(raw), publie, now, now, nullIf(publie, now))
	return publie && !was.Valid, err
}

func nullIf(ok bool, v int64) any {
	if ok {
		return v
	}
	return nil
}

func (s *Store) museeRows() ([]*museeRow, error) {
	rows, err := s.db.Query(`SELECT user_id, config, publie, visites, cree_le, COALESCE(publie_le, 0) FROM musees WHERE publie = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*museeRow
	for rows.Next() {
		r := &museeRow{m: &Musee{}}
		var raw string
		if err := rows.Scan(&r.uid, &raw, &r.publie, &r.visites, &r.creeLe, &r.publieLe); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(raw), r.m) == nil {
			out = append(out, r)
		}
	}
	return out, rows.Err()
}

func (s *Store) museeLikes() map[uint32]int {
	res := map[uint32]int{}
	rows, err := s.db.Query(`SELECT musee_id, COUNT(*) FROM musee_likes GROUP BY musee_id`)
	if err != nil {
		return res
	}
	defer rows.Close()
	for rows.Next() {
		var id uint32
		var n int
		rows.Scan(&id, &n)
		res[id] = n
	}
	return res
}

func (s *Store) sonsOf(uid uint32) map[int64]*Son {
	res := map[int64]*Son{}
	rows, err := s.db.Query(`SELECT id, url, nom, type, duree, statut FROM sons WHERE user_id = ?`, uid)
	if err != nil {
		return res
	}
	defer rows.Close()
	for rows.Next() {
		so := &Son{uid: uid}
		rows.Scan(&so.ID, &so.URL, &so.Nom, &so.Type, &so.Duree, &so.Statut)
		res[so.ID] = so
	}
	return res
}

func (s *Store) Son(id int64) *Son {
	so := &Son{}
	if err := s.db.QueryRow(`SELECT id, user_id, url, nom, type, duree, statut FROM sons WHERE id = ?`, id).Scan(&so.ID, &so.uid, &so.URL, &so.Nom, &so.Type, &so.Duree, &so.Statut); err != nil {
		return nil
	}
	return so
}

type guestEntry struct {
	ID     int64       `json:"id"`
	Auteur *PublicUser `json:"auteur"`
	Texte  string      `json:"texte"`
	TS     int64       `json:"ts"`
	aid    uint32
}

func (s *Store) guestbook(musee uint32, limit int) ([]*guestEntry, int) {
	var total int
	s.db.QueryRow(`SELECT COUNT(*) FROM livre_or WHERE musee_id = ? AND masque = 0`, musee).Scan(&total)
	rows, err := s.db.Query(`SELECT id, auteur_id, texte, ts FROM livre_or WHERE musee_id = ? AND masque = 0 ORDER BY id DESC LIMIT ?`, musee, limit)
	out := []*guestEntry{}
	if err != nil {
		return out, total
	}
	var ids []uint32
	for rows.Next() {
		e := &guestEntry{}
		rows.Scan(&e.ID, &e.aid, &e.Texte, &e.TS)
		out = append(out, e)
		ids = append(ids, e.aid)
	}
	rows.Close()
	users, _ := s.UsersByIDs(ids)
	for _, e := range out {
		e.Auteur = users[e.aid].Public()
	}
	return out, total
}

// ------------------------------------------------------------------
// Hooks: profile, public museum page, moderation

func init() {
	ProfileExtras = append(ProfileExtras, func(api *API, u *User, viewer *User) (string, any) {
		r, _ := api.store.Musee(u.ID)
		if r == nil || !r.publie || !canVisit(r, u, viewer) {
			return "musee", nil
		}
		return "musee", map[string]any{"nom": r.m.Nom, "theme": r.m.Theme, "visites": r.visites}
	})
	MuseumExtras = append(MuseumExtras, func(api *API, viewer *User) (string, any) {
		list := api.museeList(viewer, "populaires", "")
		out := []map[string]any{}
		for _, m := range list {
			if len(out) == 8 {
				break
			}
			sub := fmt.Sprintf("%d œuvre%s · %d visite%s", m["n"], plural(m["n"].(int)), m["visites"], plural(m["visites"].(int)))
			out = append(out, map[string]any{"user": m["user"], "nom": m["nom"], "fond": m["fond"], "accent": m["accent"], "musique": m["musique"], "sub": sub})
		}
		return "musees", out
	})
	ReportTargets["musee"] = func(api *API, rep *Report, id int64) bool {
		r, _ := api.store.Musee(uint32(id))
		if r == nil {
			return false
		}
		rep.uid = uint32(id)
		rep.Contenu = strings.TrimSpace(r.m.Nom + " · " + r.m.Accueil + " · " + r.m.PhraseGuide)
		return true
	}
	ReportTargets["livre_or"] = func(api *API, rep *Report, id int64) bool {
		var aid uint32
		var texte string
		if api.store.db.QueryRow(`SELECT auteur_id, texte FROM livre_or WHERE id = ?`, id).Scan(&aid, &texte) != nil {
			return false
		}
		rep.uid, rep.Contenu = aid, texte
		return true
	}
	ReportTargets["son"] = func(api *API, rep *Report, id int64) bool {
		so := api.store.Son(id)
		if so == nil {
			return false
		}
		rep.uid, rep.Contenu = so.uid, so.URL
		return true
	}
	HideHandlers["musee"] = func(api *API, rep *Report, rec moderationRecord) (moderationRecord, error) {
		uid := uint32(idOf(rep.Cible))
		r, _ := api.store.Musee(uid)
		u, _ := api.store.UserByID(uid)
		if r == nil || u == nil {
			return rec, fmt.Errorf("museum not found")
		}
		old, _ := json.Marshal(map[string]string{"nom": r.m.Nom, "accueil": r.m.Accueil, "phrase": r.m.PhraseGuide})
		d := defaultMusee(u)
		r.m.Nom, r.m.Accueil, r.m.PhraseGuide = d.Nom, "", d.PhraseGuide
		_, err := api.store.SaveMusee(uid, r.m, r.publie)
		rec.Field, rec.Value = "musee_textes", string(old)
		return rec, err
	}
	UnhideHandlers["musee_textes"] = func(api *API, rec moderationRecord, cible string) error {
		uid := uint32(idOf(cible))
		r, _ := api.store.Musee(uid)
		if r == nil {
			return nil
		}
		var old map[string]string
		json.Unmarshal([]byte(rec.Value), &old)
		r.m.Nom, r.m.Accueil, r.m.PhraseGuide = old["nom"], old["accueil"], old["phrase"]
		_, err := api.store.SaveMusee(uid, r.m, r.publie)
		return err
	}
	HideHandlers["livre_or"] = func(api *API, rep *Report, rec moderationRecord) (moderationRecord, error) {
		_, err := api.store.db.Exec(`UPDATE livre_or SET masque = 1 WHERE id = ?`, idOf(rep.Cible))
		rec.Field = "livre_or"
		return rec, err
	}
	UnhideHandlers["livre_or"] = func(api *API, rec moderationRecord, cible string) error {
		_, err := api.store.db.Exec(`UPDATE livre_or SET masque = 0 WHERE id = ?`, idOf(cible))
		return err
	}
	// sounds: « masquer » refuses them, « garder » accepts them
	HideHandlers["son"] = func(api *API, rep *Report, rec moderationRecord) (moderationRecord, error) {
		so := api.store.Son(idOf(rep.Cible))
		if so == nil {
			return rec, fmt.Errorf("sound not found")
		}
		_, err := api.store.db.Exec(`UPDATE sons SET statut = 'refuse' WHERE id = ?`, so.ID)
		rec.Field, rec.Value = "son_statut", so.Statut
		return rec, err
	}
	UnhideHandlers["son_statut"] = func(api *API, rec moderationRecord, cible string) error {
		_, err := api.store.db.Exec(`UPDATE sons SET statut = ? WHERE id = ?`, rec.Value, idOf(cible))
		return err
	}
	KeepHandlers["son"] = func(api *API, rep *Report) (string, string) {
		so := api.store.Son(idOf(rep.Cible))
		if so == nil {
			return "", ""
		}
		api.store.db.Exec(`UPDATE sons SET statut = 'ok' WHERE id = ?`, so.ID)
		return "son_statut", so.Statut
	}
}

func idOf(cible string) int64 {
	_, s, _ := strings.Cut(cible, ":")
	id, _ := strconv.ParseInt(s, 10, 64)
	return id
}

func canVisit(r *museeRow, owner, viewer *User) bool {
	if viewer != nil && (viewer.ID == owner.ID || viewer.Role == "admin") {
		return true
	}
	switch r.m.Visibilite {
	case "moi":
		return false
	case "connectes":
		return viewer != nil
	}
	return true
}

// ------------------------------------------------------------------
// API

type museumAPI struct {
	mu     sync.Mutex
	visits map[string]bool // "museum:viewer:day", to count a visit once a day
}

func (api *API) mountMuseums() {
	api.museums = &museumAPI{visits: map[string]bool{}}
	api.mux.HandleFunc("/api/musees", api.handleMusees)
	api.mux.HandleFunc("/api/musees/", api.handleMuseeVisit)
	api.mux.HandleFunc("/api/me/musee", api.handleMyMusee)
	api.mux.HandleFunc("/api/me/musee/sons", api.handleMySons)
}

func museeCard(m *Musee) (fond, accent string) {
	wall := themeWalls[m.Theme]
	if m.Mur != "" {
		wall = "linear-gradient(180deg, " + m.Mur + ", " + m.Mur + "d0)"
	}
	return wall, themeAccents[m.Theme]
}

func museeCount(m *Musee) (n int) {
	for _, s := range m.Salles {
		n += len(s.Oeuvres)
	}
	return n
}

// museeList: the published museums the viewer may visit (tri: populaires | nouveaux | coeur).
func (api *API) museeList(viewer *User, tri, q string) []map[string]any {
	rows, _ := api.store.museeRows()
	var ids []uint32
	for _, r := range rows {
		ids = append(ids, r.uid)
	}
	users, _ := api.store.UsersByIDs(ids)
	likes := api.store.museeLikes()
	liked := map[uint32]bool{}
	if viewer != nil {
		lr, err := api.store.db.Query(`SELECT musee_id FROM musee_likes WHERE user_id = ?`, viewer.ID)
		if err == nil {
			for lr.Next() {
				var id uint32
				lr.Scan(&id)
				liked[id] = true
			}
			lr.Close()
		}
	}
	q = strings.ToLower(strings.TrimSpace(q))
	week := api.store.now().Add(-7 * 24 * time.Hour).UnixMilli()
	var out []map[string]any
	for _, r := range rows {
		u := users[r.uid]
		if u == nil || !canVisit(r, u, viewer) || museeCount(r.m) == 0 && (viewer == nil || viewer.ID != u.ID) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(r.m.Nom+" "+u.Pseudo), q) {
			continue
		}
		if tri == "coeur" && !liked[r.uid] {
			continue
		}
		fond, accent := museeCard(r.m)
		var apercu *Oeuvre
		for _, s := range r.m.Salles {
			for _, w := range s.Oeuvres {
				if apercu == nil || w.Vedette {
					if o, _ := api.store.Oeuvre(w.OeuvreID); o != nil {
						apercu = o
					}
				}
			}
		}
		card := map[string]any{
			"user": u.Public(), "nom": r.m.Nom, "theme": r.m.Theme, "fond": fond, "accent": accent, "particules": r.m.Particules,
			"cadre": r.m.Cadre, "musique": musicLabels[r.m.Musique], "n": museeCount(r.m), "salles": len(r.m.Salles),
			"visites": r.visites, "likes": likes[r.uid], "liked": liked[r.uid], "nouveau": r.publieLe > week, "publie_le": r.publieLe,
		}
		if strings.HasPrefix(r.m.Musique, "son:") {
			card["musique"] = "Musique perso"
		}
		if apercu != nil {
			card["apercu"] = map[string]int{"x": apercu.X, "y": apercu.Y, "w": apercu.W, "h": apercu.H}
		}
		out = append(out, card)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if tri == "nouveaux" {
			return out[i]["publie_le"].(int64) > out[j]["publie_le"].(int64)
		}
		si := out[i]["visites"].(int) + 3*out[i]["likes"].(int)
		sj := out[j]["visites"].(int) + 3*out[j]["likes"].(int)
		return si > sj
	})
	if out == nil {
		out = []map[string]any{}
	}
	return out
}

// GET /api/musees?tri=&q= — the directory; GET /api/musees?hasard=1 — a random museum.
func (api *API) handleMusees(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	viewer := api.auth.User(r)
	list := api.museeList(viewer, r.URL.Query().Get("tri"), r.URL.Query().Get("q"))
	res := map[string]any{"musees": list}
	if r.URL.Query().Get("hasard") != "" {
		var pool []map[string]any
		for _, m := range list {
			if viewer == nil || m["user"].(*PublicUser).ID != viewer.ID {
				pool = append(pool, m)
			}
		}
		if len(pool) > 0 {
			res["hasard"] = pool[rand.Intn(len(pool))]["user"]
		}
	}
	if viewer != nil {
		if mr, _ := api.store.Musee(viewer.ID); mr != nil {
			_, total := api.store.guestbook(viewer.ID, 0)
			res["moi"] = map[string]any{"nom": mr.m.Nom, "theme": mr.m.Theme, "publie": mr.publie, "n": museeCount(mr.m), "visites": mr.visites, "mots": total}
		}
	}
	writeJSON(w, res)
}

// fill loads the artworks and sounds of a museum; hidden sounds only for their owner.
func (api *API) fillMusee(m *Musee, owner uint32, viewer *User, forOwner bool) {
	sons := api.store.sonsOf(owner)
	visible := func(s *Son) *Son {
		if s == nil || s.Statut == "refuse" || s.Statut == "attente" && !forOwner {
			return nil
		}
		return s
	}
	var all []*Oeuvre
	for _, s := range m.Salles {
		var keep []*MuseeOeuvre
		for _, w := range s.Oeuvres {
			o, _ := api.store.Oeuvre(w.OeuvreID)
			if o == nil || o.Statut != "active" {
				continue
			}
			w.Oeuvre = o
			all = append(all, o)
			w.Voix = visible(sons[w.VoixID])
			if mm := sonRefRE.FindStringSubmatch(w.Son); mm != nil {
				id, _ := strconv.ParseInt(mm[1], 10, 64)
				w.SonFile = visible(sons[id])
			}
			keep = append(keep, w)
		}
		if keep == nil {
			keep = []*MuseeOeuvre{}
		}
		s.Oeuvres = keep
	}
	for _, f := range OeuvreDecorators {
		f(api, all, viewer)
	}
}

func (api *API) musicOf(m *Musee, owner uint32, forOwner bool) map[string]any {
	if mm := sonRefRE.FindStringSubmatch(m.Musique); mm != nil {
		id, _ := strconv.ParseInt(mm[1], 10, 64)
		if s := api.store.sonsOf(owner)[id]; s != nil && (s.Statut == "ok" || forOwner && s.Statut == "attente") {
			return map[string]any{"type": "fichier", "url": s.URL, "nom": s.Nom, "statut": s.Statut}
		}
		return map[string]any{"type": "silence"}
	}
	return map[string]any{"type": m.Musique, "nom": musicLabels[m.Musique]}
}

// /api/musees/<slug> (GET) · /api/musees/<slug>/livre-or (POST) · /api/musees/<slug>/like (POST)
func (api *API) handleMuseeVisit(w http.ResponseWriter, r *http.Request) {
	slug, sub, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/musees/"), "/")
	owner, _ := api.store.UserBySlug(slug)
	viewer := api.auth.User(r)
	var mr *museeRow
	if owner != nil {
		mr, _ = api.store.Musee(owner.ID)
	}
	if owner == nil || mr == nil || !mr.publie && (viewer == nil || viewer.ID != owner.ID) {
		writeError(w, http.StatusNotFound, "Ce musée n'a pas encore ouvert ses portes.")
		return
	}
	if !canVisit(mr, owner, viewer) {
		if viewer == nil {
			writeError(w, http.StatusUnauthorized, "Ce musée est réservé aux joueurs connectés.")
		} else {
			writeError(w, http.StatusForbidden, "Ce musée est privé pour l'instant.")
		}
		return
	}
	switch sub {
	case "":
		if !getOnly(w, r) {
			return
		}
		isOwner := viewer != nil && viewer.ID == owner.ID
		if r.URL.Query().Get("visite") != "" && !isOwner {
			key := fmt.Sprintf("%d:%s:%s", owner.ID, visitorKey(r, viewer), api.store.now().In(Paris).Format("2006-01-02"))
			api.museums.mu.Lock()
			fresh := !api.museums.visits[key]
			api.museums.visits[key] = true
			if len(api.museums.visits) > 100000 {
				api.museums.visits = map[string]bool{}
			}
			api.museums.mu.Unlock()
			if fresh {
				api.store.db.Exec(`UPDATE musees SET visites = visites + 1 WHERE user_id = ?`, owner.ID)
				mr.visites++
			}
		}
		api.fillMusee(mr.m, owner.ID, viewer, isOwner)
		book, total := api.store.guestbook(owner.ID, 50)
		var likes int
		var liked bool
		api.store.db.QueryRow(`SELECT COUNT(*) FROM musee_likes WHERE musee_id = ?`, owner.ID).Scan(&likes)
		if viewer != nil {
			var one int
			liked = api.store.db.QueryRow(`SELECT 1 FROM musee_likes WHERE musee_id = ? AND user_id = ?`, owner.ID, viewer.ID).Scan(&one) == nil
		}
		fond, accent := museeCard(mr.m)
		writeJSON(w, map[string]any{
			"musee": mr.m, "user": owner.Public(), "publie": mr.publie, "visites": mr.visites, "likes": likes, "liked": liked,
			"livre": book, "mots": total, "musique": api.musicOf(mr.m, owner.ID, isOwner), "fond": fond, "accent": accent,
			"moi": isOwner, "oeuvres": museeCount(mr.m),
		})
	case "livre-or":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
			return
		}
		if viewer == nil {
			writeError(w, http.StatusUnauthorized, "Connecte-toi pour signer le livre d'or.")
			return
		}
		if !mr.m.LivreOr {
			writeError(w, http.StatusForbidden, "Le livre d'or de ce musée est fermé.")
			return
		}
		if viewer.WriteBlock(api.store.now()) != "" {
			writeError(w, http.StatusForbidden, "Ton compte ne peut pas écrire pour l'instant.")
			return
		}
		var req struct {
			Texte string `json:"texte"`
		}
		if !readJSON(w, r, &req) {
			return
		}
		texte := strings.Join(strings.Fields(req.Texte), " ")
		switch {
		case texte == "":
			writeFieldError(w, &FieldError{"texte", "Écris un petit mot."})
			return
		case utf8.RuneCountInString(texte) > 200:
			writeFieldError(w, &FieldError{"texte", "200 caractères au maximum."})
			return
		}
		st := api.Settings()
		if st.GuestbookFlt || st.WordsFilter {
			if bad := st.filterWords(texte); bad != "" {
				writeFieldError(w, &FieldError{"texte", "Ce message contient un mot refusé par le filtre."})
				return
			}
		}
		var recent int
		api.store.db.QueryRow(`SELECT COUNT(*) FROM livre_or WHERE auteur_id = ? AND ts > ?`, viewer.ID, api.store.now().Add(-time.Minute).UnixMilli()).Scan(&recent)
		if recent >= 3 {
			writeError(w, http.StatusTooManyRequests, "Doucement ! Réessaie dans une minute.")
			return
		}
		now := api.store.now().UnixMilli()
		res, err := api.store.db.Exec(`INSERT INTO livre_or (musee_id, auteur_id, texte, ts) VALUES (?, ?, ?, ?)`, owner.ID, viewer.ID, texte, now)
		if err != nil {
			api.serverError(w, "Guestbook", err)
			return
		}
		id, _ := res.LastInsertId()
		if viewer.ID != owner.ID {
			api.community.alert(owner.ID, "livre_or", map[string]any{"by": viewer.Public(), "texte": texte, "href": "/u/" + owner.Slug + "/musee"})
		}
		writeJSON(w, map[string]any{"ok": true, "entree": guestEntry{ID: id, Auteur: viewer.Public(), Texte: texte, TS: now}})
	case "like":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
			return
		}
		if viewer == nil {
			writeError(w, http.StatusUnauthorized, "Connecte-toi pour donner un coup de cœur.")
			return
		}
		res, _ := api.store.db.Exec(`DELETE FROM musee_likes WHERE user_id = ? AND musee_id = ?`, viewer.ID, owner.ID)
		liked := false
		if n, _ := res.RowsAffected(); n == 0 {
			api.store.db.Exec(`INSERT INTO musee_likes (user_id, musee_id, ts) VALUES (?, ?, ?)`, viewer.ID, owner.ID, api.store.now().UnixMilli())
			liked = true
		}
		var likes int
		api.store.db.QueryRow(`SELECT COUNT(*) FROM musee_likes WHERE musee_id = ?`, owner.ID).Scan(&likes)
		writeJSON(w, map[string]any{"liked": liked, "likes": likes})
	default:
		http.NotFound(w, r)
	}
}

func visitorKey(r *http.Request, viewer *User) string {
	if viewer != nil {
		return "u" + strconv.Itoa(int(viewer.ID))
	}
	ip := r.RemoteAddr
	if i := strings.LastIndex(ip, ":"); i > 0 {
		ip = ip[:i]
	}
	return "ip" + ip
}

// GET /api/me/musee — the editor; PUT saves {musee, publie}.
func (api *API) handleMyMusee(w http.ResponseWriter, r *http.Request) {
	u := api.auth.User(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "Connecte-toi pour ouvrir ton musée.")
		return
	}
	switch r.Method {
	case http.MethodGet:
		mr, err := api.store.Musee(u.ID)
		if err != nil {
			api.serverError(w, "Musee", err)
			return
		}
		m, publie, visites := defaultMusee(u), false, 0
		if mr != nil {
			m, publie, visites = mr.m, mr.publie, mr.visites
		}
		api.fillMusee(m, u.ID, u, true)
		// what can be hung: the player's artworks, then their favourites
		mine, _ := api.store.Oeuvres(u.ID, false)
		liked := api.store.Liked(u.ID)
		all, _ := api.store.Oeuvres(0, false)
		var coeur []*Oeuvre
		for _, o := range all {
			if liked[o.ID] {
				coeur = append(coeur, o)
			}
		}
		for _, f := range OeuvreDecorators {
			f(api, mine, u)
			f(api, coeur, u)
		}
		if mine == nil {
			mine = []*Oeuvre{}
		}
		if coeur == nil {
			coeur = []*Oeuvre{}
		}
		sons := []*Son{}
		for _, s := range api.store.sonsOf(u.ID) {
			if s.Statut != "refuse" {
				sons = append(sons, s)
			}
		}
		sort.Slice(sons, func(i, j int) bool { return sons[i].ID > sons[j].ID })
		_, mots := api.store.guestbook(u.ID, 0)
		writeJSON(w, map[string]any{"musee": m, "publie": publie, "existe": mr != nil, "visites": visites, "mots": mots,
			"miennes": mine, "coeurs": coeur, "sons": sons, "choix": museeChoices, "user": u.Public()})
	case http.MethodPut:
		var req struct {
			Musee  *Musee `json:"musee"`
			Publie bool   `json:"publie"`
		}
		if !readJSON(w, r, &req) {
			return
		}
		if req.Musee == nil {
			writeError(w, http.StatusBadRequest, "Requête illisible.")
			return
		}
		if u.WriteBlock(api.store.now()) != "" {
			writeError(w, http.StatusForbidden, "Ton compte ne peut pas modifier son musée pour l'instant.")
			return
		}
		m := api.sanitizeMusee(u, req.Musee)
		first, err := api.store.SaveMusee(u.ID, m, req.Publie)
		if err != nil {
			api.serverError(w, "SaveMusee", err)
			return
		}
		st := api.Settings()
		if st.WordsFilter {
			for _, t := range []string{m.Nom, m.Accueil, m.PhraseGuide} {
				if bad := st.filterWords(t); bad != "" {
					api.store.AddReport(Report{Type: "musee", Cible: fmt.Sprintf("musee:%d", u.ID), uid: u.ID, Source: "filtre", Raison: "Mot interdit : « " + bad + " »", Contenu: t})
					break
				}
			}
		}
		if first && api.community != nil {
			api.community.Emit("musee", u, 0, 0, 1, fmt.Sprintf("musee:%d", u.ID), m.Nom)
		}
		api.fillMusee(m, u.ID, u, true)
		writeJSON(w, map[string]any{"ok": true, "musee": m, "publie": req.Publie})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
	}
}

// audioKind recognises the formats a browser can record or play.
func audioKind(b []byte) string {
	switch {
	case len(b) > 4 && bytes.Equal(b[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}):
		return "webm"
	case len(b) > 4 && string(b[:4]) == "OggS":
		return "ogg"
	case len(b) > 3 && string(b[:3]) == "ID3", len(b) > 2 && b[0] == 0xff && b[1]&0xe0 == 0xe0:
		return "mp3"
	case len(b) > 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WAVE":
		return "wav"
	case len(b) > 12 && string(b[4:8]) == "ftyp":
		return "m4a"
	}
	return ""
}

var sonNameRE = regexp.MustCompile(`^\d+-[0-9a-f]{12}\.(webm|ogg|mp3|wav|m4a)$`)

// POST /api/me/musee/sons (multipart: fichier, type, nom, duree) — upload a sound.
// DELETE /api/me/musee/sons?id= — remove one.
func (api *API) handleMySons(w http.ResponseWriter, r *http.Request) {
	u := api.auth.User(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "Connecte-toi.")
		return
	}
	if r.Method == http.MethodDelete {
		id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		api.store.db.Exec(`DELETE FROM sons WHERE id = ? AND user_id = ?`, id, u.ID)
		writeJSON(w, map[string]any{"ok": true})
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
		return
	}
	if u.WriteBlock(api.store.now()) != "" {
		writeError(w, http.StatusForbidden, "Ton compte ne peut pas importer de son pour l'instant.")
		return
	}
	typ := r.FormValue("type")
	limit := int64(4 << 20)
	switch typ {
	case "voix":
		limit = 1 << 20
	case "passage":
		limit = 512 << 10
	case "musique":
	default:
		writeFieldError(w, &FieldError{"type", "Type de son inconnu."})
		return
	}
	var count int
	api.store.db.QueryRow(`SELECT COUNT(*) FROM sons WHERE user_id = ? AND statut != 'refuse'`, u.ID).Scan(&count)
	if count >= maxSons {
		writeFieldError(w, &FieldError{"fichier", fmt.Sprintf("%d sons au maximum : supprimes-en un d'abord.", maxSons)})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit+64<<10)
	f, _, err := r.FormFile("fichier")
	if err != nil {
		writeFieldError(w, &FieldError{"fichier", fmt.Sprintf("Fichier trop lourd ou illisible (%d Ko max).", limit>>10)})
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		writeFieldError(w, &FieldError{"fichier", fmt.Sprintf("Fichier trop lourd (%d Ko max).", limit>>10)})
		return
	}
	ext := audioKind(data)
	if ext == "" {
		writeFieldError(w, &FieldError{"fichier", "Ce fichier n'est pas un son (mp3, ogg, webm, wav ou m4a)."})
		return
	}
	duree, _ := strconv.ParseFloat(r.FormValue("duree"), 64)
	st := api.Settings()
	if typ == "voix" && st.VoiceLimit && duree > 31 {
		writeFieldError(w, &FieldError{"fichier", "Le commentaire vocal dure 30 secondes au maximum."})
		return
	}
	sum := sha1.Sum(data)
	name := fmt.Sprintf("%d-%s.%s", u.ID, hex.EncodeToString(sum[:6]), ext)
	dir := filepath.Join(api.mediaDir, "sons")
	if err := os.MkdirAll(dir, 0755); err != nil {
		api.serverError(w, "Sons", err)
		return
	}
	if err := writeFileAtomic(filepath.Join(dir, name), data); err != nil {
		api.serverError(w, "Sons", err)
		return
	}
	statut := "ok"
	if st.SoundsCheck {
		statut = "attente"
	}
	nom := clip(r.FormValue("nom"), 40)
	if nom == "" {
		nom = map[string]string{"voix": "Commentaire vocal", "musique": "Ma musique", "passage": "Mon son"}[typ]
	}
	url := "/media/sons/" + name
	res, err := api.store.db.Exec(`INSERT INTO sons (user_id, url, nom, type, duree, statut, ts) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.ID, url, nom, typ, duree, statut, api.store.now().UnixMilli())
	if err != nil {
		api.serverError(w, "Sons", err)
		return
	}
	id, _ := res.LastInsertId()
	if statut == "attente" {
		api.store.AddReport(Report{Type: "son", Cible: fmt.Sprintf("son:%d", id), uid: u.ID, Source: "filtre", Raison: "Nouveau son à écouter : " + nom, Contenu: url})
	}
	writeJSON(w, map[string]any{"ok": true, "son": Son{ID: id, URL: url, Nom: nom, Type: typ, Duree: duree, Statut: statut}})
}
