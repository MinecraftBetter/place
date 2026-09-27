package place

// Admin — players (a-utilisateurs): search, details, suspend, ban, reset the profile,
// private note, role. Every action goes to the journal and can be undone.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (api *API) mountAdminUsers() {
	api.mux.HandleFunc("/api/admin/users", api.admin(api.handleAdminUsers))
	api.mux.HandleFunc("/api/admin/users/", api.admin(api.handleAdminUser))
}

type adminUserRow struct {
	*PublicUser
	Placed    int64  `json:"pixels_poses"`
	Visible   int64  `json:"pixels_visibles"`
	CreatedAt int64  `json:"cree_le"`
	LastPixel int64  `json:"dernier_pixel"`
	Status    string `json:"statut"`
	Until     int64  `json:"suspendu_jusqua,omitempty"`
	Role      string `json:"role"`
	Provider  string `json:"fournisseur"`
}

// effectiveStatus: a suspension that ended counts as active.
func effectiveStatus(u *User, now time.Time) string {
	if u.Status == "suspendu" && u.SuspendedUntil != 0 && now.UnixMilli() >= u.SuspendedUntil {
		return "actif"
	}
	return u.Status
}

// GET /api/admin/users?q=&filtre=tous|actifs|suspendus|bannis
func (api *API) handleAdminUsers(w http.ResponseWriter, r *http.Request, admin *User) {
	if !getOnly(w, r) {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	filter := r.URL.Query().Get("filtre")
	sqlq := `SELECT ` + userCols + `, COALESCE((SELECT MAX(ts) FROM pixel_events e WHERE e.user_id = u.id), 0) FROM users u WHERE 1 = 1`
	var args []any
	if q != "" {
		sqlq += ` AND (u.pseudo LIKE ? OR u.slug LIKE ? OR u.id_externe LIKE ?)`
		like := "%" + q + "%"
		args = append(args, like, "%"+Slugify(q)+"%", like)
	}
	now := api.store.now()
	switch filter {
	case "suspendus":
		sqlq += ` AND u.statut = 'suspendu' AND (u.suspendu_jusqua IS NULL OR u.suspendu_jusqua > ?)`
		args = append(args, now.UnixMilli())
	case "bannis":
		sqlq += ` AND u.statut = 'banni'`
	case "actifs":
		sqlq += ` AND u.id IN (SELECT user_id FROM pixel_events WHERE ts >= ?)`
		args = append(args, DayStart(now).UnixMilli())
	}
	rows, err := api.store.db.Query(sqlq+` ORDER BY u.pixels_poses DESC, u.id LIMIT 200`, args...)
	if err != nil {
		api.serverError(w, "Admin users", err)
		return
	}
	out := []adminUserRow{}
	for rows.Next() {
		var last int64
		u := &User{}
		var pinned string
		err := rows.Scan(&u.ID, &u.Provider, &u.ExternalID, &u.Pseudo, &u.Slug, &u.AvatarURL, &u.Accent, &u.Role,
			&u.Status, &u.SuspendedUntil, &u.PixelsPlaced, &u.PixelsVisible, &u.CreatedAt, &u.Bio, &u.Banner, &u.FavColor,
			&u.PixelsRestored, &u.PixelsNight, &u.PixelsPioneer, &pinned, &u.MapPublic, &u.RetouchAlerts, &u.BannerURL,
			&u.SanctionReason, &u.AdminNote, &last)
		if err != nil {
			rows.Close()
			api.serverError(w, "Admin users", err)
			return
		}
		out = append(out, adminUserRow{u.Public(), u.PixelsPlaced, u.PixelsVisible, u.CreatedAt, last, effectiveStatus(u, now), u.SuspendedUntil, u.Role, u.Provider})
	}
	rows.Close()
	var total, today int
	api.store.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&total)
	api.store.db.QueryRow(`SELECT COUNT(DISTINCT user_id) FROM pixel_events WHERE ts >= ?`, DayStart(now).UnixMilli()).Scan(&today)
	writeJSON(w, map[string]any{"users": out, "total": total, "actifs_jour": today})
}

// userSnapshot is what an action on a player can change (kept to undo it).
type userSnapshot struct {
	Status, Reason, Avatar, Bio, Banner, BannerURL, Role, Note string
	Until                                                      int64
}

func snapshotOf(u *User) userSnapshot {
	return userSnapshot{u.Status, u.SanctionReason, u.AvatarURL, u.Bio, u.Banner, u.BannerURL, u.Role, u.AdminNote, u.SuspendedUntil}
}

func (s *Store) restoreUser(uid uint32, sn userSnapshot) error {
	var until any
	if sn.Until != 0 {
		until = sn.Until
	}
	_, err := s.db.Exec(`UPDATE users SET statut = ?, motif_sanction = ?, avatar_url = ?, bio = ?, banner = ?, banniere_url = ?, role = ?, note_admin = ?, suspendu_jusqua = ? WHERE id = ?`,
		sn.Status, sn.Reason, sn.Avatar, sn.Bio, sn.Banner, sn.BannerURL, sn.Role, sn.Note, until, uid)
	return err
}

// /api/admin/users/:id and /api/admin/users/:id/{suspend,ban,unban,reset,note,role,pixels}
func (api *API) handleAdminUser(w http.ResponseWriter, r *http.Request, admin *User) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
	idStr, sub, _ := strings.Cut(rest, "/")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		writeError(w, http.StatusNotFound, "Joueur introuvable.")
		return
	}
	u, err := api.store.UserByID(uint32(id))
	if err != nil || u == nil {
		writeError(w, http.StatusNotFound, "Joueur introuvable.")
		return
	}
	now := api.store.now()
	if sub == "" {
		if !getOnly(w, r) {
			return
		}
		var oeuvres, maxMinute int
		var last int64
		api.store.db.QueryRow(`SELECT COUNT(*) FROM oeuvre_auteurs WHERE user_id = ?`, u.ID).Scan(&oeuvres)
		api.store.db.QueryRow(`SELECT COALESCE(MAX(ts), 0) FROM pixel_events WHERE user_id = ?`, u.ID).Scan(&last)
		api.store.db.QueryRow(`SELECT COALESCE(MAX(n), 0) FROM (SELECT COUNT(*) AS n FROM pixel_events WHERE user_id = ? GROUP BY ts / 60000)`, u.ID).Scan(&maxMinute)
		var recent int
		api.store.db.QueryRow(`SELECT COUNT(*) FROM pixel_events WHERE user_id = ? AND ts >= ?`, u.ID, now.Add(-10*time.Minute).UnixMilli()).Scan(&recent)
		writeJSON(w, map[string]any{
			"user": u.Public(), "statut": effectiveStatus(u, now), "suspendu_jusqua": u.SuspendedUntil, "motif": u.SanctionReason,
			"role": u.Role, "fournisseur": u.Provider, "id_externe": u.ExternalID, "cree_le": u.CreatedAt, "bio": u.Bio,
			"pixels_poses": u.PixelsPlaced, "pixels_visibles": u.PixelsVisible, "pixels_pionniers": u.PixelsPioneer,
			"oeuvres": oeuvres, "dernier_pixel": last, "rythme_max": maxMinute, "recents_10min": recent, "note": u.AdminNote,
		})
		return
	}
	if sub == "pixels" {
		// the pixels this player owns on the canvas now (zone tool: « pixels d'un joueur »)
		if !getOnly(w, r) {
			return
		}
		var pos []int32
		api.canvas.mu.RLock()
		for i, o := range api.canvas.owners {
			if o == u.ID {
				pos = append(pos, int32(i))
			}
		}
		api.canvas.mu.RUnlock()
		cw, _ := api.canvas.Size()
		var first, last int64
		api.store.db.QueryRow(`SELECT COALESCE(MIN(ts), 0), COALESCE(MAX(ts), 0) FROM pixel_events WHERE user_id = ?`, u.ID).Scan(&first, &last)
		res := map[string]any{"pixels": len(pos), "premier": first, "dernier": last}
		if len(pos) > 0 {
			res["masque"] = MaskFromPositions(pos, cw)
		}
		writeJSON(w, res)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
		return
	}
	var req struct {
		Duration string `json:"duree"` // 1h | 24h | 7j
		Motif    string `json:"motif"`
		Note     string `json:"note"`
		Role     string `json:"role"`
	}
	if r.ContentLength != 0 && !readJSON(w, r, &req) {
		return
	}
	if u.ID == admin.ID && sub != "note" {
		writeError(w, http.StatusBadRequest, "Tu ne peux pas faire ça sur ton propre compte.")
		return
	}
	before := snapshotOf(u)
	var detail string
	motif := strings.TrimSpace(req.Motif)
	switch sub {
	case "suspend":
		d := map[string]time.Duration{"1h": time.Hour, "24h": 24 * time.Hour, "7j": 7 * 24 * time.Hour}[req.Duration]
		if d == 0 {
			writeFieldError(w, &FieldError{"duree", "Durée inconnue."})
			return
		}
		until := now.Add(d).UnixMilli()
		api.store.db.Exec(`UPDATE users SET statut = 'suspendu', suspendu_jusqua = ?, motif_sanction = ? WHERE id = ?`, until, motif, u.ID)
		detail = fmt.Sprintf("%s suspendu %s", u.Pseudo, map[string]string{"1h": "1 h", "24h": "24 h", "7j": "7 jours"}[req.Duration])
	case "ban":
		if motif == "" {
			writeFieldError(w, &FieldError{"motif", "Indique le motif du bannissement."})
			return
		}
		api.store.db.Exec(`UPDATE users SET statut = 'banni', suspendu_jusqua = NULL, motif_sanction = ? WHERE id = ?`, motif, u.ID)
		detail = u.Pseudo + " banni"
	case "unban":
		api.store.db.Exec(`UPDATE users SET statut = 'actif', suspendu_jusqua = NULL, motif_sanction = '' WHERE id = ?`, u.ID)
		detail = u.Pseudo + " réactivé"
	case "reset":
		api.store.db.Exec(`UPDATE users SET avatar_url = '', bio = '', banner = '', banniere_url = '' WHERE id = ?`, u.ID)
		detail = "profil de " + u.Pseudo + " réinitialisé (avatar, bio, bannière)"
	case "note":
		api.store.db.Exec(`UPDATE users SET note_admin = ? WHERE id = ?`, strings.TrimSpace(req.Note), u.ID)
		detail = "note sur " + u.Pseudo
	case "role":
		if req.Role != "admin" && req.Role != "joueur" {
			writeFieldError(w, &FieldError{"role", "Rôle inconnu."})
			return
		}
		api.store.db.Exec(`UPDATE users SET role = ? WHERE id = ?`, req.Role, u.ID)
		detail = fmt.Sprintf("%s devient %s", u.Pseudo, map[string]string{"admin": "admin", "joueur": "joueur"}[req.Role])
	default:
		writeError(w, http.StatusNotFound, "Introuvable.")
		return
	}
	updated, _ := api.store.UserByID(u.ID)
	api.hub.UpdateUser(updated)
	bj, _ := json.Marshal(before)
	aj, _ := json.Marshal(snapshotOf(updated))
	if motif != "" {
		detail += " · " + motif
	}
	actionID := api.logAction(admin, "user."+sub, fmt.Sprintf("user:%d", u.ID), string(bj), string(aj), detail)
	if sub == "suspend" || sub == "ban" {
		api.hub.SendToUser(u.ID, mustJSON(map[string]any{"type": "error", "err": map[string]string{"suspend": "suspended", "ban": "banned"}[sub]}))
	}
	writeJSON(w, map[string]any{"ok": true, "action_id": actionID})
}
