package place

// Phase 3 — claims (revendications): players claim artworks drawn before accounts,
// the backups tell when and how they were drawn, the community confirms or doubts,
// an admin decides. A validated claim becomes an œuvre with authors, pioneer pixels
// and badges.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

func init() {
	// Profiles show the player's artworks and a claim in progress.
	ProfileExtras = append(ProfileExtras, func(api *API, u *User, viewer *User) (string, any) {
		os, err := api.store.Oeuvres(u.ID, false)
		if err != nil || len(os) == 0 {
			return "", nil
		}
		for _, f := range OeuvreDecorators {
			f(api, os, viewer)
		}
		type item struct {
			ID        int64  `json:"id"`
			Titre     string `json:"titre"`
			Href      string `json:"href"`
			Thumb     string `json:"thumb"`
			Sub       string `json:"sub"`
			Pill      string `json:"pill,omitempty"`
			PillClass string `json:"pillClass,omitempty"`
		}
		var out []item
		for _, o := range os {
			sub := frenchNumber(o.Pixels) + " px"
			if o.Apparue != 0 {
				sub += " · " + frenchDate(time.UnixMilli(o.Apparue))
			}
			var others []string
			for _, a := range o.Auteurs {
				if a.User != nil && a.User.ID != u.ID {
					others = append(others, a.User.Pseudo)
				}
			}
			if len(others) > 0 {
				sub = "avec " + strings.Join(others, ", ") + " · " + sub
			}
			it := item{ID: o.ID, Titre: o.Titre, Href: fmt.Sprintf("/oeuvre/%d", o.ID), Thumb: cropURL(o.X, o.Y, o.W, o.H, 96), Sub: sub}
			if o.Origine == "avant_migration" {
				it.Pill, it.PillClass = "Pionnière", "st-attente"
			}
			if s, ok := o.Extra["retouchee"].(bool); ok && s {
				it.Pill, it.PillClass = "Retouchée", "st-info"
			}
			out = append(out, it)
		}
		return "oeuvres", out
	})
	ProfileExtras = append(ProfileExtras, func(api *API, u *User, viewer *User) (string, any) {
		cs, err := api.store.Claims("attente", u.ID, 0)
		if err != nil || len(cs) == 0 {
			return "", nil
		}
		return "claim_en_cours", map[string]any{"id": cs[0].ID, "titre": cs[0].Titre}
	})

}

const (
	maxPendingClaims = 5
	maxCoauthors     = 5
	conflictOverlap  = 0.30 // a claim overlapping another one by 30 % of the smaller is "à départager"
)

var goldBadges = []string{"pionnier", "veteran", "batisseur", "indemodable"}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// cropURL is a thumbnail of a zone of the live canvas, about size px wide.
func cropURL(x, y, w, h, size int) string {
	m := max(2, min(8, max(w, h)/6))
	x, y, w, h = x-m, y-m, w+2*m, h+2*m
	z := max(1, min(16, size/max(w, h, 1)))
	return fmt.Sprintf("/api/crop.png?x=%d&y=%d&w=%d&h=%d&z=%d", max(x, 0), max(y, 0), w, h, z)
}

// ------------------------------------------------------------------
// Settings

func (s *Store) Setting(key, def string) string {
	var v string
	if err := s.db.QueryRow(`SELECT valeur FROM settings WHERE cle = ?`, key).Scan(&v); err != nil {
		return def
	}
	return v
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (cle, valeur) VALUES (?, ?) ON CONFLICT (cle) DO UPDATE SET valeur = excluded.valeur`, key, value)
	return err
}

// MigrationTime is when accounts opened: the "migration" setting, else the first account.
func (s *Store) MigrationTime() time.Time {
	if v := s.Setting("migration", ""); v != "" {
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
			return time.UnixMilli(ms)
		}
	}
	var first sql.NullInt64
	s.db.QueryRow(`SELECT MIN(cree_le) FROM users`).Scan(&first)
	if first.Valid {
		return time.UnixMilli(first.Int64)
	}
	return time.Time{}
}

// ------------------------------------------------------------------
// Artworks

type Oeuvre struct {
	ID          int64          `json:"id"`
	Titre       string         `json:"titre"`
	Description string         `json:"description"`
	Mask        Mask           `json:"masque"`
	X           int            `json:"x"`
	Y           int            `json:"y"`
	W           int            `json:"w"`
	H           int            `json:"h"`
	Pixels      int            `json:"pixels"`
	Origine     string         `json:"origine"`
	Statut      string         `json:"statut"`
	Apparue     int64          `json:"apparue_le,omitempty"`
	Terminee    int64          `json:"terminee_le,omitempty"`
	ClaimID     int64          `json:"claim_id,omitempty"`
	CreeLe      int64          `json:"creee_le"`
	Auteurs     []OeuvreAuteur `json:"auteurs"`
	Extra       map[string]any `json:"extra,omitempty"`
	analyseJSON string
}

type OeuvreAuteur struct {
	User    *PublicUser `json:"user"`
	Role    string      `json:"role"`
	Pioneer int64       `json:"pixels_pionniers"`
	userID  uint32
}

func (o *Oeuvre) Analysis() *ZoneAnalysis {
	if o.analyseJSON == "" {
		return nil
	}
	var a ZoneAnalysis
	if json.Unmarshal([]byte(o.analyseJSON), &a) != nil {
		return nil
	}
	return &a
}

const oeuvreCols = `id, titre, description, masque, x, y, w, h, pixels, origine, statut, COALESCE(apparue_le, 0),
	COALESCE(terminee_le, 0), COALESCE(claim_id, 0), creee_le, analyse_json`

func scanOeuvre(row scanner) (*Oeuvre, error) {
	o := &Oeuvre{}
	var mask string
	err := row.Scan(&o.ID, &o.Titre, &o.Description, &mask, &o.X, &o.Y, &o.W, &o.H, &o.Pixels, &o.Origine, &o.Statut,
		&o.Apparue, &o.Terminee, &o.ClaimID, &o.CreeLe, &o.analyseJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err == nil {
		err = json.Unmarshal([]byte(mask), &o.Mask)
	}
	return o, err
}

func (s *Store) loadAuthors(os []*Oeuvre) error {
	if len(os) == 0 {
		return nil
	}
	byID := map[int64]*Oeuvre{}
	ids := []string{}
	for _, o := range os {
		byID[o.ID] = o
		o.Auteurs = []OeuvreAuteur{}
		ids = append(ids, strconv.FormatInt(o.ID, 10))
	}
	rows, err := s.db.Query(`SELECT oeuvre_id, user_id, role, pixels_pionniers FROM oeuvre_auteurs WHERE oeuvre_id IN (` + strings.Join(ids, ",") + `)
		ORDER BY role = 'co_auteur', user_id`)
	if err != nil {
		return err
	}
	var uids []uint32
	for rows.Next() {
		var oid int64
		var a OeuvreAuteur
		if err := rows.Scan(&oid, &a.userID, &a.Role, &a.Pioneer); err != nil {
			rows.Close()
			return err
		}
		byID[oid].Auteurs = append(byID[oid].Auteurs, a)
		uids = append(uids, a.userID)
	}
	rows.Close()
	users, err := s.UsersByIDs(uids)
	if err != nil {
		return err
	}
	for _, o := range os {
		for i := range o.Auteurs {
			o.Auteurs[i].User = users[o.Auteurs[i].userID].Public()
		}
	}
	return nil
}

func (s *Store) Oeuvre(id int64) (*Oeuvre, error) {
	o, err := scanOeuvre(s.db.QueryRow(`SELECT `+oeuvreCols+` FROM oeuvres WHERE id = ?`, id))
	if o == nil || err != nil {
		return o, err
	}
	return o, s.loadAuthors([]*Oeuvre{o})
}

// Oeuvres lists artworks (active ones unless all), newest first; uid > 0: by that author.
func (s *Store) Oeuvres(uid uint32, all bool) ([]*Oeuvre, error) {
	q := `SELECT ` + oeuvreCols + ` FROM oeuvres WHERE 1 = 1`
	var args []any
	if !all {
		q += ` AND statut = 'active'`
	}
	if uid > 0 {
		q += ` AND id IN (SELECT oeuvre_id FROM oeuvre_auteurs WHERE user_id = ?)`
		args = append(args, uid)
	}
	rows, err := s.db.Query(q+` ORDER BY COALESCE(apparue_le, creee_le) DESC`, args...)
	if err != nil {
		return nil, err
	}
	var out []*Oeuvre
	for rows.Next() {
		o, err := scanOeuvre(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, o)
	}
	rows.Close()
	return out, s.loadAuthors(out)
}

// OeuvreIndex finds the artwork a pixel belongs to.
type OeuvreIndex struct {
	mu    sync.RWMutex
	items []oeuvreArea
}

type oeuvreArea struct {
	id   int64
	box  image.Rectangle
	bits map[int32]bool
}

func (oi *OeuvreIndex) Rebuild(os []*Oeuvre, cw, ch int) {
	var items []oeuvreArea
	for _, o := range os {
		if o.Statut != "active" {
			continue
		}
		pos, err := o.Mask.Positions(cw, ch)
		if err != nil {
			continue
		}
		bits := make(map[int32]bool, len(pos))
		for _, p := range pos {
			bits[p] = true
		}
		items = append(items, oeuvreArea{o.ID, o.Mask.Bounds(cw, ch), bits})
	}
	// smaller artworks first: an artwork inside another one wins
	sort.Slice(items, func(i, j int) bool { return len(items[i].bits) < len(items[j].bits) })
	oi.mu.Lock()
	oi.items = items
	oi.mu.Unlock()
}

// At returns the id of the artwork containing (x, y), 0 if none.
func (oi *OeuvreIndex) At(x, y, cw int) int64 {
	oi.mu.RLock()
	defer oi.mu.RUnlock()
	p := image.Pt(x, y)
	for _, it := range oi.items {
		if p.In(it.box) && it.bits[int32(y*cw+x)] {
			return it.id
		}
	}
	return 0
}

// ------------------------------------------------------------------
// Claims

type Claim struct {
	ID          int64           `json:"id"`
	Requester   *PublicUser     `json:"demandeur"`
	Mask        Mask            `json:"masque"`
	X           int             `json:"x"`
	Y           int             `json:"y"`
	W           int             `json:"w"`
	H           int             `json:"h"`
	Pixels      int             `json:"pixels"`
	Titre       string          `json:"titre"`
	Message     string          `json:"message"`
	Repartition string          `json:"repartition"`
	Statut      string          `json:"statut"`
	Badges      []string        `json:"badges_donnes"`
	Motif       string          `json:"motif,omitempty"`
	OeuvreID    int64           `json:"oeuvre_id,omitempty"`
	DecideLe    int64           `json:"decide_le,omitempty"`
	CreeLe      int64           `json:"cree_le"`
	Coauthors   []ClaimCoauthor `json:"co_auteurs"`
	Confirms    int             `json:"confirmations"`
	Doubts      int             `json:"doutes"`
	MyVote      string          `json:"mon_vote,omitempty"`
	Analysis    *ZoneAnalysis   `json:"analyse,omitempty"`
	Conflicts   []ClaimConflict `json:"conflits"`
	Summary     string          `json:"resume"`

	requesterID uint32
	weight      float64
	analyseJSON string
	positions   []int32
}

type ClaimCoauthor struct {
	User      *PublicUser `json:"user"`
	Weight    float64     `json:"poids"`
	Confirmed bool        `json:"confirme"`
	userID    uint32
}

type ClaimConflict struct {
	ClaimID  int64       `json:"claim_id,omitempty"`
	OeuvreID int64       `json:"oeuvre_id,omitempty"`
	Titre    string      `json:"titre"`
	By       *PublicUser `json:"par,omitempty"`
	Pct      float64     `json:"recouvrement_pct"`
	Mask     Mask        `json:"masque"`
}

const claimCols = `id, demandeur_id, masque, x, y, w, h, pixels, titre, message, repartition, poids_demandeur, statut,
	analyse_json, badges, motif, COALESCE(oeuvre_id, 0), COALESCE(decide_le, 0), cree_le`

func scanClaim(row scanner) (*Claim, error) {
	c := &Claim{}
	var mask, badges string
	err := row.Scan(&c.ID, &c.requesterID, &mask, &c.X, &c.Y, &c.W, &c.H, &c.Pixels, &c.Titre, &c.Message, &c.Repartition,
		&c.weight, &c.Statut, &c.analyseJSON, &badges, &c.Motif, &c.OeuvreID, &c.DecideLe, &c.CreeLe)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Badges = []string{}
	if badges != "" {
		c.Badges = strings.Split(badges, ",")
	}
	c.Conflicts = []ClaimConflict{}
	if c.analyseJSON != "" {
		var a ZoneAnalysis
		if json.Unmarshal([]byte(c.analyseJSON), &a) == nil {
			c.Analysis = &a
		}
	}
	return c, json.Unmarshal([]byte(mask), &c.Mask)
}

// fillClaims loads people, co-authors and votes of claims.
func (s *Store) fillClaims(cs []*Claim, viewer uint32) error {
	if len(cs) == 0 {
		return nil
	}
	byID := map[int64]*Claim{}
	ids := []string{}
	uids := []uint32{}
	for _, c := range cs {
		byID[c.ID] = c
		c.Coauthors = []ClaimCoauthor{}
		ids = append(ids, strconv.FormatInt(c.ID, 10))
		uids = append(uids, c.requesterID)
	}
	in := strings.Join(ids, ",")
	rows, err := s.db.Query(`SELECT claim_id, user_id, poids, confirme FROM claim_coauteurs WHERE claim_id IN (` + in + `) ORDER BY user_id`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var cid int64
		var ca ClaimCoauthor
		if err := rows.Scan(&cid, &ca.userID, &ca.Weight, &ca.Confirmed); err != nil {
			rows.Close()
			return err
		}
		byID[cid].Coauthors = append(byID[cid].Coauthors, ca)
		uids = append(uids, ca.userID)
	}
	rows.Close()
	rows, err = s.db.Query(`SELECT claim_id, user_id, type FROM claim_votes WHERE claim_id IN (` + in + `)`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var cid int64
		var uid uint32
		var typ string
		if err := rows.Scan(&cid, &uid, &typ); err != nil {
			rows.Close()
			return err
		}
		c := byID[cid]
		if typ == "confirme" {
			c.Confirms++
		} else {
			c.Doubts++
		}
		if uid == viewer {
			c.MyVote = typ
		}
	}
	rows.Close()
	users, err := s.UsersByIDs(uids)
	if err != nil {
		return err
	}
	for _, c := range cs {
		c.Requester = users[c.requesterID].Public()
		for i := range c.Coauthors {
			c.Coauthors[i].User = users[c.Coauthors[i].userID].Public()
		}
	}
	return nil
}

func (s *Store) Claim(id int64, viewer uint32) (*Claim, error) {
	c, err := scanClaim(s.db.QueryRow(`SELECT `+claimCols+` FROM claims WHERE id = ?`, id))
	if c == nil || err != nil {
		return c, err
	}
	return c, s.fillClaims([]*Claim{c}, viewer)
}

// Claims lists claims by status ("attente", "validee", "refusee"); uid > 0: only that player's.
func (s *Store) Claims(statut string, uid uint32, viewer uint32) ([]*Claim, error) {
	q := `SELECT ` + claimCols + ` FROM claims WHERE statut = ?`
	args := []any{statut}
	if uid > 0 {
		q += ` AND (demandeur_id = ? OR id IN (SELECT claim_id FROM claim_coauteurs WHERE user_id = ?))`
		args = append(args, uid, uid)
	}
	order := ` ORDER BY id DESC`
	if statut != "attente" {
		order = ` ORDER BY COALESCE(decide_le, cree_le) DESC`
	}
	rows, err := s.db.Query(q+order+` LIMIT 200`, args...)
	if err != nil {
		return nil, err
	}
	var out []*Claim
	for rows.Next() {
		c, err := scanClaim(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, c)
	}
	rows.Close()
	return out, s.fillClaims(out, viewer)
}

func (s *Store) ClaimCounts() (map[string]int, error) {
	res := map[string]int{"attente": 0, "validee": 0, "refusee": 0}
	rows, err := s.db.Query(`SELECT statut, COUNT(*) FROM claims GROUP BY statut`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		res[st] = n
	}
	return res, rows.Err()
}

// NewClaim is what a player sends.
type NewClaim struct {
	Mask        Mask               `json:"masque"`
	Titre       string             `json:"titre"`
	Message     string             `json:"message"`
	Coauthors   []string           `json:"co_auteurs"` // slugs
	Repartition string             `json:"repartition"`
	Weights     map[string]float64 `json:"poids"` // slug → weight, "moi" for the requester
}

func (s *Store) CreateClaim(u *User, n NewClaim, pos []int32, cw, ch int, analysis *ZoneAnalysis) (int64, error) {
	titre := strings.Join(strings.Fields(n.Titre), " ")
	if l := utf8.RuneCountInString(titre); l < 2 || l > 60 {
		return 0, &FieldError{"titre", "Entre 2 et 60 caractères."}
	}
	msg := strings.TrimSpace(n.Message)
	if utf8.RuneCountInString(msg) > 1000 {
		return 0, &FieldError{"message", "1 000 caractères au maximum."}
	}
	if len(n.Coauthors) > maxCoauthors {
		return 0, &FieldError{"co_auteurs", fmt.Sprintf("%d co-auteurs au maximum.", maxCoauthors)}
	}
	var pending int
	s.db.QueryRow(`SELECT COUNT(*) FROM claims WHERE demandeur_id = ? AND statut = 'attente'`, u.ID).Scan(&pending)
	if pending >= maxPendingClaims {
		return 0, &FieldError{"", fmt.Sprintf("Tu as déjà %d demandes en attente : patiente un peu.", maxPendingClaims)}
	}
	rep := "egale"
	if n.Repartition == "poids" {
		rep = "poids"
	}
	weight := func(key string) float64 {
		if rep != "poids" {
			return 1
		}
		w := n.Weights[key]
		if w <= 0 || math.IsNaN(w) || w > 100 {
			return 1
		}
		return w
	}
	coauthors := []*User{}
	seen := map[uint32]bool{u.ID: true}
	for _, slug := range n.Coauthors {
		cu, err := s.UserBySlug(slug)
		if err != nil {
			return 0, err
		}
		if cu == nil {
			return 0, &FieldError{"co_auteurs", "Joueur introuvable : " + slug}
		}
		if seen[cu.ID] {
			continue
		}
		seen[cu.ID] = true
		coauthors = append(coauthors, cu)
	}
	b := n.Mask.Bounds(cw, ch)
	maskJSON, _ := json.Marshal(n.Mask)
	var aJSON []byte
	if analysis != nil {
		aJSON, _ = json.Marshal(analysis)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO claims (demandeur_id, masque, x, y, w, h, pixels, titre, message, repartition, poids_demandeur, analyse_json, cree_le)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, string(maskJSON), b.Min.X, b.Min.Y, b.Dx(), b.Dy(), len(pos), titre, msg, rep, weight("moi"), string(aJSON), s.now().UnixMilli())
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	for _, cu := range coauthors {
		if _, err := tx.Exec(`INSERT INTO claim_coauteurs (claim_id, user_id, poids) VALUES (?, ?, ?)`, id, cu.ID, weight(cu.Slug)); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

func (s *Store) SetClaimAnalysis(id int64, a *ZoneAnalysis) error {
	b, _ := json.Marshal(a)
	_, err := s.db.Exec(`UPDATE claims SET analyse_json = ? WHERE id = ?`, string(b), id)
	return err
}

// Vote records "confirme" or "doute" ("" removes the vote).
func (s *Store) Vote(c *Claim, u *User, typ, comment string) error {
	if c.Statut != "attente" {
		return &FieldError{"", "Cette demande est déjà tranchée."}
	}
	if c.requesterID == u.ID {
		return &FieldError{"", "Tu ne peux pas voter pour ta propre demande."}
	}
	for _, ca := range c.Coauthors {
		if ca.userID == u.ID {
			return &FieldError{"", "Tu fais partie des auteurs de cette demande."}
		}
	}
	if typ == "" {
		_, err := s.db.Exec(`DELETE FROM claim_votes WHERE claim_id = ? AND user_id = ?`, c.ID, u.ID)
		return err
	}
	if typ != "confirme" && typ != "doute" {
		return &FieldError{"type", "Vote inconnu."}
	}
	comment = strings.TrimSpace(comment)
	if utf8.RuneCountInString(comment) > 280 {
		return &FieldError{"commentaire", "280 caractères au maximum."}
	}
	_, err := s.db.Exec(`INSERT INTO claim_votes (claim_id, user_id, type, commentaire, ts) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (claim_id, user_id) DO UPDATE SET type = excluded.type, commentaire = excluded.commentaire, ts = excluded.ts`,
		c.ID, u.ID, typ, comment, s.now().UnixMilli())
	return err
}

type ClaimVote struct {
	User    *PublicUser `json:"user"`
	Type    string      `json:"type"`
	Comment string      `json:"commentaire"`
	TS      int64       `json:"ts"`
}

func (s *Store) ClaimVotes(id int64) ([]ClaimVote, error) {
	rows, err := s.db.Query(`SELECT user_id, type, commentaire, ts FROM claim_votes WHERE claim_id = ? ORDER BY ts DESC`, id)
	if err != nil {
		return nil, err
	}
	var out []ClaimVote
	var uids []uint32
	var raw []uint32
	for rows.Next() {
		var v ClaimVote
		var uid uint32
		if err := rows.Scan(&uid, &v.Type, &v.Comment, &v.TS); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, v)
		raw = append(raw, uid)
		uids = append(uids, uid)
	}
	rows.Close()
	users, err := s.UsersByIDs(uids)
	for i := range out {
		out[i].User = users[raw[i]].Public()
	}
	if out == nil {
		out = []ClaimVote{}
	}
	return out, err
}

// ConfirmCoauthor records a co-author's answer (false removes them from the claim).
func (s *Store) ConfirmCoauthor(c *Claim, u *User, accept bool) error {
	if c.Statut != "attente" {
		return &FieldError{"", "Cette demande est déjà tranchée."}
	}
	var res sql.Result
	var err error
	if accept {
		res, err = s.db.Exec(`UPDATE claim_coauteurs SET confirme = 1 WHERE claim_id = ? AND user_id = ?`, c.ID, u.ID)
	} else {
		res, err = s.db.Exec(`DELETE FROM claim_coauteurs WHERE claim_id = ? AND user_id = ?`, c.ID, u.ID)
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return &FieldError{"", "Tu n'es pas co-auteur de cette demande."}
	}
	return nil
}

func (s *Store) CancelClaim(c *Claim, u *User) error {
	if c.requesterID != u.ID {
		return &FieldError{"", "Seul l'auteur de la demande peut l'annuler."}
	}
	if c.Statut != "attente" {
		return &FieldError{"", "Cette demande est déjà tranchée."}
	}
	_, err := s.db.Exec(`UPDATE claims SET statut = 'annulee', decide_le = ? WHERE id = ?`, s.now().UnixMilli(), c.ID)
	return err
}

// ------------------------------------------------------------------
// Decisions (admin)

type Decision struct {
	Action   string   `json:"action"` // valider | fusionner | attribuer | refuser
	Badges   []string `json:"badges"` // gold badges to give (subset of the proposed ones)
	User     string   `json:"user"`   // attribuer: slug of the author
	Motif    string   `json:"motif"`
	MergeIDs []int64  `json:"fusion"` // fusionner: the other claims of the same artwork
}

// decisionRecord is kept in admin_actions to undo a decision.
type decisionRecord struct {
	Claims  []int64             `json:"claims"`
	Statuts map[int64]string    `json:"statuts"`
	Oeuvre  int64               `json:"oeuvre,omitempty"`
	Pioneer map[uint32]int64    `json:"pionniers,omitempty"`
	Badges  map[uint32][]string `json:"badges,omitempty"` // badges this decision granted
}

// Decide applies an admin decision. It returns the journal id.
func (s *Store) Decide(admin *User, c *Claim, d Decision, cw, ch int) (int64, error) {
	if c.Statut != "attente" {
		return 0, &FieldError{"", "Cette demande est déjà tranchée."}
	}
	now := s.now().UnixMilli()
	rec := decisionRecord{Claims: []int64{c.ID}, Statuts: map[int64]string{c.ID: c.Statut}, Pioneer: map[uint32]int64{}, Badges: map[uint32][]string{}}
	var targetAuthors []struct {
		id     uint32
		role   string
		weight float64
	}
	addAuthor := func(id uint32, role string, w float64) {
		for i, a := range targetAuthors {
			if a.id == id {
				targetAuthors[i].weight += w
				return
			}
		}
		targetAuthors = append(targetAuthors, struct {
			id     uint32
			role   string
			weight float64
		}{id, role, w})
	}
	claimStatut := "validee"
	motif := strings.TrimSpace(d.Motif)
	switch d.Action {
	case "valider", "fusionner":
		addAuthor(c.requesterID, "auteur", c.weight)
		for _, ca := range c.Coauthors {
			addAuthor(ca.userID, "co_auteur", ca.Weight)
		}
		if d.Action == "fusionner" {
			if len(d.MergeIDs) == 0 {
				return 0, &FieldError{"fusion", "Choisis la demande à fusionner."}
			}
			for _, mid := range d.MergeIDs {
				other, err := s.Claim(mid, 0)
				if err != nil {
					return 0, err
				}
				if other == nil || other.Statut != "attente" || other.ID == c.ID {
					return 0, &FieldError{"fusion", "Demande à fusionner introuvable."}
				}
				rec.Claims = append(rec.Claims, other.ID)
				rec.Statuts[other.ID] = other.Statut
				addAuthor(other.requesterID, "co_auteur", other.weight)
				for _, ca := range other.Coauthors {
					addAuthor(ca.userID, "co_auteur", ca.Weight)
				}
			}
		}
	case "attribuer":
		target, err := s.UserBySlug(d.User)
		if err != nil {
			return 0, err
		}
		if target == nil {
			return 0, &FieldError{"user", "Joueur introuvable."}
		}
		addAuthor(target.ID, "auteur", 1)
		claimStatut = "refusee"
		if motif == "" {
			motif = "Attribuée à " + target.Pseudo + " par l'équipe après vérification."
		}
	case "refuser":
		if motif == "" {
			return 0, &FieldError{"motif", "Indique le motif du refus : le joueur le recevra."}
		}
		claimStatut = "refusee"
	default:
		return 0, &FieldError{"action", "Décision inconnue."}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	badges := []string{}
	for _, b := range d.Badges {
		if contains(goldBadges, b) && !contains(badges, b) {
			badges = append(badges, b)
		}
	}
	if len(targetAuthors) > 0 {
		a := c.Analysis
		origine := "compte"
		if a == nil || contains(a.Badges, "pionnier") || contains(badges, "pionnier") {
			origine = "avant_migration"
		}
		var apparue, terminee any
		placed := 0
		if a != nil {
			if a.Appeared != 0 {
				apparue = a.Appeared
			}
			if a.Finished != 0 {
				terminee = a.Finished
			}
			placed = a.Placed
		}
		maskJSON, _ := json.Marshal(c.Mask)
		res, err := tx.Exec(`INSERT INTO oeuvres (titre, masque, x, y, w, h, pixels, origine, apparue_le, terminee_le, analyse_json, claim_id, creee_le)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.Titre, string(maskJSON), c.X, c.Y, c.W, c.H, c.Pixels, origine, apparue, terminee, c.analyseJSON, c.ID, now)
		if err != nil {
			return 0, err
		}
		rec.Oeuvre, _ = res.LastInsertId()
		total := 0.0
		for _, ta := range targetAuthors {
			total += ta.weight
		}
		for _, ta := range targetAuthors {
			share := int64(0)
			if origine == "avant_migration" && total > 0 {
				share = int64(math.Round(float64(placed) * ta.weight / total))
			}
			if _, err := tx.Exec(`INSERT INTO oeuvre_auteurs (oeuvre_id, user_id, role, pixels_pionniers) VALUES (?, ?, ?, ?)`, rec.Oeuvre, ta.id, ta.role, share); err != nil {
				return 0, err
			}
			if share > 0 {
				if _, err := tx.Exec(`UPDATE users SET pixels_pionniers = pixels_pionniers + ? WHERE id = ?`, share, ta.id); err != nil {
					return 0, err
				}
				rec.Pioneer[ta.id] = share
			}
			for _, b := range badges {
				isNew, err := grant(tx, ta.id, b, fmt.Sprintf("claim:%d", c.ID), c.Titre, now)
				if err != nil {
					return 0, err
				}
				if isNew {
					rec.Badges[ta.id] = append(rec.Badges[ta.id], b)
				}
			}
			earned, err := awardCounterBadges(tx, ta.id, now)
			if err != nil {
				return 0, err
			}
			rec.Badges[ta.id] = append(rec.Badges[ta.id], earned...)
		}
	}
	for _, id := range rec.Claims {
		st := claimStatut
		if id != c.ID {
			st = "validee"
		}
		var oeuvre any
		if rec.Oeuvre != 0 {
			oeuvre = rec.Oeuvre
		}
		if _, err := tx.Exec(`UPDATE claims SET statut = ?, badges = ?, motif = ?, oeuvre_id = ?, decide_par = ?, decide_le = ? WHERE id = ?`,
			st, strings.Join(badges, ","), motif, oeuvre, admin.ID, now, id); err != nil {
			return 0, err
		}
	}
	// Témoin: 10 confirmations on validated claims.
	if claimStatut == "validee" {
		rows, err := tx.Query(`SELECT v.user_id, (SELECT COUNT(*) FROM claim_votes v2 JOIN claims c2 ON c2.id = v2.claim_id
			WHERE v2.user_id = v.user_id AND v2.type = 'confirme' AND c2.statut = 'validee')
			FROM claim_votes v WHERE v.claim_id = ? AND v.type = 'confirme'`, c.ID)
		if err != nil {
			return 0, err
		}
		var witnesses []uint32
		for rows.Next() {
			var uid uint32
			var n int
			if err := rows.Scan(&uid, &n); err != nil {
				rows.Close()
				return 0, err
			}
			if n >= temoinThreshold {
				witnesses = append(witnesses, uid)
			}
		}
		rows.Close()
		for _, uid := range witnesses {
			isNew, err := grant(tx, uid, "temoin", fmt.Sprintf("claim:%d", c.ID), fmt.Sprintf("%d confirmations", temoinThreshold), now)
			if err != nil {
				return 0, err
			}
			if isNew {
				rec.Badges[uid] = append(rec.Badges[uid], "temoin")
			}
		}
	}
	recJSON, _ := json.Marshal(rec)
	res, err := tx.Exec(`INSERT INTO admin_actions (admin_id, type, cible, avant_json, apres_json, motif, ts) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		admin.ID, "claim."+d.Action, fmt.Sprintf("claim:%d", c.ID), string(recJSON), fmt.Sprintf(`{"statut":%q,"badges":%q}`, claimStatut, strings.Join(badges, ",")), motif, now)
	if err != nil {
		return 0, err
	}
	actionID, _ := res.LastInsertId()
	return actionID, tx.Commit()
}

// UndoDecision reverts a claim decision recorded in the journal.
func (s *Store) UndoDecision(actionID int64) error {
	var raw string
	var undone bool
	var typ string
	if err := s.db.QueryRow(`SELECT type, avant_json, annulee FROM admin_actions WHERE id = ?`, actionID).Scan(&typ, &raw, &undone); err != nil {
		return err
	}
	if undone {
		return &FieldError{"", "Cette action est déjà annulée."}
	}
	if !strings.HasPrefix(typ, "claim.") {
		return &FieldError{"", "Cette action ne s'annule pas ici."}
	}
	var rec decisionRecord
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for uid, n := range rec.Pioneer {
		if _, err := tx.Exec(`UPDATE users SET pixels_pionniers = MAX(pixels_pionniers - ?, 0) WHERE id = ?`, n, uid); err != nil {
			return err
		}
	}
	for uid, bs := range rec.Badges {
		for _, b := range bs {
			if _, err := tx.Exec(`DELETE FROM user_badges WHERE user_id = ? AND badge = ?`, uid, b); err != nil {
				return err
			}
		}
	}
	if rec.Oeuvre != 0 {
		if _, err := tx.Exec(`DELETE FROM oeuvres WHERE id = ?`, rec.Oeuvre); err != nil {
			return err
		}
	}
	for id, st := range rec.Statuts {
		if _, err := tx.Exec(`UPDATE claims SET statut = ?, badges = '', motif = '', oeuvre_id = NULL, decide_par = NULL, decide_le = NULL WHERE id = ?`, st, id); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE admin_actions SET annulee = 1 WHERE id = ?`, actionID); err != nil {
		return err
	}
	return tx.Commit()
}

// ------------------------------------------------------------------
// Conflicts

// claimPositions caches the pixels of each claim and artwork mask.
var maskCache = struct {
	sync.Mutex
	m map[string][]int32
}{m: map[string][]int32{}}

func maskPositions(m Mask, cw, ch int) []int32 {
	key, _ := json.Marshal(m)
	k := fmt.Sprintf("%d:%d:%s", cw, ch, key)
	maskCache.Lock()
	defer maskCache.Unlock()
	if p, ok := maskCache.m[k]; ok {
		return p
	}
	p, err := m.Positions(cw, ch)
	if err != nil {
		p = nil
	}
	if len(maskCache.m) > 500 {
		maskCache.m = map[string][]int32{}
	}
	maskCache.m[k] = p
	return p
}

// Conflicts finds pending claims of other players and artworks overlapping a zone.
func (s *Store) Conflicts(pos []int32, self uint32, exceptClaim int64, cw, ch int) ([]ClaimConflict, error) {
	out := []ClaimConflict{}
	pending, err := s.Claims("attente", 0, 0)
	if err != nil {
		return nil, err
	}
	for _, c := range pending {
		if c.ID == exceptClaim || c.requesterID == self && exceptClaim == 0 {
			continue
		}
		other := maskPositions(c.Mask, cw, ch)
		n := Overlap(pos, other)
		if n == 0 {
			continue
		}
		smaller := min(len(pos), len(other))
		out = append(out, ClaimConflict{ClaimID: c.ID, Titre: c.Titre, By: c.Requester, Pct: round1(float64(n) / float64(smaller)), Mask: c.Mask})
	}
	oeuvres, err := s.Oeuvres(0, false)
	if err != nil {
		return nil, err
	}
	for _, o := range oeuvres {
		other := maskPositions(o.Mask, cw, ch)
		n := Overlap(pos, other)
		if n == 0 {
			continue
		}
		smaller := min(len(pos), len(other))
		var by *PublicUser
		if len(o.Auteurs) > 0 {
			by = o.Auteurs[0].User
		}
		out = append(out, ClaimConflict{OeuvreID: o.ID, Titre: o.Titre, By: by, Pct: round1(float64(n) / float64(smaller)), Mask: o.Mask})
	}
	return out, nil
}

// Disputed is true when another pending claim covers a good part of this one.
func (c *Claim) Disputed() bool {
	for _, k := range c.Conflicts {
		if k.ClaimID != 0 && k.Pct >= conflictOverlap*100 {
			return true
		}
	}
	return false
}
