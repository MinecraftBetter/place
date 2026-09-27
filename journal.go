package place

// Admin journal (a-journal): every admin action, filters, undo within 30 days, CSV export.

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const undoWindow = 30 * 24 * time.Hour

type journalRow struct {
	ID      int64       `json:"id"`
	Admin   *PublicUser `json:"admin"`
	Type    string      `json:"type"`
	Label   string      `json:"action"`
	Class   string      `json:"classe"`
	Cible   string      `json:"cible"`
	Detail  string      `json:"detail"`
	TS      int64       `json:"ts"`
	Undone  bool        `json:"annulee"`
	CanUndo bool        `json:"annulable"`
}

var actionLabels = map[string][2]string{
	"claim.valider": {"Revendication validée", "st-valide"}, "claim.fusionner": {"Fusion en co-auteurs", "st-valide"},
	"claim.attribuer": {"Œuvre attribuée", "st-info"}, "claim.refuser": {"Revendication refusée", "st-refuse"},
	"user.suspend": {"Suspension", "st-conflit"}, "user.ban": {"Bannissement", "st-refuse"}, "user.unban": {"Réactivation", "st-valide"},
	"user.reset": {"Profil réinitialisé", "st-neutre"}, "user.note": {"Note", "st-neutre"}, "user.role": {"Rôle", "st-info"},
	"zone.restore": {"Zone restaurée", "st-info"}, "zone.erase": {"Zone effacée", "st-refuse"}, "zone.assign": {"Zone attribuée", "st-valide"},
	"moderation.masquer": {"Contenu masqué", "st-conflit"}, "moderation.garder": {"Contenu gardé", "st-neutre"}, "moderation.message": {"Message au joueur", "st-neutre"},
	"settings.update": {"Réglages", "st-neutre"},
}

var journalFilters = map[string]string{"claims": "claim.", "users": "user.", "zones": "zone.", "moderation": "moderation.", "settings": "settings."}

func (api *API) mountJournal() {
	api.mux.HandleFunc("/api/admin/journal", api.admin(api.handleJournal))
	api.mux.HandleFunc("/api/admin/journal.csv", api.admin(api.handleJournalCSV))
	api.mux.HandleFunc("/api/admin/journal/", api.admin(api.handleJournalUndo))
}

func (api *API) journal(filter string, limit int) ([]journalRow, error) {
	q := `SELECT id, admin_id, type, cible, motif, ts, annulee FROM admin_actions`
	var args []any
	if p, ok := journalFilters[filter]; ok {
		q += ` WHERE type LIKE ?`
		args = append(args, p+"%")
	}
	rows, err := api.store.db.Query(q+` ORDER BY id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	var out []journalRow
	var ids []uint32
	var admins []uint32
	for rows.Next() {
		var jr journalRow
		var aid uint32
		if err := rows.Scan(&jr.ID, &aid, &jr.Type, &jr.Cible, &jr.Detail, &jr.TS, &jr.Undone); err != nil {
			rows.Close()
			return nil, err
		}
		l := actionLabels[jr.Type]
		jr.Label, jr.Class = l[0], l[1]
		if jr.Label == "" {
			jr.Label, jr.Class = jr.Type, "st-neutre"
		}
		undoable := jr.Type != "user.note" && jr.Type != "moderation.message"
		jr.CanUndo = !jr.Undone && undoable && time.Since(time.UnixMilli(jr.TS)) < undoWindow
		out = append(out, jr)
		admins = append(admins, aid)
		ids = append(ids, aid)
	}
	rows.Close()
	users, _ := api.store.UsersByIDs(ids)
	for i := range out {
		out[i].Admin = users[admins[i]].Public()
	}
	if out == nil {
		out = []journalRow{}
	}
	return out, nil
}

func (api *API) handleJournal(w http.ResponseWriter, r *http.Request, admin *User) {
	if !getOnly(w, r) {
		return
	}
	rows, err := api.journal(r.URL.Query().Get("filtre"), 300)
	if err != nil {
		api.serverError(w, "Journal", err)
		return
	}
	writeJSON(w, map[string]any{"actions": rows})
}

func (api *API) handleJournalCSV(w http.ResponseWriter, r *http.Request, admin *User) {
	rows, err := api.journal(r.URL.Query().Get("filtre"), 100000)
	if err != nil {
		api.serverError(w, "Journal", err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="journal-betterplace.csv"`)
	w.Write([]byte("\xEF\xBB\xBF")) // BOM: accents in spreadsheets
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	cw.Write([]string{"date", "admin", "action", "cible", "détail", "annulée"})
	for _, jr := range rows {
		admin := ""
		if jr.Admin != nil {
			admin = jr.Admin.Pseudo
		}
		undone := "non"
		if jr.Undone {
			undone = "oui"
		}
		cw.Write([]string{time.UnixMilli(jr.TS).In(Paris).Format("2006-01-02 15:04"), admin, jr.Label, jr.Cible, jr.Detail, undone})
	}
	cw.Flush()
}

// POST /api/admin/journal/:id/undo
func (api *API) handleJournalUndo(w http.ResponseWriter, r *http.Request, admin *User) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
		return
	}
	idStr, sub, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/admin/journal/"), "/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || sub != "undo" {
		writeError(w, http.StatusNotFound, "Introuvable.")
		return
	}
	if err := api.Undo(id); writeFieldError(w, err) {
		return
	} else if err != nil {
		api.serverError(w, "Undo", err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// Undo reverts a journal entry.
func (api *API) Undo(id int64) error {
	var typ, cible, before string
	var ts int64
	var undone bool
	if err := api.store.db.QueryRow(`SELECT type, cible, avant_json, ts, annulee FROM admin_actions WHERE id = ?`, id).Scan(&typ, &cible, &before, &ts, &undone); err != nil {
		return &FieldError{"", "Action introuvable."}
	}
	if undone {
		return &FieldError{"", "Cette action est déjà annulée."}
	}
	if time.Since(time.UnixMilli(ts)) > undoWindow {
		return &FieldError{"", "Trop ancienne : on ne peut annuler que pendant 30 jours."}
	}
	markUndone := func() error {
		_, err := api.store.db.Exec(`UPDATE admin_actions SET annulee = 1 WHERE id = ?`, id)
		return err
	}
	switch {
	case strings.HasPrefix(typ, "claim."):
		if err := api.store.UndoDecision(id); err != nil {
			return err
		}
		api.RefreshOeuvres()
		return nil
	case strings.HasPrefix(typ, "user."):
		var sn userSnapshot
		if err := json.Unmarshal([]byte(before), &sn); err != nil {
			return err
		}
		uid, _ := strconv.ParseUint(strings.TrimPrefix(cible, "user:"), 10, 32)
		if err := api.store.restoreUser(uint32(uid), sn); err != nil {
			return err
		}
		if u, _ := api.store.UserByID(uint32(uid)); u != nil {
			api.hub.UpdateUser(u)
		}
		return markUndone()
	case strings.HasPrefix(typ, "zone."):
		if err := api.undoZone(before); err != nil {
			return err
		}
		return markUndone()
	case strings.HasPrefix(typ, "moderation."):
		if err := api.undoModeration(cible, before); err != nil {
			return err
		}
		return markUndone()
	case typ == "settings.update":
		var st Settings
		if err := json.Unmarshal([]byte(before), &st); err != nil {
			return err
		}
		if err := api.store.SaveSettings(st); err != nil {
			return err
		}
		api.applySettings(st)
		api.hub.Broadcast(mustJSON(map[string]any{"type": "mode", "mode": st.Mode, "msg": st.ModeMsg, "until": st.ModeUntil}))
		return markUndone()
	}
	return &FieldError{"", fmt.Sprintf("Action « %s » non annulable.", typ)}
}
