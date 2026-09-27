package place

// Admin — moderation (a-moderation): avatars, bios, artwork and museum names, sounds,
// reported by players or caught by the automatic filter. « Masquer » puts the default
// value back and tells the player. Drawings on the canvas never come here.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Report struct {
	ID      int64       `json:"id"`
	Type    string      `json:"type"` // avatar | bio | pseudo | oeuvre | banniere | musee | son | livre_or
	Cible   string      `json:"cible"`
	User    *PublicUser `json:"user"` // whose content it is
	Author  *PublicUser `json:"auteur,omitempty"`
	Source  string      `json:"source"` // joueur | filtre
	Raison  string      `json:"raison"`
	Contenu string      `json:"contenu"`
	Statut  string      `json:"statut"` // attente | garde | masque
	TS      int64       `json:"ts"`
	uid     uint32
	aid     uint32
}

var linkRE = regexp.MustCompile(`(?i)(https?://|www\.|\b[a-z0-9-]+\.(com|fr|net|org|io|gg|xyz|ly)\b)`)

func (s *Store) AddReport(r Report) (int64, error) {
	// one pending report per content is enough
	var existing int64
	s.db.QueryRow(`SELECT id FROM reports WHERE type = ? AND cible = ? AND statut = 'attente'`, r.Type, r.Cible).Scan(&existing)
	if existing != 0 {
		s.db.Exec(`UPDATE reports SET contenu = ?, raison = ?, ts = ? WHERE id = ?`, r.Contenu, r.Raison, s.now().UnixMilli(), existing)
		return existing, nil
	}
	res, err := s.db.Exec(`INSERT INTO reports (type, cible, user_id, auteur_id, source, raison, contenu, ts) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Type, r.Cible, r.uid, r.aid, r.Source, r.Raison, r.Contenu, s.now().UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// filterWords returns the forbidden word found in a text, if any.
func (st Settings) filterWords(text string) string {
	if !st.WordsFilter {
		return ""
	}
	lower := " " + strings.ToLower(Slugify(text)) + " "
	lower = strings.ReplaceAll(lower, "-", " ")
	for _, w := range strings.Split(st.Words, ",") {
		w = strings.TrimSpace(strings.ToLower(Slugify(w)))
		if w != "" && w != "joueur" && strings.Contains(lower, " "+strings.ReplaceAll(w, "-", " ")+" ") {
			return w
		}
	}
	return ""
}

func init() {
	// The automatic filter looks at profile changes.
	ProfileHooks = append(ProfileHooks, func(api *API, u *User, what string) {
		st := api.Settings()
		switch what {
		case "profile":
			if w := st.filterWords(u.Pseudo); w != "" {
				api.store.AddReport(Report{Type: "pseudo", Cible: fmt.Sprintf("user:%d", u.ID), uid: u.ID, Source: "filtre", Raison: "Mot interdit : « " + w + " »", Contenu: u.Pseudo})
			}
			if w := st.filterWords(u.Bio); w != "" {
				api.store.AddReport(Report{Type: "bio", Cible: fmt.Sprintf("user:%d", u.ID), uid: u.ID, Source: "filtre", Raison: "Mot interdit : « " + w + " »", Contenu: u.Bio})
			} else if st.NoLinks && linkRE.MatchString(u.Bio) {
				api.store.AddReport(Report{Type: "bio", Cible: fmt.Sprintf("user:%d", u.ID), uid: u.ID, Source: "filtre", Raison: "Lien dans la bio", Contenu: u.Bio})
			}
		case "avatar":
			if st.AvatarCheck {
				api.store.AddReport(Report{Type: "avatar", Cible: fmt.Sprintf("user:%d", u.ID), uid: u.ID, Source: "filtre", Raison: "Nouvel avatar à vérifier", Contenu: u.AvatarURL})
			}
		case "banner":
			if st.AvatarCheck {
				api.store.AddReport(Report{Type: "banniere", Cible: fmt.Sprintf("user:%d", u.ID), uid: u.ID, Source: "filtre", Raison: "Nouvelle bannière à vérifier", Contenu: u.BannerURL})
			}
		}
	})
}

func (api *API) mountModeration() {
	api.mux.HandleFunc("/api/reports", api.handleReportCreate)
	api.mux.HandleFunc("/api/admin/reports", api.admin(api.handleReports))
	api.mux.HandleFunc("/api/admin/reports/", api.admin(api.handleReportDecision))
}

// POST /api/reports {type, cible, raison} — a player reports a content.
func (api *API) handleReportCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
		return
	}
	u := api.auth.User(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "Connecte-toi pour signaler.")
		return
	}
	var req struct {
		Type   string `json:"type"`
		Cible  string `json:"cible"`
		Raison string `json:"raison"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	raison := strings.TrimSpace(req.Raison)
	if utf8.RuneCountInString(raison) > 280 {
		writeFieldError(w, &FieldError{"raison", "280 caractères au maximum."})
		return
	}
	rep := Report{Type: req.Type, Cible: req.Cible, aid: u.ID, Source: "joueur", Raison: raison}
	kind, idStr, _ := strings.Cut(req.Cible, ":")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	switch {
	case kind == "user" && (req.Type == "avatar" || req.Type == "bio" || req.Type == "pseudo" || req.Type == "banniere"):
		t, _ := api.store.UserByID(uint32(id))
		if t == nil {
			writeError(w, http.StatusNotFound, "Contenu introuvable.")
			return
		}
		rep.uid = t.ID
		rep.Contenu = map[string]string{"avatar": t.AvatarURL, "bio": t.Bio, "pseudo": t.Pseudo, "banniere": t.BannerURL}[req.Type]
	case kind == "oeuvre" && req.Type == "oeuvre":
		o, _ := api.store.Oeuvre(id)
		if o == nil {
			writeError(w, http.StatusNotFound, "Contenu introuvable.")
			return
		}
		rep.Contenu = o.Titre
		if len(o.Auteurs) > 0 {
			rep.uid = o.Auteurs[0].userID
		}
	default:
		if h, ok := ReportTargets[req.Type]; ok && h(api, &rep, id) {
			break
		}
		writeError(w, http.StatusBadRequest, "Ce contenu ne peut pas être signalé.")
		return
	}
	if _, err := api.store.AddReport(rep); err != nil {
		api.serverError(w, "Report", err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// ReportTargets resolve other reportable contents (museum names, sounds, guestbook…).
var ReportTargets = map[string]func(api *API, r *Report, id int64) bool{}

func (api *API) reports(statut, typ string) ([]*Report, error) {
	q := `SELECT id, type, cible, COALESCE(user_id, 0), auteur_id, source, raison, contenu, statut, ts FROM reports WHERE statut = ?`
	args := []any{statut}
	if typ != "" {
		q += ` AND type IN (?` + strings.Repeat(",?", strings.Count(typ, ",")) + `)`
		for _, t := range strings.Split(typ, ",") {
			args = append(args, t)
		}
	}
	rows, err := api.store.db.Query(q+` ORDER BY id DESC LIMIT 200`, args...)
	if err != nil {
		return nil, err
	}
	var out []*Report
	var ids []uint32
	for rows.Next() {
		r := &Report{}
		if err := rows.Scan(&r.ID, &r.Type, &r.Cible, &r.uid, &r.aid, &r.Source, &r.Raison, &r.Contenu, &r.Statut, &r.TS); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
		ids = append(ids, r.uid, r.aid)
	}
	rows.Close()
	users, _ := api.store.UsersByIDs(ids)
	for _, r := range out {
		r.User = users[r.uid].Public()
		r.Author = users[r.aid].Public()
	}
	if out == nil {
		out = []*Report{}
	}
	return out, nil
}

// GET /api/admin/reports?statut=attente&type=avatar,banniere
func (api *API) handleReports(w http.ResponseWriter, r *http.Request, admin *User) {
	if !getOnly(w, r) {
		return
	}
	statut := r.URL.Query().Get("statut")
	if statut != "garde" && statut != "masque" {
		statut = "attente"
	}
	list, err := api.reports(statut, r.URL.Query().Get("type"))
	if err != nil {
		api.serverError(w, "Reports", err)
		return
	}
	st := api.Settings()
	writeJSON(w, map[string]any{"reports": list, "filtres": map[string]any{
		"words_filter": st.WordsFilter, "no_links": st.NoLinks, "avatar_check": st.AvatarCheck, "sounds_check": st.SoundsCheck, "words": st.Words,
	}})
}

type moderationRecord struct {
	Report int64  `json:"report"`
	Field  string `json:"field"`
	Value  string `json:"value"`
	Statut string `json:"statut"`
}

// hideContent resets a content to its default value and returns what it was.
func (api *API) hideContent(rep *Report) (moderationRecord, error) {
	rec := moderationRecord{Report: rep.ID, Statut: rep.Statut}
	kind, idStr, _ := strings.Cut(rep.Cible, ":")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	switch {
	case kind == "user":
		u, _ := api.store.UserByID(uint32(id))
		if u == nil {
			return rec, fmt.Errorf("user not found")
		}
		switch rep.Type {
		case "avatar":
			rec.Field, rec.Value = "avatar_url", u.AvatarURL
			api.store.db.Exec(`UPDATE users SET avatar_url = '' WHERE id = ?`, u.ID)
		case "bio":
			rec.Field, rec.Value = "bio", u.Bio
			api.store.db.Exec(`UPDATE users SET bio = '' WHERE id = ?`, u.ID)
		case "banniere":
			rec.Field, rec.Value = "banniere_url", u.BannerURL
			api.store.db.Exec(`UPDATE users SET banniere_url = '', banner = 'desert' WHERE id = ?`, u.ID)
		case "pseudo":
			rec.Field, rec.Value = "pseudo", u.Pseudo
			newPseudo := fmt.Sprintf("Joueur %d", u.ID)
			slug, _ := api.store.freeSlug(Slugify(newPseudo), u.ID)
			api.store.db.Exec(`UPDATE users SET pseudo = ?, slug = ? WHERE id = ?`, newPseudo, slug, u.ID)
			rec.Value = u.Pseudo + "\x00" + u.Slug
		}
	case kind == "oeuvre":
		o, _ := api.store.Oeuvre(id)
		if o == nil {
			return rec, fmt.Errorf("artwork not found")
		}
		rec.Field, rec.Value = "oeuvre_titre", o.Titre
		api.store.db.Exec(`UPDATE oeuvres SET titre = 'Sans titre' WHERE id = ?`, o.ID)
	default:
		if h, ok := HideHandlers[rep.Type]; ok {
			return h(api, rep, rec)
		}
		return rec, fmt.Errorf("cannot hide %s", rep.Type)
	}
	return rec, nil
}

// HideHandlers hide other contents (museums…) and UnhideHandlers put them back.
var (
	HideHandlers   = map[string]func(api *API, rep *Report, rec moderationRecord) (moderationRecord, error){}
	UnhideHandlers = map[string]func(api *API, rec moderationRecord, cible string) error{}
)

// POST /api/admin/reports/:id/{garder,masquer,message}
func (api *API) handleReportDecision(w http.ResponseWriter, r *http.Request, admin *User) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
		return
	}
	idStr, sub, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/admin/reports/"), "/")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	list, err := api.store.db.Query(`SELECT id FROM reports WHERE id = ?`, id)
	if err == nil {
		list.Close()
	}
	var rep *Report
	for _, st := range []string{"attente", "garde", "masque"} {
		all, _ := api.reports(st, "")
		for _, x := range all {
			if x.ID == id {
				rep = x
			}
		}
	}
	if rep == nil {
		writeError(w, http.StatusNotFound, "Signalement introuvable.")
		return
	}
	var req struct {
		Message string `json:"message"`
	}
	if r.ContentLength != 0 && !readJSON(w, r, &req) {
		return
	}
	now := api.store.now().UnixMilli()
	label := map[string]string{"avatar": "avatar", "bio": "bio", "pseudo": "pseudo", "banniere": "bannière", "oeuvre": "titre d'œuvre", "musee": "musée", "son": "son", "livre_or": "message du livre d'or"}[rep.Type]
	who := ""
	if rep.User != nil {
		who = rep.User.Pseudo
	}
	switch sub {
	case "garder":
		api.store.db.Exec(`UPDATE reports SET statut = 'garde', decide_par = ?, decide_le = ? WHERE id = ?`, admin.ID, now, id)
		rj, _ := json.Marshal(moderationRecord{Report: id, Statut: rep.Statut})
		api.logAction(admin, "moderation.garder", fmt.Sprintf("report:%d", id), string(rj), "", fmt.Sprintf("%s de %s gardé", label, who))
	case "masquer":
		rec, err := api.hideContent(rep)
		if err != nil {
			api.serverError(w, "Hide", err)
			return
		}
		api.store.db.Exec(`UPDATE reports SET statut = 'masque', decide_par = ?, decide_le = ? WHERE id = ?`, admin.ID, now, id)
		rj, _ := json.Marshal(rec)
		api.logAction(admin, "moderation.masquer", rep.Cible, string(rj), "", fmt.Sprintf("%s de %s masqué", label, who))
		if rep.uid != 0 {
			api.community.alert(rep.uid, "moderation", map[string]any{"type": rep.Type, "label": label, "texte": "L'équipe a masqué ton " + label + " : il ne respectait pas les règles du canvas."})
			if u, _ := api.store.UserByID(rep.uid); u != nil {
				api.hub.UpdateUser(u)
			}
		}
	case "message":
		msg := strings.TrimSpace(req.Message)
		if msg == "" || rep.uid == 0 {
			writeFieldError(w, &FieldError{"message", "Écris un message."})
			return
		}
		api.community.alert(rep.uid, "message_equipe", map[string]any{"texte": msg, "par": admin.Public()})
		api.logAction(admin, "moderation.message", fmt.Sprintf("user:%d", rep.uid), "", "", fmt.Sprintf("message à %s : %s", who, msg))
	default:
		writeError(w, http.StatusNotFound, "Introuvable.")
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (api *API) undoModeration(cible, raw string) error {
	var rec moderationRecord
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		return err
	}
	kind, idStr, _ := strings.Cut(cible, ":")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	switch rec.Field {
	case "avatar_url", "bio", "banniere_url":
		api.store.db.Exec(`UPDATE users SET `+rec.Field+` = ? WHERE id = ?`, rec.Value, id)
		if rec.Field == "banniere_url" && rec.Value != "" {
			api.store.db.Exec(`UPDATE users SET banner = 'custom' WHERE id = ?`, id)
		}
	case "pseudo":
		pseudo, slug, _ := strings.Cut(rec.Value, "\x00")
		free, _ := api.store.freeSlug(slug, uint32(id))
		api.store.db.Exec(`UPDATE users SET pseudo = ?, slug = ? WHERE id = ?`, pseudo, free, id)
	case "oeuvre_titre":
		api.store.db.Exec(`UPDATE oeuvres SET titre = ? WHERE id = ?`, rec.Value, id)
	case "":
		// « garder »: nothing to put back
	default:
		if h, ok := UnhideHandlers[rec.Field]; ok {
			if err := h(api, rec, cible); err != nil {
				return err
			}
		}
	}
	_ = kind
	api.store.db.Exec(`UPDATE reports SET statut = 'attente', decide_par = NULL, decide_le = NULL WHERE id = ?`, rec.Report)
	return nil
}
