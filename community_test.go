package place

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

func TestActivityGroupsPixels(t *testing.T) {
	e := newEnv(t, 8)
	e.hub.SetCooldown(0)
	conn := e.dial(t, e.login(t, "Brindille"), "")
	for i := 0; i < 3; i++ {
		sendPixel(t, conn, i, 0)
		next(t, conn, isPixel)
	}
	e.api.community.mu.Lock()
	g := e.api.community.groups[1]
	e.api.community.mu.Unlock()
	if g == nil || g.act.N != 3 {
		t.Fatalf("group %+v", g)
	}
	items := e.getJSON(t, "/api/activity", nil)["items"].([]any)
	var pixels int
	for _, it := range items {
		if it.(map[string]any)["kind"] == "pixels" {
			pixels++
		}
	}
	if pixels != 1 {
		t.Fatalf("pixels lines %d, want 1 (grouped)", pixels)
	}
	// another player: another line
	other := e.dial(t, e.login(t, "Kaelen"), "")
	sendPixel(t, other, 15, 15)
	next(t, other, func(m map[string]any) bool { return isPixel(m) && m["x"] == 15.0 })
	// the feed is written right after the broadcast
	for i := 0; i < 50; i++ {
		if items = e.getJSON(t, "/api/activity?kind=pixels", nil)["items"].([]any); len(items) == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(items) != 2 {
		t.Fatalf("lines %d, want 2", len(items))
	}
}

func TestRetouchAlertAndCommunity(t *testing.T) {
	e := claimEnv(t)
	e.hub.SetCooldown(0)
	brin, admin, zozo := e.login(t, "Brindille"), e.login(t, "Tiago"), e.login(t, "Zozo42")
	_, m := e.post(t, "/api/claims", brin, map[string]any{"masque": zone, "titre": "Bord de mer"})
	id := itoa(int(m["id"].(float64)))
	if status, d := e.post(t, "/api/admin/claims/"+id+"/decision", admin, map[string]any{"action": "valider", "badges": []string{"pionnier"}}); status != 200 {
		t.Fatalf("validate %d %v", status, d)
	}
	me := e.getJSON(t, "/api/me", brin)
	if me["alerts"] != 1.0 { // claim validated
		t.Fatalf("alerts after validation %v", me["alerts"])
	}

	owner := e.dial(t, brin, "")
	joker := e.dial(t, zozo, "")
	for i := 0; i < retouchThreshold; i++ {
		joker.WriteMessage(1, []byte(`{"x":`+itoa(i%4)+`,"y":`+itoa(i/4)+`,"color":{"R":0,"G":0,"B":0,"A":255}}`))
	}
	a := next(t, owner, isType("alert"))
	if a["kind"] != "retouche" || a["titre"] != "Bord de mer" || a["count"] != float64(retouchThreshold) {
		t.Fatalf("alert %v", a)
	}
	alerts := e.getJSON(t, "/api/alerts", brin)
	if alerts["unread"] != 2.0 {
		t.Fatalf("alerts %v", alerts)
	}
	e.post(t, "/api/alerts", brin, nil)
	if n := e.getJSON(t, "/api/me", brin)["alerts"]; n != 0.0 {
		t.Fatalf("unread after reading %v", n)
	}

	// artwork page: retoucher listed, intact < 100 %, keep resets the reference
	o := e.getJSON(t, "/api/oeuvres/1", brin)
	ow := o["oeuvre"].(map[string]any)
	extra := ow["extra"].(map[string]any)
	if extra["retouchee"] != true || extra["intact_pct"].(float64) >= 95 || o["auteur"] != true {
		t.Fatalf("artwork after retouch %v / %v", extra, o["auteur"])
	}
	contrib := o["contributeurs"].([]any)
	if len(contrib) != 2 || contrib[1].(map[string]any)["role"] != "retouche" || contrib[1].(map[string]any)["pixels"] != float64(retouchThreshold) {
		t.Fatalf("contributors %v", contrib)
	}
	if len(o["histoire"].([]any)) == 0 {
		t.Fatal("empty history")
	}
	if status, _ := e.post(t, "/api/oeuvres/1/keep", zozo, nil); status != 403 {
		t.Fatalf("keep by a non-author: %d", status)
	}
	e.post(t, "/api/oeuvres/1/keep", brin, nil)
	if extra := e.getJSON(t, "/api/oeuvres/1", nil)["oeuvre"].(map[string]any)["extra"].(map[string]any); extra["retouchee"] != false {
		t.Fatalf("still retouched after keep %v", extra)
	}
	for _, p := range []string{"/api/oeuvres/1/reference.png", "/api/oeuvres/1/avant.png?z=4", "/api/oeuvres/1/construction.png?n=6"} {
		if status, b := e.get(t, p, nil); status != 200 || len(b) < 50 {
			t.Fatalf("%s: %d", p, status)
		}
	}

	// likes, exhibition, museum, leaderboards
	if status, l := e.post(t, "/api/oeuvres/1/like", zozo, map[string]any{"on": true}); status != 200 || l["likes"] != 1.0 {
		t.Fatalf("like %d %v", status, l)
	}
	if status, _ := e.post(t, "/api/oeuvres/1/exposition", zozo, map[string]any{"salle": "paysages"}); status != 403 {
		t.Fatalf("exhibition by a non-author: %d", status)
	}
	if status, m := e.post(t, "/api/oeuvres/1/exposition", brin, map[string]any{"salle": "nulle-part"}); status != 400 || m["field"] != "salle" {
		t.Fatalf("unknown room: %d %v", status, m)
	}
	if status, _ := e.post(t, "/api/oeuvres/1/exposition", brin, map[string]any{"salle": "paysages", "cadre": "or", "mot": "Le premier soir.", "visite": true}); status != 200 {
		t.Fatalf("exhibit: %d", status)
	}
	musee := e.getJSON(t, "/api/musee", nil)
	if musee["affiche"].(map[string]any)["titre"] != "Bord de mer" || len(musee["oeuvres"].([]any)) != 1 {
		t.Fatalf("museum %v", musee)
	}
	if n := len(e.getJSON(t, "/api/musee/visite", nil)["oeuvres"].([]any)); n != 1 {
		t.Fatalf("tour %d", n)
	}
	board := e.getJSON(t, "/api/leaderboard?kind=joueurs&period=jour", zozo)
	if board["moi"].(map[string]any)["value"] != float64(retouchThreshold) || len(board["entries"].([]any)) != 1 {
		t.Fatalf("players board %v", board)
	}
	works := e.getJSON(t, "/api/leaderboard?kind=oeuvres&period=tout", nil)["entries"].([]any)
	if len(works) != 1 || works[0].(map[string]any)["value"] != 1.0 {
		t.Fatalf("artworks board %v", works)
	}
	kinds := map[string]bool{}
	for _, it := range e.getJSON(t, "/api/activity", nil)["items"].([]any) {
		kinds[it.(map[string]any)["kind"].(string)] = true
	}
	for _, k := range []string{"claim", "claim_valid", "retouche", "exposition", "pixels"} {
		if !kinds[k] {
			t.Errorf("no %q line in the feed (%v)", k, kinds)
		}
	}
}

func TestSnapshots(t *testing.T) {
	e := claimEnv(t)
	s := e.getJSON(t, "/api/snapshots", nil)
	days := s["days"].([]any)
	if len(days) != 2 || s["width"] != 16.0 {
		t.Fatalf("days %v", s)
	}
	d := days[1].(map[string]any)
	status, b := e.get(t, "/api/snapshots/"+itoa(int(d["frame"].(float64)))+".png", nil)
	if status != 200 || len(b) < 50 {
		t.Fatalf("capture: %d", status)
	}
	status, b = e.get(t, "/api/snapshots/day/"+d["date"].(string)+".bin", nil)
	if status != 200 || string(b[:4]) != "BPDY" {
		t.Fatalf("day replay: %d", status)
	}
	r := bytes.NewReader(b[4:])
	var ver, start, frames uint32
	binary.Read(r, binary.LittleEndian, &ver)
	binary.Read(r, binary.LittleEndian, &start)
	binary.Read(r, binary.LittleEndian, &frames)
	times := make([]int64, frames)
	binary.Read(r, binary.LittleEndian, times)
	var n uint32
	binary.Read(r, binary.LittleEndian, &n)
	if ver != 1 || start != 0 || frames != 1 || n != 16 || time.UnixMilli(times[0]).Year() != 2022 {
		t.Fatalf("header ver %d start %d frames %d changes %d", ver, start, frames, n)
	}
	months := s["months"].([]any)
	if len(months) != 1 || months[0].(map[string]any)["days"] != 2.0 {
		t.Fatalf("months %v", months)
	}
}
