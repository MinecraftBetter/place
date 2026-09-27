package place

import (
	"encoding/json"
	"image"
	"net/http"
	"strings"
	"testing"
	"time"
)

// claimEnv: a 16×16 canvas, backups where a 4×4 artwork appears at (0,0) in 2022.
func claimEnv(t *testing.T) *testEnv {
	t.Helper()
	e := newEnv(t, 8)
	dir := t.TempDir()
	blue := [3]uint8{0x5e, 0xb3, 0xff}
	f1 := blankImg(16, 16)
	f2 := paint(f1, 0, 0, 4, 4, blue)
	writeCaptures(t, dir, []struct {
		at  time.Time
		img *image.NRGBA
	}{{time.Date(2022, 9, 10, 20, 0, 0, 0, time.UTC), f1}, {time.Date(2022, 9, 11, 20, 0, 0, 0, time.UTC), f2}})
	b := NewBackups([]string{dir}, "")
	if err := b.Refresh(); err != nil {
		t.Fatal(err)
	}
	e.api.SetBackups(b)
	e.auth.SetAdmins([]string{"tiago"})
	return e
}

func (e *testEnv) post(t *testing.T, path string, ck *http.Cookie, body any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", e.srv.URL+path, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	if ck != nil {
		req.AddCookie(ck)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	m := map[string]any{}
	json.NewDecoder(res.Body).Decode(&m)
	return res.StatusCode, m
}

var zone = map[string]any{"type": "rect", "rect": []int{0, 0, 4, 4}}

func TestClaimFlow(t *testing.T) {
	e := claimEnv(t)
	brin, nyx, zozo, lucie := e.login(t, "Brindille"), e.login(t, "Nyx"), e.login(t, "Zozo42"), e.login(t, "Lucie_px")
	admin := e.login(t, "Tiago")

	// Preview with the backup analysis.
	status, a := e.post(t, "/api/claims/analyse", brin, map[string]any{"masque": zone})
	an := a["analyse"].(map[string]any)
	if status != 200 || a["pixels"] != 16.0 || an["pixels_poses_estimes"] != 16.0 || an["apparition"] == nil {
		t.Fatalf("analyse %d %v", status, a)
	}
	badges := an["badges"].([]any)
	if len(badges) != 3 || badges[0] != "pionnier" || badges[1] != "veteran" || badges[2] != "indemodable" {
		t.Fatalf("proposed badges %v", badges)
	}

	// Invalid claims.
	if status, m := e.post(t, "/api/claims", brin, map[string]any{"masque": zone, "titre": "x"}); status != 400 || m["field"] != "titre" {
		t.Fatalf("short title: %d %v", status, m)
	}
	if status, _ := e.post(t, "/api/claims", nil, map[string]any{"masque": zone, "titre": "Bord de mer"}); status != 401 {
		t.Fatalf("guest claim: %d", status)
	}
	if status, m := e.post(t, "/api/claims", brin, map[string]any{"masque": zone, "titre": "Bord", "co_auteurs": []string{"personne"}}); status != 400 || m["field"] != "co_auteurs" {
		t.Fatalf("unknown co-author: %d %v", status, m)
	}

	status, m := e.post(t, "/api/claims", brin, map[string]any{
		"masque": zone, "titre": "Bord de mer", "message": "Le premier soir !", "co_auteurs": []string{"nyx"},
		"repartition": "poids", "poids": map[string]float64{"moi": 3, "nyx": 1},
	})
	if status != 200 {
		t.Fatalf("create: %d %v", status, m)
	}
	id := int64(m["id"].(float64))
	path := "/api/claims/" + itoa(int(id))

	// A rival claim on the same zone makes it "à départager".
	_, m2 := e.post(t, "/api/claims", zozo, map[string]any{"masque": map[string]any{"type": "rect", "rect": []int{0, 0, 5, 5}}, "titre": "Mon carré"})
	rival := int64(m2["id"].(float64))

	list := e.getJSON(t, "/api/claims?statut=attente", nil)
	cs := list["claims"].([]any)
	if len(cs) != 2 || list["counts"].(map[string]any)["attente"] != 2.0 {
		t.Fatalf("list %v", list)
	}
	first := cs[1].(map[string]any)
	if first["titre"] != "Bord de mer" || len(first["conflits"].([]any)) != 1 || !strings.Contains(first["resume"].(string), "Zozo42") {
		t.Fatalf("claim in list %v", first)
	}

	// Votes: not on your own claim, not as a co-author; others can confirm or doubt.
	if status, _ := e.post(t, path+"/votes", brin, map[string]any{"type": "confirme"}); status != 400 {
		t.Fatalf("self vote: %d", status)
	}
	if status, _ := e.post(t, path+"/votes", nyx, map[string]any{"type": "confirme"}); status != 400 {
		t.Fatalf("co-author vote: %d", status)
	}
	if status, _ := e.post(t, path+"/votes", lucie, map[string]any{"type": "doute", "commentaire": "Pas sûre"}); status != 200 {
		t.Fatalf("doubt: %d", status)
	}
	_, v := e.post(t, path+"/votes", lucie, map[string]any{"type": "confirme"})
	c := v["claim"].(map[string]any)
	if c["confirmations"] != 1.0 || c["doutes"] != 0.0 || c["mon_vote"] != "confirme" {
		t.Fatalf("changed vote %v", c)
	}
	if status, _ := e.post(t, path+"/confirm", nyx, map[string]any{"accepte": true}); status != 200 {
		t.Fatalf("confirm co-author: %d", status)
	}

	// Admin only.
	if status, _ := e.get(t, "/api/admin/claims", brin); status != 403 {
		t.Fatalf("player on admin: %d", status)
	}
	adminList := e.getJSON(t, "/api/admin/claims", admin)
	if n := len(adminList["claims"].([]any)); n != 2 {
		t.Fatalf("admin queue %d", n)
	}
	if status, m := e.post(t, "/api/admin/claims/"+itoa(int(rival))+"/decision", admin, map[string]any{"action": "refuser"}); status != 400 || m["field"] != "motif" {
		t.Fatalf("refuse without reason: %d %v", status, m)
	}

	// Validate with the proposed badges: artwork, pioneer pixels 3:1, badges.
	status, d := e.post(t, "/api/admin/claims/"+itoa(int(id))+"/decision", admin, map[string]any{"action": "valider", "badges": []string{"pionnier", "veteran"}})
	if status != 200 {
		t.Fatalf("validate: %d %v", status, d)
	}
	pb := e.getJSON(t, "/api/users/brindille", nil)
	pn := e.getJSON(t, "/api/users/nyx", nil)
	if pb["pixels_pionniers"] != 12.0 || pn["pixels_pionniers"] != 4.0 {
		t.Fatalf("pioneer pixels %v / %v", pb["pixels_pionniers"], pn["pixels_pionniers"])
	}
	if !hasBadge(pb, "pionnier") || !hasBadge(pn, "veteran") {
		t.Fatalf("badges not given")
	}
	works := pb["extra"].(map[string]any)["oeuvres"].([]any)
	if len(works) != 1 || works[0].(map[string]any)["titre"] != "Bord de mer" {
		t.Fatalf("profile artworks %v", pb["extra"])
	}
	px := e.getJSON(t, "/api/pixel?x=1&y=1", nil)
	ow := px["oeuvre"].(map[string]any)
	if ow["titre"] != "Bord de mer" || ow["origine"] != "avant_migration" || len(ow["auteurs"].([]any)) != 2 {
		t.Fatalf("inspector artwork %v", ow)
	}
	if px := e.getJSON(t, "/api/pixel?x=10&y=10", nil); px["oeuvre"] != nil {
		t.Fatal("pixel outside the artwork")
	}
	if status, _ := e.post(t, path+"/votes", lucie, map[string]any{"type": "doute"}); status != 400 {
		t.Fatal("vote after the decision")
	}

	// Undo: everything is put back.
	if status, m := e.post(t, "/api/admin/claims/"+itoa(int(id))+"/undo", admin, nil); status != 200 {
		t.Fatalf("undo: %d %v", status, m)
	}
	pb = e.getJSON(t, "/api/users/brindille", nil)
	if pb["pixels_pionniers"] != 0.0 || hasBadge(pb, "pionnier") {
		t.Fatalf("undo left %v", pb["pixels_pionniers"])
	}
	if px := e.getJSON(t, "/api/pixel?x=1&y=1", nil); px["oeuvre"] != nil {
		t.Fatal("artwork still there after undo")
	}

	// Merge the two claims: everyone becomes an author.
	status, d = e.post(t, "/api/admin/claims/"+itoa(int(id))+"/decision", admin, map[string]any{"action": "fusionner", "fusion": []int64{rival}, "badges": []string{"pionnier"}})
	if status != 200 {
		t.Fatalf("merge: %d %v", status, d)
	}
	pz := e.getJSON(t, "/api/users/zozo42", nil)
	if !hasBadge(pz, "pionnier") || pz["pixels_pionniers"].(float64) <= 0 {
		t.Fatalf("merged author %v", pz["pixels_pionniers"])
	}
	counts := e.getJSON(t, "/api/claims?statut=validee", nil)["counts"].(map[string]any)
	if counts["validee"] != 2.0 || counts["attente"] != 0.0 {
		t.Fatalf("counts after merge %v", counts)
	}
}

func TestAttributeAndCancel(t *testing.T) {
	e := claimEnv(t)
	bot, poulpe, admin := e.login(t, "Bot"), e.login(t, "Poulpe"), e.login(t, "Tiago")
	_ = poulpe
	_, m := e.post(t, "/api/claims", bot, map[string]any{"masque": zone, "titre": "La grande torche"})
	id := itoa(int(m["id"].(float64)))
	status, d := e.post(t, "/api/admin/claims/"+id+"/decision", admin, map[string]any{"action": "attribuer", "user": "poulpe"})
	if status != 200 {
		t.Fatalf("attribute: %d %v", status, d)
	}
	c := d["claim"].(map[string]any)
	if c["statut"] != "refusee" || !strings.Contains(c["motif"].(string), "Poulpe") {
		t.Fatalf("attributed claim %v", c)
	}
	if px := e.getJSON(t, "/api/pixel?x=0&y=0", nil); px["oeuvre"].(map[string]any)["auteurs"].([]any)[0].(map[string]any)["user"].(map[string]any)["pseudo"] != "Poulpe" {
		t.Fatalf("artwork author %v", px["oeuvre"])
	}

	_, m = e.post(t, "/api/claims", bot, map[string]any{"masque": map[string]any{"type": "rect", "rect": []int{8, 8, 2, 2}}, "titre": "Autre"})
	id = itoa(int(m["id"].(float64)))
	if status, _ := e.post(t, "/api/claims/"+id+"/cancel", poulpe, nil); status != 400 {
		t.Fatal("someone else cancelled the claim")
	}
	if status, _ := e.post(t, "/api/claims/"+id+"/cancel", bot, nil); status != 200 {
		t.Fatal("cancel")
	}
	if n := len(e.getJSON(t, "/api/claims", nil)["claims"].([]any)); n != 0 {
		t.Fatalf("cancelled claim still listed: %d", n)
	}
}

func TestCropAndSearch(t *testing.T) {
	e := claimEnv(t)
	status, b := e.get(t, "/api/crop.png?x=2&y=2&w=4&h=3&z=5", nil)
	w, h, err := decodePNGSize(b)
	if status != 200 || err != nil || w != 20 || h != 15 {
		t.Fatalf("crop %d %dx%d %v", status, w, h, err)
	}
	if status, _ := e.get(t, "/api/crop.png?x=100&y=100&w=4&h=4", nil); status != 400 {
		t.Fatalf("crop outside: %d", status)
	}
	e.login(t, "Mamie Pixel")
	e.login(t, "Kaelen")
	users := e.getJSON(t, "/api/users/search?q=mam", nil)["users"].([]any)
	if len(users) != 1 || users[0].(map[string]any)["pseudo"] != "Mamie Pixel" {
		t.Fatalf("search %v", users)
	}
}

func hasBadge(p map[string]any, id string) bool {
	for _, b := range p["badges"].([]any) {
		bm := b.(map[string]any)
		if bm["id"] == id && bm["earned"] == true {
			return true
		}
	}
	return false
}
