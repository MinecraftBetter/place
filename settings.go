package place

// Admin settings (a-reglages): cooldowns, canvas mode, announcement banner, server,
// moderation filters. Stored in the settings table, applied live.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Settings struct {
	Cooldown      int    `json:"cooldown"`     // seconds between two pixels
	CooldownNew   int    `json:"cooldown_new"` // seconds, accounts of less than an hour
	Mode          string `json:"mode"`         // normal | readonly | maintenance
	ModeMsg       string `json:"mode_msg"`     // shown in read-only / maintenance
	ModeUntil     string `json:"mode_until"`   // free text, e.g. "18:00"
	BannerOn      bool   `json:"banner_on"`    // announcement banner
	BannerText    string `json:"banner_text"`
	BannerStyle   string `json:"banner_style"` // event | info | danger
	BannerFrom    string `json:"banner_from"`  // YYYY-MM-DD, empty = now
	BannerUntil   string `json:"banner_until"` // YYYY-MM-DD, empty = no end
	MaxConns      int    `json:"max_conns"`
	MaxRate       int    `json:"max_rate"`
	Contact       string `json:"contact"`        // address shown to suspended players
	WordsFilter   bool   `json:"words_filter"`   // forbidden words in pseudos, bios, titles
	Words         string `json:"words"`          // comma-separated
	NoLinks       bool   `json:"no_links"`       // no links in bios
	AvatarCheck   bool   `json:"avatar_check"`   // avatars reviewed before being shown
	SoundsCheck   bool   `json:"sounds_check"`   // museum sounds reviewed (phase 6)
	VoiceLimit    bool   `json:"voice_limit"`    // voice comments limited to 30 s
	GuestbookFlt  bool   `json:"guestbook_flt"`  // guestbook filter
	LaunchAt      string `json:"launch_at"`      // release, Paris time "2006-01-02T15:04" (launch.go)
	LaunchGate    string `json:"launch_gate"`    // off | tout (site closed until then) | accueil (home page only)
	LaunchTrailer bool   `json:"launch_trailer"` // play the trailer at zero
	Guests        bool   `json:"guests"`         // visitors without an account may look around (read-only)
}

var defaultSettings = Settings{
	Mode: "normal", BannerStyle: "event", MaxConns: 64, MaxRate: 30,
	WordsFilter: true, Words: "connard,connasse,salope,pute,encule,enculé,nazi,pd,fdp,ntm",
	NoLinks: true, SoundsCheck: true, VoiceLimit: true, GuestbookFlt: true,
	LaunchGate: "off", LaunchTrailer: true,
}

func (s *Store) LoadSettings() Settings {
	st := defaultSettings
	if raw := s.Setting("reglages", ""); raw != "" {
		json.Unmarshal([]byte(raw), &st)
	}
	return st
}

func (s *Store) SaveSettings(st Settings) error {
	b, _ := json.Marshal(st)
	return s.SetSetting("reglages", string(b))
}

// applySettings pushes settings to the running hub.
func (api *API) applySettings(st Settings) {
	api.hub.SetCooldown(time.Duration(st.Cooldown) * time.Second)
	api.hub.mu.Lock()
	api.hub.cooldownNew = time.Duration(st.CooldownNew) * time.Second
	api.hub.mode = st.Mode
	api.hub.closedUntil = time.Time{}
	if t, ok := st.launchTime(); ok && st.LaunchGate == "tout" {
		api.hub.closedUntil = t // nobody but the admins draws before the release
	}
	if st.MaxConns > 0 {
		api.hub.maxConns = st.MaxConns
	}
	api.hub.maxRate = float64(st.MaxRate)
	api.hub.mu.Unlock()
	api.settings.Store(&st)
}

func (api *API) Settings() Settings {
	if p := api.settings.Load(); p != nil {
		return *p
	}
	return api.store.LoadSettings()
}

// announce returns the banner to show now, or nil.
func (st Settings) announce(now time.Time) map[string]any {
	if !st.BannerOn || strings.TrimSpace(st.BannerText) == "" {
		return nil
	}
	day := now.In(Paris).Format("2006-01-02")
	if st.BannerFrom != "" && day < st.BannerFrom || st.BannerUntil != "" && day > st.BannerUntil {
		return nil
	}
	return map[string]any{"text": st.BannerText, "style": st.BannerStyle, "until": st.BannerUntil}
}

func (st Settings) modeInfo() map[string]any {
	return map[string]any{"mode": st.Mode, "msg": st.ModeMsg, "until": st.ModeUntil}
}

func (api *API) mountSettings() {
	api.mux.HandleFunc("/api/admin/settings", api.admin(api.handleSettings))
	api.mux.HandleFunc("/api/status", api.handlePublicStatus)
	api.mux.HandleFunc("/api/admin/reanalyse", api.admin(func(w http.ResponseWriter, r *http.Request, admin *User) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
			return
		}
		rep, err := api.Reanalyse()
		if err != nil {
			writeError(w, http.StatusConflict, "Les sauvegardes ne sont pas encore indexées.")
			return
		}
		writeJSON(w, rep)
	}))
}

// GET /api/status — mode and announcement, for every page.
func (api *API) handlePublicStatus(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	st := api.Settings()
	writeJSON(w, map[string]any{"mode": st.modeInfo(), "announce": st.announce(api.store.now()), "contact": st.Contact, "launch": api.launchStatus(st), "guests": st.Guests, "now": api.store.now().UnixMilli()})
}

// GET/PUT /api/admin/settings
func (api *API) handleSettings(w http.ResponseWriter, r *http.Request, admin *User) {
	switch r.Method {
	case http.MethodGet:
		st := api.Settings()
		writeJSON(w, map[string]any{"settings": st, "save_interval": api.saveInterval, "backups": api.backupStatus(), "saved_at": lastSave.Load(), "reanalyse": api.LastReanalysis()})
	case http.MethodPut, http.MethodPost:
		before := api.Settings()
		st := before
		if !readJSON(w, r, &st) {
			return
		}
		st.Cooldown = max(0, min(st.Cooldown, 3600))
		st.CooldownNew = max(0, min(st.CooldownNew, 3600))
		if st.Mode != "readonly" && st.Mode != "maintenance" {
			st.Mode = "normal"
		}
		if st.BannerStyle != "info" && st.BannerStyle != "danger" {
			st.BannerStyle = "event"
		}
		if st.LaunchGate != "tout" && st.LaunchGate != "accueil" {
			st.LaunchGate = "off"
		}
		if _, err := time.ParseInLocation(launchLayout, st.LaunchAt, Paris); err != nil {
			st.LaunchAt = ""
		}
		st.MaxConns = max(1, min(st.MaxConns, 10000))
		st.MaxRate = max(0, min(st.MaxRate, 1000))
		if err := api.store.SaveSettings(st); err != nil {
			api.serverError(w, "Settings", err)
			return
		}
		api.applySettings(st)
		if before.Mode != st.Mode || before.ModeMsg != st.ModeMsg || before.ModeUntil != st.ModeUntil {
			api.hub.Broadcast(mustJSON(map[string]any{"type": "mode", "mode": st.Mode, "msg": st.ModeMsg, "until": st.ModeUntil}))
		}
		if fmt.Sprint(before.announce(api.store.now())) != fmt.Sprint(st.announce(api.store.now())) {
			a := st.announce(api.store.now())
			msg := map[string]any{"type": "announce", "text": "", "style": st.BannerStyle}
			if a != nil {
				for k, v := range a {
					msg[k] = v
				}
			}
			api.hub.Broadcast(mustJSON(msg))
		}
		bj, _ := json.Marshal(before)
		aj, _ := json.Marshal(st)
		api.logAction(admin, "settings.update", "reglages", string(bj), string(aj), settingsDiff(before, st))
		writeJSON(w, map[string]any{"settings": st})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
	}
}

// settingsDiff describes what changed, for the journal.
func settingsDiff(a, b Settings) string {
	var out []string
	if a.Cooldown != b.Cooldown {
		out = append(out, fmt.Sprintf("délai %d s → %d s", a.Cooldown, b.Cooldown))
	}
	if a.CooldownNew != b.CooldownNew {
		out = append(out, fmt.Sprintf("délai nouveaux comptes %d s → %d s", a.CooldownNew, b.CooldownNew))
	}
	if a.Mode != b.Mode {
		labels := map[string]string{"normal": "normal", "readonly": "lecture seule", "maintenance": "maintenance"}
		out = append(out, "mode "+labels[a.Mode]+" → "+labels[b.Mode])
	}
	if a.BannerOn != b.BannerOn || a.BannerText != b.BannerText {
		if b.BannerOn {
			out = append(out, "bannière « "+b.BannerText+" »")
		} else {
			out = append(out, "bannière retirée")
		}
	}
	if a.LaunchAt != b.LaunchAt || a.LaunchGate != b.LaunchGate {
		if t, ok := b.launchTime(); ok && b.LaunchGate != "off" {
			out = append(out, "lancement "+frenchLaunchDate(t))
		} else {
			out = append(out, "compte à rebours coupé")
		}
	}
	if a.Guests != b.Guests {
		if b.Guests {
			out = append(out, "visiteurs sans compte autorisés")
		} else {
			out = append(out, "connexion obligatoire")
		}
	}
	if a.MaxConns != b.MaxConns {
		out = append(out, "connexions max "+strconv.Itoa(b.MaxConns))
	}
	if len(out) == 0 {
		return "réglages modifiés"
	}
	return strings.Join(out, " · ")
}

// logAction writes to the admin journal and returns its id.
func (api *API) logAction(admin *User, typ, cible, before, after, motif string) int64 {
	res, err := api.store.db.Exec(`INSERT INTO admin_actions (admin_id, type, cible, avant_json, apres_json, motif, ts) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		admin.ID, typ, cible, before, after, motif, api.store.now().UnixMilli())
	if err != nil {
		return 0
	}
	id, _ := res.LastInsertId()
	return id
}
