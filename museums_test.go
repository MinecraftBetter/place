package place

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

// myMusee reads the editor data of a player.
func myMusee(t *testing.T, e *testEnv, ck *http.Cookie) map[string]any {
	t.Helper()
	return e.getJSON(t, "/api/me/musee", ck)
}

func putMusee(t *testing.T, e *testEnv, ck *http.Cookie, m map[string]any, publie bool) int {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"musee": m, "publie": publie})
	return e.put(t, "/api/me/musee", ck, string(b))
}

func uploadSon(t *testing.T, e *testEnv, ck *http.Cookie, typ string, data []byte, duree string) (int, map[string]any) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("type", typ)
	mw.WriteField("duree", duree)
	fw, _ := mw.CreateFormFile("fichier", "son.ogg")
	fw.Write(data)
	mw.Close()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/me/musee/sons", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(ck)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	m := map[string]any{}
	json.NewDecoder(res.Body).Decode(&m)
	return res.StatusCode, m
}

// an artwork validated for Brindille, id returned
func museumArtwork(t *testing.T, e *testEnv, owner, admin *http.Cookie) float64 {
	t.Helper()
	_, m := e.post(t, "/api/claims", owner, map[string]any{"masque": zone, "titre": "Bord de mer"})
	id := itoa(int(m["id"].(float64)))
	if status, d := e.post(t, "/api/admin/claims/"+id+"/decision", admin, map[string]any{"action": "valider"}); status != 200 {
		t.Fatalf("validate %v", d)
	}
	return e.getJSON(t, "/api/oeuvres", nil)["oeuvres"].([]any)[0].(map[string]any)["id"].(float64)
}

func TestMuseumEditVisitGuestbook(t *testing.T) {
	e := claimEnv(t)
	admin, brin, lucie := e.login(t, "Tiago"), e.login(t, "Brindille"), e.login(t, "Lucie")
	oid := museumArtwork(t, e, brin, admin)

	// a fresh museum: defaults, not open yet
	d := myMusee(t, e, brin)
	if d["existe"] != false || d["musee"].(map[string]any)["nom"] != "Le musée de Brindille" || len(d["miennes"].([]any)) != 1 {
		t.Fatalf("editor defaults %v", d)
	}
	if status := e.getStatus(t, "/api/musees/brindille", nil); status != 404 {
		t.Fatalf("unpublished museum visible: %d", status)
	}

	m := d["musee"].(map[string]any)
	m["theme"] = "etoiles"
	m["mur"] = "#511E9F"
	m["accueil"] = "  Des   vagues et des étoiles  "
	m["musique"] = "son:999" // not mine: falls back to a built-in tune
	m["salles"] = []any{
		map[string]any{"titre": "Mes œuvres", "oeuvres": []any{
			map[string]any{"oeuvre_id": oid, "animation": "construction", "texte": "Ma première", "spot": "#ff63aa", "vedette": true, "cadre": "n'importe quoi"},
			map[string]any{"oeuvre_id": oid}, // twice: dropped
			map[string]any{"oeuvre_id": 12345},
		}},
	}
	if status := putMusee(t, e, brin, m, true); status != 200 {
		t.Fatalf("save %d", status)
	}
	v := e.getJSON(t, "/api/musees/brindille?visite=1", lucie)
	mv := v["musee"].(map[string]any)
	works := mv["salles"].([]any)[0].(map[string]any)["oeuvres"].([]any)
	if mv["theme"] != "etoiles" || mv["accueil"] != "Des vagues et des étoiles" || mv["musique"] != "vagues" || len(works) != 1 {
		t.Fatalf("visit %v", mv)
	}
	w := works[0].(map[string]any)
	if w["cadre"] != "musee" || w["animation"] != "construction" || w["oeuvre"].(map[string]any)["titre"] != "Bord de mer" {
		t.Fatalf("work %v", w)
	}
	if v["visites"] != 1.0 {
		t.Fatalf("visit count %v", v["visites"])
	}
	e.getJSON(t, "/api/musees/brindille?visite=1", lucie) // same day: counted once
	if v := e.getJSON(t, "/api/musees/brindille?visite=1", brin); v["visites"] != 1.0 || v["moi"] != true {
		t.Fatalf("owner visit counted %v", v["visites"])
	}

	// directory, profile, public museum page
	if l := e.getJSON(t, "/api/musees", nil)["musees"].([]any); len(l) != 1 || l[0].(map[string]any)["nom"] != "Le musée de Brindille" {
		t.Fatalf("directory %v", l)
	}
	if p := e.getJSON(t, "/api/users/brindille", nil)["extra"].(map[string]any); p["musee"] == nil {
		t.Fatal("profile has no museum link")
	}
	if mu := e.getJSON(t, "/api/musee", nil)["musees"].([]any); len(mu) != 1 {
		t.Fatalf("public museum page %v", mu)
	}

	// guestbook, like, alert to the owner
	if status, _ := e.post(t, "/api/musees/brindille/livre-or", nil, map[string]any{"texte": "coucou"}); status != 401 {
		t.Fatal("guest signed the guestbook")
	}
	if status, r := e.post(t, "/api/musees/brindille/livre-or", lucie, map[string]any{"texte": "  Trop  beau !  "}); status != 200 || r["entree"].(map[string]any)["texte"] != "Trop beau !" {
		t.Fatalf("sign %d %v", status, r)
	}
	if status, _ := e.post(t, "/api/musees/brindille/livre-or", lucie, map[string]any{"texte": "espèce de connard"}); status != 400 {
		t.Fatal("forbidden word accepted in the guestbook")
	}
	if _, r := e.post(t, "/api/musees/brindille/like", lucie, nil); r["liked"] != true || r["likes"] != 1.0 {
		t.Fatalf("like %v", r)
	}
	v = e.getJSON(t, "/api/musees/brindille", lucie)
	if v["mots"] != 1.0 || v["liked"] != true {
		t.Fatalf("after guestbook %v %v", v["mots"], v["liked"])
	}
	if a := e.getJSON(t, "/api/alerts", brin)["alerts"].([]any); len(a) == 0 || a[0].(map[string]any)["kind"] != "livre_or" {
		t.Fatalf("owner not told about the guestbook: %v", a)
	}

	// report + hide a guestbook entry, then undo
	entry := itoa(int(v["livre"].([]any)[0].(map[string]any)["id"].(float64)))
	if status, _ := e.post(t, "/api/reports", brin, map[string]any{"type": "livre_or", "cible": "livre_or:" + entry}); status != 200 {
		t.Fatal("report guestbook entry")
	}
	rep := e.getJSON(t, "/api/admin/reports", admin)["reports"].([]any)[0].(map[string]any)
	e.post(t, "/api/admin/reports/"+itoa(int(rep["id"].(float64)))+"/masquer", admin, nil)
	if v := e.getJSON(t, "/api/musees/brindille", lucie); v["mots"] != 0.0 {
		t.Fatal("hidden entry still shown")
	}
	j := e.getJSON(t, "/api/admin/journal?filtre=moderation", admin)["actions"].([]any)
	e.post(t, "/api/admin/journal/"+itoa(int(j[0].(map[string]any)["id"].(float64)))+"/undo", admin, nil)
	if v := e.getJSON(t, "/api/musees/brindille", lucie); v["mots"] != 1.0 {
		t.Fatal("undo did not bring the entry back")
	}

	// private museum
	m["visibilite"] = "moi"
	putMusee(t, e, brin, m, true)
	if status := e.getStatus(t, "/api/musees/brindille", lucie); status != 403 {
		t.Fatalf("private museum: %d", status)
	}
	if l := e.getJSON(t, "/api/musees", lucie)["musees"].([]any); len(l) != 0 {
		t.Fatal("private museum listed")
	}
	if status := e.getStatus(t, "/api/musees/brindille", admin); status != 200 {
		t.Fatal("admins can check private museums")
	}
}

func TestMuseumSoundsModeration(t *testing.T) {
	e := claimEnv(t)
	e.api.SetMediaDir(t.TempDir())
	admin, brin, lucie := e.login(t, "Tiago"), e.login(t, "Brindille"), e.login(t, "Lucie")
	oid := museumArtwork(t, e, brin, admin)

	if status, _ := uploadSon(t, e, brin, "voix", []byte("<html>pas un son</html>"), "3"); status != 400 {
		t.Fatal("non-audio accepted")
	}
	ogg := append([]byte("OggS"), bytes.Repeat([]byte{0}, 200)...)
	if status, _ := uploadSon(t, e, brin, "voix", ogg, "45"); status != 400 {
		t.Fatal("45 s voice comment accepted")
	}
	status, r := uploadSon(t, e, brin, "voix", ogg, "12.5")
	if status != 200 {
		t.Fatalf("upload %d %v", status, r)
	}
	son := r["son"].(map[string]any)
	if son["statut"] != "attente" || !strings.HasPrefix(son["url"].(string), "/media/sons/") {
		t.Fatalf("sound %v", son)
	}
	sid := son["id"].(float64)
	m := myMusee(t, e, brin)["musee"].(map[string]any)
	m["salles"] = []any{map[string]any{"titre": "Salle", "oeuvres": []any{map[string]any{"oeuvre_id": oid, "voix_id": sid}}}}
	putMusee(t, e, brin, m, true)
	voix := func(ck *http.Cookie) any {
		v := e.getJSON(t, "/api/musees/brindille", ck)["musee"].(map[string]any)
		return v["salles"].([]any)[0].(map[string]any)["oeuvres"].([]any)[0].(map[string]any)["voix"]
	}
	if voix(lucie) != nil || voix(brin) == nil {
		t.Fatal("a sound waiting for the team is only heard by its owner")
	}
	reports := e.getJSON(t, "/api/admin/reports?type=son", admin)["reports"].([]any)
	if len(reports) != 1 {
		t.Fatalf("sound not queued: %v", reports)
	}
	e.post(t, "/api/admin/reports/"+itoa(int(reports[0].(map[string]any)["id"].(float64)))+"/garder", admin, nil)
	if voix(lucie) == nil {
		t.Fatal("accepted sound not heard")
	}
	j := e.getJSON(t, "/api/admin/journal?filtre=moderation", admin)["actions"].([]any)
	e.post(t, "/api/admin/journal/"+itoa(int(j[0].(map[string]any)["id"].(float64)))+"/undo", admin, nil)
	if voix(lucie) != nil {
		t.Fatal("undo of « garder » did not put the sound back in the queue")
	}
	// someone else's sound cannot be used
	lm := myMusee(t, e, lucie)["musee"].(map[string]any)
	lm["salles"] = []any{map[string]any{"titre": "Salle", "oeuvres": []any{map[string]any{"oeuvre_id": oid, "voix_id": sid}}}}
	putMusee(t, e, lucie, lm, true)
	if w := e.getJSON(t, "/api/musees/lucie", lucie)["musee"].(map[string]any)["salles"].([]any)[0].(map[string]any)["oeuvres"].([]any)[0].(map[string]any); w["voix_id"] != 0.0 {
		t.Fatalf("borrowed a sound: %v", w["voix_id"])
	}
}

func (e *testEnv) getStatus(t *testing.T, path string, ck *http.Cookie) int {
	t.Helper()
	req, _ := http.NewRequest("GET", e.srv.URL+path, nil)
	if ck != nil {
		req.AddCookie(ck)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}
