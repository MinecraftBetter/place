package place

// « Demander à rejoindre » a build: a player who helped asks to be added to a validated
// artwork, or to a claim still waiting for a decision. Its authors (or an admin) accept or
// refuse. demandes_ajout.cible is "oeuvre:<id>" or "claim:<id>".

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxDemandesAttente = 10

type DemandeAjout struct {
	ID      int64       `json:"id"`
	Cible   string      `json:"cible"`
	User    *PublicUser `json:"user"`
	Message string      `json:"message"`
	Statut  string      `json:"statut"` // attente | acceptee | refusee
	CreeLe  int64       `json:"cree_le"`
	uid     uint32
}

// build is what a request points at, once resolved.
type build struct {
	cible   string
	titre   string
	href    string
	authors []uint32 // who may accept
	members []uint32 // already in (authors, and co-authors not confirmed yet)
	claim   *Claim
	oeuvre  *Oeuvre
}

// buildOf resolves a target; nil when it doesn't exist or no longer takes requests.
// A validated claim becomes its artwork.
func (api *API) buildOf(cible string) (*build, error) {
	kind, idStr, _ := strings.Cut(cible, ":")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, nil
	}
	switch kind {
	case "claim":
		c, err := api.store.Claim(id, 0)
		if err != nil || c == nil {
			return nil, err
		}
		if c.Statut == "validee" && c.OeuvreID != 0 {
			return api.buildOf(fmt.Sprintf("oeuvre:%d", c.OeuvreID))
		}
		if c.Statut != "attente" {
			return nil, nil
		}
		b := &build{cible: fmt.Sprintf("claim:%d", c.ID), titre: c.Titre, href: "/revendications", claim: c,
			authors: []uint32{c.requesterID}, members: []uint32{c.requesterID}}
		for _, ca := range c.Coauthors {
			b.members = append(b.members, ca.userID)
			if ca.Confirmed {
				b.authors = append(b.authors, ca.userID)
			}
		}
		return b, nil
	case "oeuvre":
		o, err := api.store.Oeuvre(id)
		if err != nil || o == nil || o.Statut != "active" {
			return nil, err
		}
		b := &build{cible: fmt.Sprintf("oeuvre:%d", o.ID), titre: o.Titre, href: fmt.Sprintf("/oeuvre/%d", o.ID), oeuvre: o}
		for _, a := range o.Auteurs {
			b.authors = append(b.authors, a.userID)
		}
		b.members = b.authors
		return b, nil
	}
	return nil, nil
}

func hasID(l []uint32, id uint32) bool {
	for _, x := range l {
		if x == id {
			return true
		}
	}
	return false
}

func (b *build) canDecide(u *User) bool {
	return u != nil && (u.Role == "admin" || hasID(b.authors, u.ID))
}

func (s *Store) demandes(where string, args ...any) ([]*DemandeAjout, error) {
	rows, err := s.db.Query(`SELECT id, cible, user_id, message, statut, cree_le FROM demandes_ajout WHERE `+where+` ORDER BY id DESC LIMIT 200`, args...)
	if err != nil {
		return nil, err
	}
	out := []*DemandeAjout{}
	var uids []uint32
	for rows.Next() {
		d := &DemandeAjout{}
		if err := rows.Scan(&d.ID, &d.Cible, &d.uid, &d.Message, &d.Statut, &d.CreeLe); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, d)
		uids = append(uids, d.uid)
	}
	rows.Close()
	users, err := s.UsersByIDs(uids)
	for _, d := range out {
		d.User = users[d.uid].Public()
	}
	return out, err
}

func (api *API) mountRejoindre() {
	api.mux.HandleFunc("/api/rejoindre", api.handleRejoindre)
	api.mux.HandleFunc("/api/rejoindre/", api.handleRejoindreDecision)
}

// GET /api/rejoindre — the requests waiting for me (as an author, or all of them for an
// admin) and my own requests. POST /api/rejoindre {cible, message} — ask to join a build.
func (api *API) handleRejoindre(w http.ResponseWriter, r *http.Request) {
	u := api.auth.User(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "Connecte-toi.")
		return
	}
	switch r.Method {
	case http.MethodGet:
		mine, err := api.store.demandes(`user_id = ?`, u.ID)
		if err != nil {
			api.serverError(w, "Rejoindre", err)
			return
		}
		for _, d := range mine {
			if b, _ := api.buildOf(d.Cible); b != nil && d.Statut == "attente" {
				d.Cible = b.cible
			}
		}
		pending, err := api.store.demandes(`statut = 'attente'`)
		if err != nil {
			api.serverError(w, "Rejoindre", err)
			return
		}
		// ponytail: resolves every pending request on each call; index them by author if there are ever hundreds
		toDecide := []*DemandeAjout{}
		for _, d := range pending {
			if b, _ := api.buildOf(d.Cible); b != nil && b.canDecide(u) {
				d.Cible = b.cible // a claim validated since then: its artwork
				toDecide = append(toDecide, d)
			}
		}
		writeJSON(w, map[string]any{"a_decider": toDecide, "miennes": mine})
	case http.MethodPost:
		var req struct {
			Cible   string `json:"cible"`
			Message string `json:"message"`
		}
		if !readJSON(w, r, &req) {
			return
		}
		if u.WriteBlock(api.store.now()) != "" {
			writeError(w, http.StatusForbidden, "Ton compte ne peut pas faire de demande pour l'instant.")
			return
		}
		b, err := api.buildOf(req.Cible)
		if err != nil {
			api.serverError(w, "Rejoindre", err)
			return
		}
		if b == nil {
			writeError(w, http.StatusNotFound, "Ce build n'accepte plus de demande.")
			return
		}
		msg := strings.TrimSpace(req.Message)
		if utf8.RuneCountInString(msg) > 280 {
			writeFieldError(w, &FieldError{"message", "280 caractères au maximum."})
			return
		}
		if bad := api.Settings().filterWords(msg); bad != "" {
			writeFieldError(w, &FieldError{"message", "Ce message contient un mot refusé par le filtre."})
			return
		}
		if hasID(b.members, u.ID) {
			writeFieldError(w, &FieldError{"", "Tu fais déjà partie des auteurs."})
			return
		}
		waiting, err := api.store.demandes(`user_id = ? AND statut = 'attente'`, u.ID)
		if err != nil {
			api.serverError(w, "Rejoindre", err)
			return
		}
		same := false
		for _, d := range waiting { // stored as a claim that may be an artwork by now
			if o, _ := api.buildOf(d.Cible); o != nil && o.cible == b.cible {
				same = true
			}
		}
		var refused int
		api.store.db.QueryRow(`SELECT COUNT(*) FROM demandes_ajout WHERE user_id = ? AND cible = ? AND statut = 'refusee'`, u.ID, b.cible).Scan(&refused)
		switch {
		case same:
			writeFieldError(w, &FieldError{"", "Ta demande est déjà envoyée."})
			return
		case refused > 0:
			writeFieldError(w, &FieldError{"", "Les auteurs ont déjà refusé ta demande."})
			return
		case len(waiting) >= maxDemandesAttente:
			writeFieldError(w, &FieldError{"", fmt.Sprintf("%d demandes en attente au maximum.", maxDemandesAttente)})
			return
		}
		res, err := api.store.db.Exec(`INSERT INTO demandes_ajout (cible, user_id, message, cree_le) VALUES (?, ?, ?, ?)`,
			b.cible, u.ID, msg, api.store.now().UnixMilli())
		if err != nil {
			api.serverError(w, "Rejoindre", err)
			return
		}
		id, _ := res.LastInsertId()
		if api.community != nil {
			for _, a := range b.authors {
				api.community.alert(a, "demande_ajout", map[string]any{"demande": id, "titre": b.titre, "by": u.Public(), "message": msg, "href": b.href})
			}
		}
		writeJSON(w, map[string]any{"ok": true, "id": id})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
	}
}

// POST /api/rejoindre/:id {accepte} — an author (or an admin) answers.
func (api *API) handleRejoindreDecision(w http.ResponseWriter, r *http.Request) {
	u := api.auth.User(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "Connecte-toi.")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
		return
	}
	id, _ := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/rejoindre/"), 10, 64)
	var req struct {
		Accept bool `json:"accepte"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	ds, err := api.store.demandes(`id = ?`, id)
	if err != nil {
		api.serverError(w, "Rejoindre", err)
		return
	}
	if len(ds) == 0 {
		writeError(w, http.StatusNotFound, "Demande introuvable.")
		return
	}
	d := ds[0]
	if d.Statut != "attente" {
		writeFieldError(w, &FieldError{"", "Cette demande a déjà reçu une réponse."})
		return
	}
	b, err := api.buildOf(d.Cible)
	if err != nil {
		api.serverError(w, "Rejoindre", err)
		return
	}
	if b == nil {
		writeError(w, http.StatusNotFound, "Ce build n'existe plus.")
		return
	}
	if !b.canDecide(u) {
		writeError(w, http.StatusForbidden, "Seuls les auteurs peuvent répondre.")
		return
	}
	statut := "refusee"
	if req.Accept {
		statut = "acceptee"
		switch {
		case b.oeuvre != nil:
			_, err = api.store.db.Exec(`INSERT OR IGNORE INTO oeuvre_auteurs (oeuvre_id, user_id, role, pixels_pionniers) VALUES (?, ?, 'co_auteur', 0)`, b.oeuvre.ID, d.uid)
		case len(b.claim.Coauthors) >= maxCoauthors:
			writeFieldError(w, &FieldError{"", fmt.Sprintf("%d co-auteurs au maximum.", maxCoauthors)})
			return
		default:
			_, err = api.store.db.Exec(`INSERT OR IGNORE INTO claim_coauteurs (claim_id, user_id, poids, confirme) VALUES (?, ?, 1, 1)`, b.claim.ID, d.uid)
		}
		if err != nil {
			api.serverError(w, "Rejoindre", err)
			return
		}
	}
	if _, err := api.store.db.Exec(`UPDATE demandes_ajout SET statut = ?, decide_par = ?, decide_le = ? WHERE id = ?`, statut, u.ID, api.store.now().UnixMilli(), d.ID); err != nil {
		api.serverError(w, "Rejoindre", err)
		return
	}
	if api.community != nil {
		if b.oeuvre != nil && req.Accept { // retouch alerts know the new author
			os, _ := api.store.Oeuvres(0, false)
			api.community.refreshAuthors(os)
		}
		kind := map[bool]string{true: "ajout_accepte", false: "ajout_refuse"}[req.Accept]
		api.community.alert(d.uid, kind, map[string]any{"demande": d.ID, "titre": b.titre, "by": u.Public(), "href": b.href})
	}
	writeJSON(w, map[string]any{"ok": true, "statut": statut})
}
