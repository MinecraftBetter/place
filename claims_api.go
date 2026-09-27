package place

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// SetBackups gives the API the backup index (claims analysis, timelapse, stats).
func (api *API) SetBackups(b *Backups) { api.backups = b }

func (api *API) index() *BackupIndex {
	if api.backups == nil {
		return nil
	}
	return api.backups.Index()
}

func (api *API) mountClaims() {
	api.mux.HandleFunc("/api/claims", api.handleClaims)
	api.mux.HandleFunc("/api/claims/", api.handleClaim)
	api.mux.HandleFunc("/api/claims/analyse", api.handleClaimAnalyse)
	api.mux.HandleFunc("/api/crop.png", api.handleCrop)
	api.mux.HandleFunc("/api/users/search", api.handleUserSearch)
	api.mux.HandleFunc("/api/oeuvres", api.handleOeuvres)
	api.mux.HandleFunc("/api/oeuvres/", api.handleOeuvre)
	api.mux.HandleFunc("/api/admin/claims", api.admin(api.handleAdminClaims))
	api.mux.HandleFunc("/api/admin/claims/", api.admin(api.handleAdminClaimDecision))
	api.mux.HandleFunc("/api/backups/status", api.handleBackupStatus)
}

// admin wraps a handler that requires the admin role.
func (api *API) admin(h func(w http.ResponseWriter, r *http.Request, u *User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := api.auth.User(r)
		if u == nil {
			writeError(w, http.StatusUnauthorized, "Connecte-toi.")
			return
		}
		if u.Role != "admin" {
			writeError(w, http.StatusForbidden, "Réservé à l'équipe.")
			return
		}
		h(w, r, u)
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 512<<10)).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "Requête illisible.")
		return false
	}
	return true
}

func writeFieldError(w http.ResponseWriter, err error) bool {
	var fe *FieldError
	if errors.As(err, &fe) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(fe)
		return true
	}
	return false
}

func (api *API) serverError(w http.ResponseWriter, what string, err error) {
	log.WithField("endpoint", "API").Error(what, ": ", err)
	writeError(w, http.StatusInternalServerError, "Erreur du serveur, réessaie plus tard.")
}

// ------------------------------------------------------------------
// Analysis cache: the same zone is analysed while the player adjusts it.

var analysisCache = struct {
	sync.Mutex
	frames int
	m      map[string]*ZoneAnalysis
}{m: map[string]*ZoneAnalysis{}}

func (api *API) analyse(m Mask) (*ZoneAnalysis, error) {
	idx := api.index()
	if idx == nil {
		return nil, nil
	}
	key, _ := json.Marshal(m)
	analysisCache.Lock()
	if analysisCache.frames != len(idx.Frames) {
		analysisCache.frames, analysisCache.m = len(idx.Frames), map[string]*ZoneAnalysis{}
	}
	a, ok := analysisCache.m[string(key)]
	analysisCache.Unlock()
	if ok {
		return a, nil
	}
	pos, err := m.Positions(idx.W, idx.H)
	if err != nil {
		return nil, err
	}
	a = AnalyzeZone(idx, pos, api.store.MigrationTime(), api.store.now())
	analysisCache.Lock()
	if len(analysisCache.m) > 200 {
		analysisCache.m = map[string]*ZoneAnalysis{}
	}
	analysisCache.m[string(key)] = a
	analysisCache.Unlock()
	return a, nil
}

// POST /api/claims/analyse {masque}
func (api *API) handleClaimAnalyse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
		return
	}
	var req struct {
		Mask Mask `json:"masque"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	cw, ch := api.canvas.Size()
	pos, err := req.Mask.Positions(cw, ch)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Zone invalide : sélectionne au moins un pixel.")
		return
	}
	a, err := api.analyse(req.Mask)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Zone invalide.")
		return
	}
	var self uint32
	if u := api.auth.User(r); u != nil {
		self = u.ID
	}
	conflicts, err := api.store.Conflicts(pos, self, 0, cw, ch)
	if err != nil {
		api.serverError(w, "Conflicts", err)
		return
	}
	b := req.Mask.Bounds(cw, ch)
	writeJSON(w, map[string]any{
		"pixels": len(pos), "x": b.Min.X, "y": b.Min.Y, "w": b.Dx(), "h": b.Dy(),
		"analyse": a, "conflits": conflicts, "sauvegardes": api.backupStatus(),
	})
}

func (api *API) backupStatus() BackupStatus {
	if api.backups == nil {
		return BackupStatus{Status: "absent"}
	}
	return api.backups.Status()
}

func (api *API) handleBackupStatus(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	writeJSON(w, api.backupStatus())
}

// claimSummary is the one line under a claim in the feed ("Sauvegardes : …").
func claimSummary(c *Claim) string {
	var parts []string
	if a := c.Analysis; a != nil {
		switch {
		case a.AlreadyThere:
			parts = append(parts, "présente dès "+frenchDate(time.UnixMilli(a.Appeared)))
		case a.Appeared != 0:
			parts = append(parts, "apparue le "+frenchDate(time.UnixMilli(a.Appeared)))
		default:
			parts = append(parts, "introuvable dans les sauvegardes")
		}
		if a.Placed > 0 {
			parts = append(parts, fmt.Sprintf("≈ %s pixels posés", frenchNumber(a.Placed)))
		}
		if len(a.Retouches) > 0 {
			parts = append(parts, fmt.Sprintf("retouchée %d fois", len(a.Retouches)))
		}
	}
	var rivals []string
	for _, k := range c.Conflicts {
		if k.ClaimID != 0 && k.By != nil && k.Pct >= conflictOverlap*100 {
			rivals = append(rivals, k.By.Pseudo)
		}
		if k.OeuvreID != 0 && k.Pct >= conflictOverlap*100 {
			rivals = append(rivals, "« "+k.Titre+" » (déjà attribuée)")
		}
	}
	s := "Sauvegardes : " + strings.Join(parts, ", ") + "."
	if len(parts) == 0 {
		s = "Analyse des sauvegardes en cours."
	}
	switch {
	case len(rivals) > 0:
		s += " Aussi revendiquée par " + strings.Join(rivals, ", ") + "."
	case c.Statut == "attente":
		s += " Personne d'autre ne la revendique."
	}
	if c.Statut == "validee" && len(c.Badges) > 0 {
		var names []string
		for _, b := range c.Badges {
			if bd := BadgeByID(b); bd != nil {
				names = append(names, bd.Name)
			}
		}
		s += " Badges : " + strings.Join(names, ", ") + "."
	}
	if c.Statut == "refusee" && c.Motif != "" {
		s = c.Motif
	}
	return s
}

func frenchNumber(n int) string {
	s := strconv.Itoa(n)
	var out []string
	for len(s) > 3 {
		out = append([]string{s[len(s)-3:]}, out...)
		s = s[:len(s)-3]
	}
	return strings.Join(append([]string{s}, out...), " ")
}

// decorate adds conflicts, the analysis (computed now if missing) and the summary.
func (api *API) decorate(cs []*Claim) {
	cw, ch := api.canvas.Size()
	for _, c := range cs {
		if c.Analysis == nil && c.Statut == "attente" {
			if a, err := api.analyse(c.Mask); err == nil && a != nil {
				c.Analysis = a
				api.store.SetClaimAnalysis(c.ID, a)
			}
		}
		if c.Statut == "attente" {
			if pos := maskPositions(c.Mask, cw, ch); pos != nil {
				if k, err := api.store.Conflicts(pos, c.requesterID, c.ID, cw, ch); err == nil {
					c.Conflicts = k
				}
			}
		}
		c.Summary = claimSummary(c)
	}
}

// GET /api/claims?statut= , POST /api/claims
func (api *API) handleClaims(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		statut := r.URL.Query().Get("statut")
		if statut != "validee" && statut != "refusee" {
			statut = "attente"
		}
		var viewer, only uint32
		if u := api.auth.User(r); u != nil {
			viewer = u.ID
			if r.URL.Query().Get("moi") == "1" {
				only = u.ID
			}
		}
		cs, err := api.store.Claims(statut, only, viewer)
		if err != nil {
			api.serverError(w, "Claims", err)
			return
		}
		api.decorate(cs)
		counts, _ := api.store.ClaimCounts()
		if cs == nil {
			cs = []*Claim{}
		}
		writeJSON(w, map[string]any{"claims": cs, "counts": counts})
	case http.MethodPost:
		u := api.auth.User(r)
		if u == nil {
			writeError(w, http.StatusUnauthorized, "Connecte-toi pour revendiquer une œuvre.")
			return
		}
		if b := u.WriteBlock(api.store.now()); b != "" {
			writeError(w, http.StatusForbidden, "Ton compte ne peut pas revendiquer pour le moment.")
			return
		}
		var n NewClaim
		if !readJSON(w, r, &n) {
			return
		}
		cw, ch := api.canvas.Size()
		pos, err := n.Mask.Positions(cw, ch)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Zone invalide : sélectionne ton œuvre.")
			return
		}
		a, _ := api.analyse(n.Mask)
		id, err := api.store.CreateClaim(u, n, pos, cw, ch, a)
		if writeFieldError(w, err) {
			return
		}
		if err != nil {
			api.serverError(w, "Create claim", err)
			return
		}
		c, _ := api.store.Claim(id, u.ID)
		for _, f := range ClaimHooks {
			f(api, c, "created", u)
		}
		writeJSON(w, map[string]any{"id": id})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
	}
}

// ClaimHooks are called when a claim is created or decided (activity, alerts…).
var ClaimHooks []func(api *API, c *Claim, event string, by *User)

// /api/claims/:id, /votes, /confirm, /cancel, /strip.png
func (api *API) handleClaim(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/claims/")
	idStr, sub, _ := strings.Cut(rest, "/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "Demande introuvable.")
		return
	}
	u := api.auth.User(r)
	var viewer uint32
	if u != nil {
		viewer = u.ID
	}
	c, err := api.store.Claim(id, viewer)
	if err != nil {
		api.serverError(w, "Claim", err)
		return
	}
	if c == nil || c.Statut == "annulee" && sub != "" {
		writeError(w, http.StatusNotFound, "Demande introuvable.")
		return
	}
	switch sub {
	case "":
		if !getOnly(w, r) {
			return
		}
		api.decorate([]*Claim{c})
		votes, err := api.store.ClaimVotes(c.ID)
		if err != nil {
			api.serverError(w, "Votes", err)
			return
		}
		writeJSON(w, map[string]any{"claim": c, "votes": votes})
	case "strip.png":
		if !getOnly(w, r) {
			return
		}
		api.serveStrip(w, c)
	case "votes", "confirm", "cancel":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
			return
		}
		if u == nil {
			writeError(w, http.StatusUnauthorized, "Connecte-toi.")
			return
		}
		var req struct {
			Type    string `json:"type"`
			Comment string `json:"commentaire"`
			Accept  bool   `json:"accepte"`
		}
		if r.ContentLength != 0 && !readJSON(w, r, &req) {
			return
		}
		switch sub {
		case "votes":
			err = api.store.Vote(c, u, req.Type, req.Comment)
		case "confirm":
			err = api.store.ConfirmCoauthor(c, u, req.Accept)
		case "cancel":
			err = api.store.CancelClaim(c, u)
		}
		if writeFieldError(w, err) {
			return
		}
		if err != nil {
			api.serverError(w, "Claim "+sub, err)
			return
		}
		c, _ = api.store.Claim(id, viewer)
		api.decorate([]*Claim{c})
		writeJSON(w, map[string]any{"claim": c})
	default:
		writeError(w, http.StatusNotFound, "Introuvable.")
	}
}

var stripCache = struct {
	sync.Mutex
	m map[string][]byte
}{m: map[string][]byte{}}

func (api *API) serveStrip(w http.ResponseWriter, c *Claim) {
	idx := api.index()
	if idx == nil {
		writeError(w, http.StatusServiceUnavailable, "Les sauvegardes sont en cours d'indexation.")
		return
	}
	key := fmt.Sprintf("%d:%d", c.ID, len(idx.Frames))
	stripCache.Lock()
	b, ok := stripCache.m[key]
	stripCache.Unlock()
	var times []int64
	if !ok {
		pos, err := c.Mask.Positions(idx.W, idx.H)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Zone invalide.")
			return
		}
		a := AnalyzeZone(idx, pos, api.store.MigrationTime(), api.store.now())
		b, times = ZoneStrip(idx, pos, a.KeyFrames(), 136)
		tj, _ := json.Marshal(times)
		stripCache.Lock()
		if len(stripCache.m) > 100 {
			stripCache.m = map[string][]byte{}
		}
		stripCache.m[key] = b
		stripCache.m[key+":t"] = tj
		stripCache.Unlock()
	}
	stripCache.Lock()
	w.Header().Set("X-Frame-Times", string(stripCache.m[key+":t"]))
	stripCache.Unlock()
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Write(b)
}

// GET /api/crop.png?x=&y=&w=&h=&z= — a scaled crop of the live canvas (thumbnails).
func (api *API) handleCrop(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	q := r.URL.Query()
	atoi := func(k string, def int) int {
		v, err := strconv.Atoi(q.Get(k))
		if err != nil {
			return def
		}
		return v
	}
	cw, ch := api.canvas.Size()
	rect := image.Rect(atoi("x", 0), atoi("y", 0), atoi("x", 0)+atoi("w", cw), atoi("y", 0)+atoi("h", ch)).Intersect(image.Rect(0, 0, cw, ch))
	if rect.Empty() {
		writeError(w, http.StatusBadRequest, "Zone hors du canvas.")
		return
	}
	z := atoi("z", 1)
	z = max(1, min(z, 16))
	for rect.Dx()*z > 1024 || rect.Dy()*z > 1024 {
		if z == 1 {
			break
		}
		z--
	}
	img := image.NewNRGBA(image.Rect(0, 0, rect.Dx()*z, rect.Dy()*z))
	api.canvas.mu.RLock()
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			p := api.canvas.img.Pix[(y*cw+x)*4:]
			for dy := 0; dy < z; dy++ {
				row := img.Pix[((y-rect.Min.Y)*z+dy)*img.Stride:]
				for dx := 0; dx < z; dx++ {
					o := ((x-rect.Min.X)*z + dx) * 4
					row[o], row[o+1], row[o+2], row[o+3] = p[0], p[1], p[2], 255
				}
			}
		}
	}
	api.canvas.mu.RUnlock()
	b := encodePNG(img)
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=15")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	w.Write(b)
}

// GET /api/users/search?q= — players whose pseudo starts with or contains q.
func (api *API) handleUserSearch(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, map[string]any{"users": []any{}})
		return
	}
	like := "%" + strings.NewReplacer("%", "", "_", "").Replace(Slugify(q)) + "%"
	rows, err := api.store.db.Query(`SELECT `+userCols+` FROM users u WHERE (slug LIKE ? OR pseudo LIKE ?) AND statut != 'banni'
		ORDER BY slug LIKE ? DESC, pixels_poses DESC LIMIT 8`, like, "%"+q+"%", Slugify(q)+"%")
	if err != nil {
		api.serverError(w, "Search", err)
		return
	}
	defer rows.Close()
	out := []*PublicUser{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			api.serverError(w, "Search", err)
			return
		}
		out = append(out, u.Public())
	}
	writeJSON(w, map[string]any{"users": out})
}

// ------------------------------------------------------------------
// Admin

// GET /api/admin/claims?statut=
func (api *API) handleAdminClaims(w http.ResponseWriter, r *http.Request, u *User) {
	if !getOnly(w, r) {
		return
	}
	statut := r.URL.Query().Get("statut")
	if statut != "validee" && statut != "refusee" {
		statut = "attente"
	}
	cs, err := api.store.Claims(statut, 0, u.ID)
	if err != nil {
		api.serverError(w, "Admin claims", err)
		return
	}
	api.decorate(cs)
	counts, _ := api.store.ClaimCounts()
	if cs == nil {
		cs = []*Claim{}
	}
	type adminClaim struct {
		*Claim
		Disputed bool        `json:"a_departager"`
		Votes    []ClaimVote `json:"votes"`
		Action   int64       `json:"action_id,omitempty"`
	}
	out := make([]adminClaim, 0, len(cs))
	for _, c := range cs {
		votes, _ := api.store.ClaimVotes(c.ID)
		ac := adminClaim{Claim: c, Disputed: c.Disputed(), Votes: votes}
		if c.Statut != "attente" {
			api.store.db.QueryRow(`SELECT id FROM admin_actions WHERE cible = ? AND annulee = 0 ORDER BY id DESC LIMIT 1`, fmt.Sprintf("claim:%d", c.ID)).Scan(&ac.Action)
		}
		out = append(out, ac)
	}
	writeJSON(w, map[string]any{"claims": out, "counts": counts, "sauvegardes": api.backupStatus()})
}

// POST /api/admin/claims/:id/decision, POST /api/admin/claims/:id/undo
func (api *API) handleAdminClaimDecision(w http.ResponseWriter, r *http.Request, admin *User) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/admin/claims/")
	idStr, sub, _ := strings.Cut(rest, "/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "Demande introuvable.")
		return
	}
	c, err := api.store.Claim(id, admin.ID)
	if err != nil || c == nil {
		writeError(w, http.StatusNotFound, "Demande introuvable.")
		return
	}
	switch sub {
	case "decision":
		var d Decision
		if !readJSON(w, r, &d) {
			return
		}
		if c.Analysis == nil {
			if a, err := api.analyse(c.Mask); err == nil {
				c.Analysis = a
			}
		}
		cw, ch := api.canvas.Size()
		actionID, err := api.store.Decide(admin, c, d, cw, ch)
		if writeFieldError(w, err) {
			return
		}
		if err != nil {
			api.serverError(w, "Decision", err)
			return
		}
		api.RefreshOeuvres()
		c, _ = api.store.Claim(id, admin.ID)
		for _, f := range ClaimHooks {
			f(api, c, "decided:"+d.Action, admin)
		}
		writeJSON(w, map[string]any{"claim": c, "action_id": actionID})
	case "undo":
		var action int64
		api.store.db.QueryRow(`SELECT id FROM admin_actions WHERE cible = ? AND annulee = 0 ORDER BY id DESC LIMIT 1`, fmt.Sprintf("claim:%d", id)).Scan(&action)
		if action == 0 {
			writeError(w, http.StatusBadRequest, "Rien à annuler.")
			return
		}
		if err := api.store.UndoDecision(action); writeFieldError(w, err) {
			return
		} else if err != nil {
			api.serverError(w, "Undo", err)
			return
		}
		api.RefreshOeuvres()
		c, _ = api.store.Claim(id, admin.ID)
		writeJSON(w, map[string]any{"claim": c})
	default:
		writeError(w, http.StatusNotFound, "Introuvable.")
	}
}

// RefreshOeuvres rebuilds the pixel → artwork index.
func (api *API) RefreshOeuvres() {
	os, err := api.store.Oeuvres(0, false)
	if err != nil {
		log.WithField("endpoint", "API").Error("Oeuvres: ", err)
		return
	}
	cw, ch := api.canvas.Size()
	api.oeuvres.Rebuild(os, cw, ch)
	if api.community != nil {
		api.community.refreshAuthors(os)
	}
}

// ------------------------------------------------------------------
// Artworks (public)

func (api *API) handleOeuvres(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	var uid uint32
	if slug := r.URL.Query().Get("user"); slug != "" {
		u, _ := api.store.UserBySlug(slug)
		if u == nil {
			writeJSON(w, map[string]any{"oeuvres": []any{}})
			return
		}
		uid = u.ID
	}
	os, err := api.store.Oeuvres(uid, false)
	if err != nil {
		api.serverError(w, "Oeuvres", err)
		return
	}
	if os == nil {
		os = []*Oeuvre{}
	}
	for _, f := range OeuvreDecorators {
		f(api, os, api.auth.User(r))
	}
	writeJSON(w, map[string]any{"oeuvres": os})
}

// OeuvreDecorators add fields to artworks (likes, exhibitions…) in later phases.
var OeuvreDecorators []func(api *API, os []*Oeuvre, viewer *User)

// OeuvreRoutes handles /api/oeuvres/:id/<sub> for later phases.
var OeuvreRoutes = map[string]func(api *API, w http.ResponseWriter, r *http.Request, o *Oeuvre){}

func (api *API) handleOeuvre(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/oeuvres/")
	idStr, sub, _ := strings.Cut(rest, "/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "Œuvre introuvable.")
		return
	}
	o, err := api.store.Oeuvre(id)
	if err != nil {
		api.serverError(w, "Oeuvre", err)
		return
	}
	viewer := api.auth.User(r)
	if o == nil || o.Statut != "active" && (viewer == nil || viewer.Role != "admin") {
		writeError(w, http.StatusNotFound, "Œuvre introuvable.")
		return
	}
	if sub == "" {
		if !getOnly(w, r) {
			return
		}
		for _, f := range OeuvreDecorators {
			f(api, []*Oeuvre{o}, viewer)
		}
		contrib, hist := api.oeuvreStory(o)
		writeJSON(w, map[string]any{"oeuvre": o, "analyse": o.Analysis(), "contributeurs": contrib, "histoire": hist,
			"auteur": isAuthor(o, viewer), "sauvegardes": api.index() != nil})
		return
	}
	if h, ok := OeuvreRoutes[sub]; ok {
		h(api, w, r, o)
		return
	}
	writeError(w, http.StatusNotFound, "Introuvable.")
}
