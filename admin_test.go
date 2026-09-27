package place

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func (e *testEnv) put(t *testing.T, path string, ck *http.Cookie, body string) int {
	t.Helper()
	req, _ := http.NewRequest("PUT", e.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(ck)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

func TestSettingsModesAndCooldowns(t *testing.T) {
	e := claimEnv(t)
	admin, brin := e.login(t, "Tiago"), e.login(t, "Brindille")
	if status := e.put(t, "/api/admin/settings", brin, `{}`); status != 403 {
		t.Fatalf("player on settings: %d", status)
	}
	s := e.getJSON(t, "/api/admin/settings", admin)["settings"].(map[string]any)
	if s["cooldown"] != 5.0 || s["mode"] != "normal" {
		t.Fatalf("starting settings follow the options: %v", s)
	}

	if status := e.put(t, "/api/admin/settings", admin, `{"cooldown":0,"mode":"readonly","mode_msg":"Vernissage à 18 h","banner_on":true,"banner_text":"Soirée pixel samedi"}`); status != 200 {
		t.Fatalf("save settings: %d", status)
	}
	player := e.dial(t, brin, "")
	sendPixel(t, player, 1, 1)
	if m := next(t, player, isType("error")); m["err"] != "readonly" {
		t.Fatalf("read-only: %v", m)
	}
	boss := e.dial(t, admin, "")
	sendPixel(t, boss, 2, 2)
	if m := next(t, boss, func(m map[string]any) bool { return isPixel(m) || m["type"] == "error" }); m["type"] == "error" {
		t.Fatalf("admins can still draw: %v", m)
	}
	st := e.getJSON(t, "/api/status", nil)
	if st["mode"].(map[string]any)["mode"] != "readonly" || st["announce"].(map[string]any)["text"] != "Soirée pixel samedi" {
		t.Fatalf("public status %v", st)
	}

	// new accounts wait longer during their first hour
	e.put(t, "/api/admin/settings", admin, `{"cooldown":0,"cooldown_new":30,"mode":"normal","banner_on":false}`)
	me := e.getJSON(t, "/api/me", brin)
	if me["cooldown"] != 30.0 {
		t.Fatalf("new account cooldown %v", me["cooldown"])
	}
	sendPixel(t, player, 3, 3)
	next(t, player, isPixel)
	sendPixel(t, player, 4, 4)
	if m := next(t, player, isType("error")); m["err"] != "cooldown" {
		t.Fatalf("new account second pixel: %v", m)
	}

	// the journal can undo settings
	j := e.getJSON(t, "/api/admin/journal?filtre=settings", admin)["actions"].([]any)
	if len(j) != 2 {
		t.Fatalf("journal %v", j)
	}
	id := itoa(int(j[0].(map[string]any)["id"].(float64)))
	if status, m := e.post(t, "/api/admin/journal/"+id+"/undo", admin, nil); status != 200 {
		t.Fatalf("undo settings: %d %v", status, m)
	}
	if s := e.getJSON(t, "/api/admin/settings", admin)["settings"].(map[string]any); s["mode"] != "readonly" {
		t.Fatalf("undo did not restore the previous settings: %v", s["mode"])
	}
}

func TestAdminUsersActions(t *testing.T) {
	e := claimEnv(t)
	admin, bot := e.login(t, "Tiago"), e.login(t, "Bot404")
	users := e.getJSON(t, "/api/admin/users?q=bot", admin)
	list := users["users"].([]any)
	if len(list) != 1 || users["total"] != 2.0 {
		t.Fatalf("search %v", users)
	}
	uid := itoa(int(list[0].(map[string]any)["id"].(float64)))
	conn := e.dial(t, bot, "")
	if status, m := e.post(t, "/api/admin/users/"+uid+"/suspend", admin, map[string]any{"duree": "24h", "motif": "Script automatique"}); status != 200 {
		t.Fatalf("suspend %d %v", status, m)
	}
	next(t, conn, func(m map[string]any) bool { return m["err"] == "suspended" })
	sendPixel(t, conn, 1, 1)
	if m := next(t, conn, isType("error")); m["err"] != "suspended" {
		t.Fatalf("suspended player drew: %v", m)
	}
	me := e.getJSON(t, "/api/me", bot)
	if me["blocked"] != "suspended" || me["sanction"].(map[string]any)["motif"] != "Script automatique" {
		t.Fatalf("me when suspended %v", me)
	}
	d := e.getJSON(t, "/api/admin/users/"+uid, admin)
	if d["statut"] != "suspendu" || d["motif"] != "Script automatique" {
		t.Fatalf("detail %v", d)
	}
	// undo from the journal
	j := e.getJSON(t, "/api/admin/journal?filtre=users", admin)["actions"].([]any)
	id := itoa(int(j[0].(map[string]any)["id"].(float64)))
	e.post(t, "/api/admin/journal/"+id+"/undo", admin, nil)
	if me := e.getJSON(t, "/api/me", bot); me["blocked"] != nil {
		t.Fatalf("still blocked after undo %v", me["blocked"])
	}
	if status, _ := e.post(t, "/api/admin/users/"+uid+"/ban", admin, map[string]any{}); status != 400 {
		t.Fatalf("ban without reason: %d", status)
	}
	adminID := itoa(int(e.getJSON(t, "/api/me", admin)["user"].(map[string]any)["id"].(float64)))
	if status, _ := e.post(t, "/api/admin/users/"+adminID+"/ban", admin, map[string]any{"motif": "x"}); status != 400 {
		t.Fatalf("self ban: %d", status)
	}
	if status, _ := e.get(t, "/api/admin/journal.csv", admin); status != 200 {
		t.Fatal("csv export")
	}
}

func TestZoneEraseRestoreUndo(t *testing.T) {
	e := claimEnv(t)
	e.hub.SetCooldown(0)
	admin, brin := e.login(t, "Tiago"), e.login(t, "Brindille")
	conn := e.dial(t, brin, "")
	sendPixel(t, conn, 1, 1)
	next(t, conn, isPixel)
	mask := map[string]any{"type": "rect", "rect": []int{0, 0, 4, 4}}

	status, p := e.post(t, "/api/admin/zones/preview", admin, map[string]any{"masque": mask, "action": "erase"})
	if status != 200 || p["pixels"] != 16.0 || p["modifies"].(float64) < 1 || len(p["joueurs"].([]any)) != 1 {
		t.Fatalf("preview %d %v", status, p)
	}
	watch := e.dial(t, nil, "")
	if status, m := e.post(t, "/api/admin/zones/apply", admin, map[string]any{"masque": mask, "action": "erase", "motif": "test"}); status != 200 {
		t.Fatalf("erase %d %v", status, m)
	}
	if m := next(t, watch, isType("zone")); m["w"] != 4.0 {
		t.Fatalf("zone message %v", m)
	}
	col, owner, _ := e.canvas.At(1, 1)
	if nrgbaToRGB(col) != white || owner != 0 {
		t.Fatalf("not erased: %v %d", col, owner)
	}
	if u, _ := e.store.UserBySlug("brindille"); u.PixelsVisible != 0 {
		t.Fatalf("visible count after erase %d", u.PixelsVisible)
	}
	j := e.getJSON(t, "/api/admin/journal?filtre=zones", admin)["actions"].([]any)
	e.post(t, "/api/admin/journal/"+itoa(int(j[0].(map[string]any)["id"].(float64)))+"/undo", admin, nil)
	if _, owner, _ := e.canvas.At(1, 1); owner == 0 {
		t.Fatal("undo did not bring the pixel back")
	}

	// restore from the backup where the 4×4 artwork was blue
	_, bk := e.post(t, "/api/admin/zones/backups", admin, map[string]any{"masque": mask})
	backups := bk["backups"].([]any)
	if len(backups) == 0 {
		t.Fatal("no backup to restore")
	}
	frame := int(backups[0].(map[string]any)["frame"].(float64))
	e.post(t, "/api/admin/zones/apply", admin, map[string]any{"masque": mask, "action": "restore", "frame": frame})
	if col, _, _ := e.canvas.At(1, 1); nrgbaToRGB(col) != 0x5eb3ff {
		t.Fatalf("restored colour %v", col)
	}

	// give a zone directly
	if status, m := e.post(t, "/api/admin/zones/apply", admin, map[string]any{"masque": mask, "action": "assign", "user": "brindille", "titre": "Carré bleu", "badges": []string{"pionnier"}}); status != 200 {
		t.Fatalf("assign %d %v", status, m)
	}
	if px := e.getJSON(t, "/api/pixel?x=1&y=1", nil); px["oeuvre"].(map[string]any)["titre"] != "Carré bleu" {
		t.Fatalf("assigned artwork %v", px["oeuvre"])
	}
	j = e.getJSON(t, "/api/admin/journal?filtre=zones", admin)["actions"].([]any)
	e.post(t, "/api/admin/journal/"+itoa(int(j[0].(map[string]any)["id"].(float64)))+"/undo", admin, nil)
	if px := e.getJSON(t, "/api/pixel?x=1&y=1", nil); px["oeuvre"] != nil {
		t.Fatal("artwork still there after undoing the attribution")
	}
}

func TestModerationFilterAndHide(t *testing.T) {
	e := claimEnv(t)
	admin, brin, lucie := e.login(t, "Tiago"), e.login(t, "Brindille"), e.login(t, "Lucie")
	req, _ := http.NewRequest("PATCH", e.srv.URL+"/api/me/profile", strings.NewReader(`{"bio":"Venez sur www.example.com"}`))
	req.AddCookie(brin)
	res, _ := http.DefaultClient.Do(req)
	res.Body.Close()
	reports := e.getJSON(t, "/api/admin/reports", admin)["reports"].([]any)
	if len(reports) != 1 || reports[0].(map[string]any)["source"] != "filtre" || reports[0].(map[string]any)["type"] != "bio" {
		t.Fatalf("automatic filter %v", reports)
	}
	// a player report on a pseudo
	brinID := int(e.getJSON(t, "/api/users/brindille", nil)["id"].(float64))
	if status, _ := e.post(t, "/api/reports", lucie, map[string]any{"type": "pseudo", "cible": "user:" + itoa(brinID), "raison": "bof"}); status != 200 {
		t.Fatal("player report")
	}
	if n := len(e.getJSON(t, "/api/admin/status", admin)["counts"].(map[string]any)); n == 0 {
		t.Fatal("status counts")
	}
	if c := e.getJSON(t, "/api/admin/status", admin)["counts"].(map[string]any)["reports"]; c != 2.0 {
		t.Fatalf("pending reports %v", c)
	}
	bio := itoa(int(reports[0].(map[string]any)["id"].(float64)))
	if status, _ := e.post(t, "/api/admin/reports/"+bio+"/masquer", admin, nil); status != 200 {
		t.Fatal("hide")
	}
	if p := e.getJSON(t, "/api/users/brindille", nil); p["bio"] != "" {
		t.Fatalf("bio still shown %v", p["bio"])
	}
	if a := e.getJSON(t, "/api/alerts", brin)["alerts"].([]any); len(a) == 0 || a[0].(map[string]any)["kind"] != "moderation" {
		t.Fatalf("player not told %v", a)
	}
	j := e.getJSON(t, "/api/admin/journal?filtre=moderation", admin)["actions"].([]any)
	e.post(t, "/api/admin/journal/"+itoa(int(j[0].(map[string]any)["id"].(float64)))+"/undo", admin, nil)
	if p := e.getJSON(t, "/api/users/brindille", nil); p["bio"] != "Venez sur www.example.com" {
		t.Fatalf("undo did not bring the bio back: %v", p["bio"])
	}
}

func TestDashboardStats(t *testing.T) {
	e := claimEnv(t)
	e.hub.SetCooldown(0)
	admin := e.login(t, "Tiago")
	conn := e.dial(t, e.login(t, "Brindille"), "")
	sendPixel(t, conn, 1, 1)
	next(t, conn, isPixel)
	s := e.getJSON(t, "/api/admin/stats?periode=jour", admin)
	kpi := s["kpi"].(map[string]any)
	if kpi["pixels"] != 1.0 || kpi["actifs"] != 1.0 || kpi["nouveaux"] != 2.0 {
		t.Fatalf("kpi %v", kpi)
	}
	hours := s["heures"].([]any)
	if len(hours) != 24 || hours[time.Now().In(Paris).Hour()] != 1.0 {
		t.Fatalf("hours %v", hours)
	}
	if len(s["historique"].([]any)) != 1 || len(s["zones"].([]any)) == 0 || s["couleurs"].(map[string]any)["distinctes"].(float64) < 2 {
		t.Fatalf("history/zones/colours %v %v %v", s["historique"], s["zones"], s["couleurs"])
	}
	if status, b := e.get(t, "/api/admin/heatmap.png", admin); status != 200 || len(b) < 50 {
		t.Fatalf("heatmap %d", status)
	}
}
