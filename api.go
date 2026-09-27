package place

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

// API serves the JSON endpoints under /api/.
type API struct {
	canvas    *Canvas
	store     *Store
	auth      *Auth
	hub       *Hub
	mux       *http.ServeMux
	mediaDir  string // uploads (avatars…); empty disables them
	backups   *Backups
	oeuvres   *OeuvreIndex
	community *Community
}

func NewAPI(c *Canvas, st *Store, a *Auth, h *Hub) *API {
	api := &API{canvas: c, store: st, auth: a, hub: h, mux: http.NewServeMux(), oeuvres: &OeuvreIndex{}}
	api.mux.HandleFunc("/api/me", api.handleMe)
	api.mux.HandleFunc("/api/pixel", api.handlePixel)
	api.mux.HandleFunc("/api/owners", api.handleOwners)
	api.mux.HandleFunc("/api/users", api.handleUsers)
	api.mux.HandleFunc("/api/users/", api.handleUserProfile)
	api.mux.HandleFunc("/api/me/profile", api.handleMeProfile)
	api.mux.HandleFunc("/api/me/avatar", api.handleMeAvatar)
	api.mux.HandleFunc("/api/me/banner", api.handleMeBanner)
	api.mountClaims()
	api.mountCommunity()
	api.mountSnapshots()
	api.mountAdmin()
	api.RefreshOeuvres()
	return api
}

// SetMediaDir enables uploads, stored under dir and served at /media/.
func (api *API) SetMediaDir(dir string) { api.mediaDir = dir }

// Handle registers extra API routes (later phases).
func (api *API) Handle(pattern string, h http.HandlerFunc) { api.mux.HandleFunc(pattern, h) }

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "Origine refusée.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	api.mux.ServeHTTP(w, r)
}

// getOnly answers 405 to anything but GET/HEAD.
func getOnly(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.WithField("endpoint", "API").Error(err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

type meUser struct {
	*PublicUser
	Role string `json:"role"`
}

func (api *API) handleMe(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	width, height := api.canvas.Size()
	res := map[string]any{
		"user":     nil,
		"cooldown": api.hub.Cooldown().Seconds(),
		"ready_in": 0.0,
		"auth":     api.auth.ProviderNames(),
		"width":    width,
		"height":   height,
	}
	if u := api.auth.User(r); u != nil {
		res["user"] = meUser{u.Public(), u.Role}
		res["ready_in"] = api.hub.ReadyIn(u.ID).Seconds()
		if block := u.WriteBlock(api.hub.now()); block != "" {
			res["blocked"] = block
		}
		res["alerts"] = api.store.UnreadAlerts(u.ID)
	}
	writeJSON(w, res)
}

type historyEntry struct {
	Color  string      `json:"color"`
	User   *PublicUser `json:"user"`
	TS     *int64      `json:"ts"`
	Origin bool        `json:"origin,omitempty"`
}

func hexColor(rgb uint32) string { return fmt.Sprintf("#%06X", rgb&0xffffff) }

func (api *API) handlePixel(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	x, errX := strconv.Atoi(r.URL.Query().Get("x"))
	y, errY := strconv.Atoi(r.URL.Query().Get("y"))
	if errX != nil || errY != nil {
		writeError(w, http.StatusBadRequest, "Paramètres x et y attendus.")
		return
	}
	col, owner, ok := api.canvas.At(x, y)
	if !ok {
		writeError(w, http.StatusNotFound, "Ce pixel est hors du canvas.")
		return
	}
	events, err := api.store.PixelHistory(x, y, 3)
	if err != nil {
		log.WithField("endpoint", "API").Error("Pixel history: ", err)
		writeError(w, http.StatusInternalServerError, "Historique indisponible.")
		return
	}
	ids := []uint32{}
	if owner != 0 {
		ids = append(ids, owner)
	}
	for _, e := range events {
		ids = append(ids, e.UserID)
	}
	users, err := api.store.UsersByIDs(ids)
	if err != nil {
		log.WithField("endpoint", "API").Error("Pixel users: ", err)
		writeError(w, http.StatusInternalServerError, "Historique indisponible.")
		return
	}

	history := make([]historyEntry, 0, 4)
	for _, e := range events {
		ts := e.TS
		history = append(history, historyEntry{Color: hexColor(e.Color), User: users[e.UserID].Public(), TS: &ts})
	}
	// Fewer than 3 changes since accounts: the oldest known state is the original artwork.
	if n := len(events); n < 3 && n > 0 && events[n-1].PrevUserID == 0 {
		history = append(history, historyEntry{Color: hexColor(events[n-1].PrevColor), Origin: true})
	}

	res := map[string]any{
		"x": x, "y": y,
		"color":     hexColor(nrgbaToRGB(col)),
		"owner":     users[owner].Public(),
		"origin":    owner == 0,
		"placed_at": nil,
		"history":   history,
	}
	if owner != 0 && len(events) > 0 && events[0].UserID == owner {
		res["placed_at"] = events[0].TS
	}
	cw, _ := api.canvas.Size()
	if oid := api.oeuvres.At(x, y, cw); oid != 0 {
		if o, err := api.store.Oeuvre(oid); err == nil && o != nil {
			var confirms int
			api.store.db.QueryRow(`SELECT COUNT(*) FROM claim_votes WHERE claim_id = ? AND type = 'confirme'`, o.ClaimID).Scan(&confirms)
			var decided int64
			api.store.db.QueryRow(`SELECT COALESCE(decide_le, 0) FROM claims WHERE id = ?`, o.ClaimID).Scan(&decided)
			res["oeuvre"] = map[string]any{
				"id": o.ID, "titre": o.Titre, "origine": o.Origine, "auteurs": o.Auteurs, "apparue_le": o.Apparue,
				"revendiquee_le": decided, "confirmations": confirms, "x": o.X, "y": o.Y, "w": o.W, "h": o.H,
			}
		}
	}
	writeJSON(w, res)
}

func (api *API) handleOwners(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	b := api.canvas.OwnersPNG()
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	w.Write(b)
}

const maxUsersPerRequest = 100

func (api *API) handleUsers(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	var ids []uint32
	for _, s := range strings.Split(r.URL.Query().Get("ids"), ",") {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		id, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Identifiants invalides.")
			return
		}
		ids = append(ids, uint32(id))
	}
	if len(ids) > maxUsersPerRequest {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("%d joueurs au maximum par requête.", maxUsersPerRequest))
		return
	}
	users, err := api.store.UsersByIDs(ids)
	if err != nil {
		log.WithField("endpoint", "API").Error("Users: ", err)
		writeError(w, http.StatusInternalServerError, "Joueurs indisponibles.")
		return
	}
	res := make(map[string]*PublicUser, len(users))
	for id, u := range users {
		res[strconv.FormatUint(uint64(id), 10)] = u.Public()
	}
	writeJSON(w, map[string]any{"users": res})
}
