package place

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFrenchLaunchDate(t *testing.T) {
	y := time.Now().In(Paris).Year()
	if s := frenchLaunchDate(time.Date(y, 10, 24, 20, 0, 0, 0, Paris)); !strings.HasSuffix(s, " 24 octobre à 20 h") {
		t.Fatalf("got %q", s)
	}
	if s := frenchLaunchDate(time.Date(y+1, 1, 3, 9, 30, 0, 0, Paris)); !strings.HasSuffix(s, " 3 janvier "+itoa(y+1)+" à 9 h 30") {
		t.Fatalf("got %q", s)
	}
}

func TestLaunchCountdownGate(t *testing.T) {
	e := newEnv(t, 8)
	e.auth.SetAdmins([]string{"tiago"})
	e.store.now = e.clock.Now // one clock for the pages and the hub
	now := e.clock.Now()
	admin, lucie := e.login(t, "Tiago"), e.login(t, "Lucie")
	at := now.Add(time.Hour).In(Paris).Format(launchLayout)
	if status := e.put(t, "/api/admin/settings", admin, `{"launch_at":"`+at+`","launch_gate":"tout","launch_trailer":true,"mode":"normal","max_conns":8}`); status != 200 {
		t.Fatalf("settings %d", status)
	}
	st := e.getJSON(t, "/api/status", nil)["launch"].(map[string]any)
	if st["active"] != true || st["at"] == nil || !strings.Contains(st["label"].(string), " à ") {
		t.Fatalf("status %v", st)
	}

	gate := e.api.LaunchGate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	get := func(path string, ck *http.Cookie) (int, string) {
		req := httptest.NewRequest("GET", path, nil)
		if ck != nil {
			req.AddCookie(ck)
		}
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, req)
		return rec.Code, rec.Header().Get("Location")
	}
	for _, p := range []string{"/", "/home", "/ace", "/musee", "/u/tiago"} {
		if code, loc := get(p, lucie); code != http.StatusFound || loc != "/lancement" {
			t.Fatalf("%s during the countdown: %d %s", p, code, loc)
		}
	}
	for _, p := range []string{"/lancement", "/bande-annonce", "/login", "/api/status", "/js/app.js", "/place.png", "/img/farwest-x8.png"} {
		if code, _ := get(p, nil); code != 200 {
			t.Fatalf("%s should stay open: %d", p, code)
		}
	}
	if code, _ := get("/ace", admin); code != 200 {
		t.Fatal("admins get in before the release")
	}
	// players cannot draw yet, admins can
	c := e.dial(t, lucie, "")
	sendPixel(t, c, 1, 1)
	if m := next(t, c, isType("error")); m["err"] != "readonly" {
		t.Fatalf("pixel before the release: %v", m)
	}
	// the link preview shows the date
	if page := e.api.RewritePage(httptest.NewRequest("GET", "https://place.justbetter.fr/lancement", nil), `<meta content="{{DESCRIPTION}}"><meta content="{{ORIGIN}}/img/og.png">`); !strings.Contains(page, "ouvre ") || !strings.Contains(page, "https://place.justbetter.fr/img") || strings.Contains(page, "{{") {
		t.Fatalf("preview %s", page)
	}

	// « accueil »: only the home page waits
	e.put(t, "/api/admin/settings", admin, `{"launch_gate":"accueil"}`)
	if code, _ := get("/ace", lucie); code != 200 {
		t.Fatal("canvas closed with the home-only countdown")
	}
	if code, _ := get("/", lucie); code != http.StatusFound {
		t.Fatal("home not replaced")
	}

	// the hour comes: everything opens by itself
	e.put(t, "/api/admin/settings", admin, `{"launch_gate":"tout"}`)
	e.clock.Add(2 * time.Hour)
	sendPixel(t, c, 2, 2)
	if m := next(t, c, func(m map[string]any) bool { return m["type"] == "error" || isPixel(m) }); !isPixel(m) {
		t.Fatalf("still closed after the release: %v", m)
	}
	if code, _ := get("/ace", lucie); code != 200 {
		t.Fatal("gate still closed after the release")
	}
}

func TestSeedLaunchOnlyOnce(t *testing.T) {
	e := newEnv(t, 8)
	if err := e.api.SeedLaunch("pas une date", "tout"); err == nil {
		t.Fatal("bad date accepted")
	}
	if err := e.api.SeedLaunch("2030-10-24T20:00", "accueil"); err != nil {
		t.Fatal(err)
	}
	if st := e.store.LoadSettings(); st.LaunchAt != "2030-10-24T20:00" || st.LaunchGate != "accueil" {
		t.Fatalf("seeded %+v", st)
	}
	e.api.SeedLaunch("2031-01-01T10:00", "tout") // an admin may have changed it since: kept
	if st := e.api.Settings(); st.LaunchAt != "2030-10-24T20:00" {
		t.Fatalf("seed overwrote the date: %s", st.LaunchAt)
	}
}

func TestLaunchTrailerVideo(t *testing.T) {
	e := newEnv(t, 8)
	dir := t.TempDir()
	e.api.SetMediaDir(dir)
	if v := e.getJSON(t, "/api/status", nil)["launch"].(map[string]any)["video"]; v != nil {
		t.Fatalf("video without a file: %v", v)
	}
	os.WriteFile(filepath.Join(dir, "bande-annonce.mp4"), []byte("mp4"), 0644)
	os.WriteFile(filepath.Join(dir, "bande-annonce.jpg"), []byte("jpg"), 0644)
	l := e.getJSON(t, "/api/status", nil)["launch"].(map[string]any)
	if v, _ := l["video"].(string); !strings.HasPrefix(v, "/media/bande-annonce.mp4?v=") || !strings.HasPrefix(l["poster"].(string), "/media/bande-annonce.jpg") {
		t.Fatalf("launch %v", l)
	}
	page := e.api.RewritePage(httptest.NewRequest("GET", "https://place.justbetter.fr/bande-annonce", nil), `<meta property="og:video" content="{{VIDEO}}">`)
	if !strings.Contains(page, `content="https://place.justbetter.fr/media/bande-annonce.mp4?v=`) {
		t.Fatalf("og:video %s", page)
	}
}

func TestNoGuestMode(t *testing.T) {
	e := newEnv(t, 8)
	e.auth.SetAdmins([]string{"tiago"})
	admin, lucie := e.login(t, "Tiago"), e.login(t, "Lucie")
	gate := e.api.LaunchGate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	get := func(path string, ck *http.Cookie) (int, string) {
		req := httptest.NewRequest("GET", path, nil)
		if ck != nil {
			req.AddCookie(ck)
		}
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, req)
		return rec.Code, rec.Header().Get("Location")
	}
	// default: an account is needed
	if code, loc := get("/ace?x=10&y=20", nil); code != http.StatusFound || loc != "/login?next=%2Face%3Fx%3D10%26y%3D20" {
		t.Fatalf("guest on the canvas: %d %s", code, loc)
	}
	for _, p := range []string{"/", "/home", "/lancement", "/bande-annonce", "/login", "/place.png", "/api/status", "/js/app.js", "/auth/justbetter"} {
		if code, _ := get(p, nil); code != 200 {
			t.Fatalf("%s should stay public: %d", p, code)
		}
	}
	for _, p := range []string{"/timelapse", "/musee", "/u/lucie"} {
		if code, _ := get(p, nil); code != http.StatusFound {
			t.Fatalf("%s open to guests: %d", p, code)
		}
	}
	if code, _ := get("/ace", lucie); code != 200 {
		t.Fatal("players get in")
	}
	if e.getJSON(t, "/api/status", nil)["guests"] != false {
		t.Fatal("status says guests are allowed")
	}
	// guest mode switched on in the settings
	e.put(t, "/api/admin/settings", admin, `{"guests":true}`)
	if code, _ := get("/ace", nil); code != 200 {
		t.Fatal("guest mode on, canvas still closed")
	}
}
