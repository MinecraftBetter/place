package place

// Admin area (HANDOFF §4.3): status, and later dashboard, users, zone tool,
// moderation, journal and settings.

import (
	"net/http"
	"sync/atomic"
)

// lastSave is when place.png was last written (ms), shown in the admin side bar.
var lastSave atomic.Int64

// MarkSaved records a successful save of the canvas.
func MarkSaved(ms int64) { lastSave.Store(ms) }

func (api *API) mountAdmin() {
	api.mux.HandleFunc("/api/admin/status", api.admin(api.handleAdminStatus))
}

func (api *API) handleAdminStatus(w http.ResponseWriter, r *http.Request, u *User) {
	if !getOnly(w, r) {
		return
	}
	online, slots := api.hub.Online()
	counts, _ := api.store.ClaimCounts()
	res := map[string]any{
		"mode": api.store.Setting("mode", "normal"), "online": online, "slots": slots, "saved_at": lastSave.Load(),
		"counts":  map[string]int{"claims": counts["attente"], "reports": api.store.PendingReports()},
		"backups": api.backupStatus(),
	}
	writeJSON(w, res)
}

// PendingReports counts the moderation items waiting for a decision (phase 5).
func (s *Store) PendingReports() int {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'reports'`).Scan(&n)
	if n == 0 {
		return 0
	}
	s.db.QueryRow(`SELECT COUNT(*) FROM reports WHERE statut = 'attente'`).Scan(&n)
	return n
}
