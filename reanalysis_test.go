package place

import (
	"image"
	"testing"
	"time"
)

// New captures make « Indémodable » true a year later: the authors get it; « Vétéran »,
// proposed at validation but not chosen by the admin, stays theirs to decide.
func TestReanalysisGivesBadgesEarnedWithNewCaptures(t *testing.T) {
	e := newEnv(t, 8)
	day := time.Date(2022, 10, 1, 12, 0, 0, 0, time.UTC)
	e.store.now = func() time.Time { return day }
	dir := t.TempDir()
	blue := [3]uint8{0x5e, 0xb3, 0xff}
	f1 := blankImg(16, 16)
	f2 := paint(f1, 0, 0, 4, 4, blue)
	caps := []struct {
		at  time.Time
		img *image.NRGBA
	}{{time.Date(2022, 9, 10, 20, 0, 0, 0, time.UTC), f1}, {time.Date(2022, 9, 11, 20, 0, 0, 0, time.UTC), f2}}
	writeCaptures(t, dir, caps)
	b := NewBackups([]string{dir}, "")
	if err := b.Refresh(); err != nil {
		t.Fatal(err)
	}
	e.api.SetBackups(b)
	e.auth.SetAdmins([]string{"tiago"})
	admin, brin := e.login(t, "Tiago"), e.login(t, "Brindille")

	_, m := e.post(t, "/api/claims", brin, map[string]any{"masque": zone, "titre": "Bord de mer"})
	id := itoa(int(m["id"].(float64)))
	claim := e.getJSON(t, "/api/claims/"+id, nil)["claim"].(map[string]any)
	proposed := claim["analyse"].(map[string]any)["badges"].([]any)
	if !containsAny(proposed, "veteran") || containsAny(proposed, "indemodable") {
		t.Fatalf("badges proposed at claim time %v", proposed)
	}
	if status, d := e.post(t, "/api/admin/claims/"+id+"/decision", admin, map[string]any{"action": "valider", "badges": []string{"pionnier"}}); status != 200 {
		t.Fatalf("validate %v", d)
	}

	// a year later the zone is still intact, and the new captures reach the index
	day = time.Date(2023, 11, 1, 12, 0, 0, 0, time.UTC)
	writeCaptures(t, dir, []struct {
		at  time.Time
		img *image.NRGBA
	}{{time.Date(2023, 10, 20, 20, 0, 0, 0, time.UTC), f2}})
	if err := b.Refresh(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for r := e.api.LastReanalysis(); r == nil || r.Frames != 3; r = e.api.LastReanalysis() {
		if time.Now().After(deadline) {
			t.Fatal("no reanalysis after the index update")
		}
		time.Sleep(50 * time.Millisecond)
	}
	p := e.getJSON(t, "/api/users/brindille", nil)
	if !hasBadge(p, "indemodable") || !hasBadge(p, "pionnier") || hasBadge(p, "veteran") {
		t.Fatalf("badges after reanalysis %v", p["badges"])
	}
	brin, admin = e.login(t, "Brindille"), e.login(t, "Tiago") // a year later: sessions expired
	if a := e.getJSON(t, "/api/alerts", brin)["alerts"].([]any); len(a) == 0 || a[0].(map[string]any)["kind"] != "badge_oeuvre" {
		t.Fatalf("author not told %v", a)
	}
	if o := e.getJSON(t, "/api/oeuvres", nil)["oeuvres"].([]any)[0].(map[string]any); o["terminee_le"] == nil {
		t.Fatalf("artwork dates %v", o)
	}
	// running it again gives nothing more
	if status, r := e.post(t, "/api/admin/reanalyse", admin, nil); status != 200 || len(r["badges"].([]any)) != 0 || r["oeuvres"] != 1.0 {
		t.Fatalf("second run %d %v", status, r)
	}
}

func containsAny(list []any, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
