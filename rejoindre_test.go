package place

import "testing"

func TestRejoindreBuild(t *testing.T) {
	e := claimEnv(t)
	brin, admin, zozo, kae := e.login(t, "Brindille"), e.login(t, "Tiago"), e.login(t, "Zozo42"), e.login(t, "Kaelen")
	_, m := e.post(t, "/api/claims", brin, map[string]any{"masque": zone, "titre": "Bord de mer"})
	claim := itoa(int(m["id"].(float64)))

	// a pending claim: the requester accepts, the helper becomes a confirmed co-author
	if st, d := e.post(t, "/api/rejoindre", zozo, map[string]any{"cible": "claim:" + claim, "message": "J'ai fait le ciel"}); st != 200 {
		t.Fatalf("ask %d %v", st, d)
	}
	if st, _ := e.post(t, "/api/rejoindre", zozo, map[string]any{"cible": "claim:" + claim}); st != 400 {
		t.Fatalf("asked twice: %d", st)
	}
	if st, _ := e.post(t, "/api/rejoindre", brin, map[string]any{"cible": "claim:" + claim}); st != 400 {
		t.Fatalf("the author asked to join: %d", st)
	}
	todo := e.getJSON(t, "/api/rejoindre", brin)["a_decider"].([]any)
	if len(todo) != 1 || todo[0].(map[string]any)["message"] != "J'ai fait le ciel" {
		t.Fatalf("to decide %v", todo)
	}
	dz := itoa(int(todo[0].(map[string]any)["id"].(float64)))
	if st, _ := e.post(t, "/api/rejoindre/"+dz, kae, map[string]any{"accepte": true}); st != 403 {
		t.Fatalf("a stranger decided: %d", st)
	}
	if st, d := e.post(t, "/api/rejoindre/"+dz, brin, map[string]any{"accepte": true}); st != 200 {
		t.Fatalf("accept %d %v", st, d)
	}
	co := e.getJSON(t, "/api/claims/"+claim, zozo)["claim"].(map[string]any)["co_auteurs"].([]any)
	if len(co) != 1 || co[0].(map[string]any)["confirme"] != true {
		t.Fatalf("co-authors %v", co)
	}

	// validated: the claim becomes the artwork, the co-author decides too, a refusal is final
	if st, d := e.post(t, "/api/admin/claims/"+claim+"/decision", admin, map[string]any{"action": "valider"}); st != 200 {
		t.Fatalf("validate %d %v", st, d)
	}
	if st, d := e.post(t, "/api/rejoindre", kae, map[string]any{"cible": "claim:" + claim}); st != 200 {
		t.Fatalf("ask the artwork %d %v", st, d)
	}
	if st, _ := e.post(t, "/api/rejoindre", kae, map[string]any{"cible": "oeuvre:1"}); st != 400 {
		t.Fatalf("asked twice (claim, then its artwork): %d", st)
	}
	todo = e.getJSON(t, "/api/rejoindre", zozo)["a_decider"].([]any)
	if len(todo) != 1 || todo[0].(map[string]any)["cible"] != "oeuvre:1" {
		t.Fatalf("to decide on the artwork %v", todo)
	}
	dk := itoa(int(todo[0].(map[string]any)["id"].(float64)))
	if st, _ := e.post(t, "/api/rejoindre/"+dk, zozo, map[string]any{"accepte": false}); st != 200 {
		t.Fatalf("refuse %d", st)
	}
	if st, _ := e.post(t, "/api/rejoindre", kae, map[string]any{"cible": "oeuvre:1"}); st != 400 {
		t.Fatalf("asked again after a refusal: %d", st)
	}
	// an admin can accept anyone's request
	if st, d := e.post(t, "/api/rejoindre", e.login(t, "Mika"), map[string]any{"cible": "oeuvre:1"}); st != 200 {
		t.Fatalf("ask %d %v", st, d)
	}
	todo = e.getJSON(t, "/api/rejoindre", admin)["a_decider"].([]any)
	if st, _ := e.post(t, "/api/rejoindre/"+itoa(int(todo[0].(map[string]any)["id"].(float64))), admin, map[string]any{"accepte": true}); st != 200 {
		t.Fatalf("admin accept %d", st)
	}
	if n := len(e.getJSON(t, "/api/oeuvres/1", nil)["oeuvre"].(map[string]any)["auteurs"].([]any)); n != 3 {
		t.Fatalf("authors %d, want 3 (Brindille, Zozo42, Mika)", n)
	}
}
