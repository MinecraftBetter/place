package place

import (
	"bytes"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func (e *testEnv) get(t *testing.T, path string, ck *http.Cookie) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest("GET", e.srv.URL+path, nil)
	if ck != nil {
		req.AddCookie(ck)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

func (e *testEnv) getJSON(t *testing.T, path string, ck *http.Cookie) map[string]any {
	t.Helper()
	status, b := e.get(t, path, ck)
	if status != 200 {
		t.Fatalf("GET %s: %d %s", path, status, b)
	}
	m := map[string]any{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return m
}

func TestMeGuestAndPlayer(t *testing.T) {
	e := newEnv(t, 8)
	m := e.getJSON(t, "/api/me", nil)
	if m["user"] != nil || m["cooldown"] != 5.0 || m["width"] != 16.0 || m["height"] != 16.0 {
		t.Fatalf("guest %v", m)
	}
	if auth := m["auth"].([]any); len(auth) != 1 || auth[0] != "dev" {
		t.Fatalf("auth %v", m["auth"])
	}

	ck := e.login(t, "Brindille")
	conn := e.dial(t, ck, "")
	sendPixel(t, conn, 0, 0)
	next(t, conn, isPixel)
	m = e.getJSON(t, "/api/me", ck)
	u := m["user"].(map[string]any)
	if u["pseudo"] != "Brindille" || u["slug"] != "brindille" || u["role"] != "joueur" || m["ready_in"] != 5.0 {
		t.Fatalf("player %v", m)
	}
}

func TestPixelInspector(t *testing.T) {
	e := newEnv(t, 8)
	m := e.getJSON(t, "/api/pixel?x=2&y=3", nil)
	if m["origin"] != true || m["owner"] != nil || m["color"] != "#FFFFFF" || len(m["history"].([]any)) != 0 || m["placed_at"] != nil {
		t.Fatalf("untouched pixel %v", m)
	}

	a := e.dial(t, e.login(t, "Brindille"), "")
	b := e.dial(t, e.login(t, "Zozo42"), "")
	sendPixel(t, a, 2, 3)
	next(t, a, isPixel)
	e.clock.Add(time.Second)
	b.WriteMessage(1, []byte(`{"x":2,"y":3,"color":{"R":0,"G":0,"B":0,"A":255}}`))
	next(t, b, func(m map[string]any) bool { return isPixel(m) && m["u"] == 2.0 })

	m = e.getJSON(t, "/api/pixel?x=2&y=3", nil)
	owner := m["owner"].(map[string]any)
	hist := m["history"].([]any)
	if owner["pseudo"] != "Zozo42" || m["origin"] != false || m["color"] != "#000000" || m["placed_at"] == nil {
		t.Fatalf("retouched pixel %v", m)
	}
	if len(hist) != 3 {
		t.Fatalf("history %v", hist)
	}
	h0, h1, h2 := hist[0].(map[string]any), hist[1].(map[string]any), hist[2].(map[string]any)
	if h0["user"].(map[string]any)["pseudo"] != "Zozo42" || h1["user"].(map[string]any)["pseudo"] != "Brindille" || h1["color"] != "#5EB3FF" {
		t.Fatalf("history order %v", hist)
	}
	if h2["origin"] != true || h2["color"] != "#FFFFFF" || h2["user"] != nil {
		t.Fatalf("origin entry %v", h2)
	}

	if status, _ := e.get(t, "/api/pixel?x=a&y=1", nil); status != 400 {
		t.Errorf("bad params: %d", status)
	}
	if status, _ := e.get(t, "/api/pixel?x=16&y=1", nil); status != 404 {
		t.Errorf("out of canvas: %d", status)
	}
}

func TestOwnersEndpoint(t *testing.T) {
	e := newEnv(t, 8)
	conn := e.dial(t, e.login(t, "Kaelen"), "")
	sendPixel(t, conn, 7, 9)
	next(t, conn, isPixel)
	status, b := e.get(t, "/api/owners", nil)
	if status != 200 {
		t.Fatalf("status %d", status)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if r, g, bl, _ := img.At(7, 9).RGBA(); (r>>8)<<16|(g>>8)<<8|bl>>8 != 1 {
		t.Fatal("owner id not encoded")
	}
}

func TestUsersEndpoint(t *testing.T) {
	e := newEnv(t, 8)
	e.login(t, "Evan")
	e.login(t, "Tiago")
	m := e.getJSON(t, "/api/users?ids=1,2,99", nil)
	users := m["users"].(map[string]any)
	if len(users) != 2 || users["1"].(map[string]any)["pseudo"] != "Evan" || users["2"].(map[string]any)["avatar"] != "/img/avatars/tiago.png" {
		t.Fatalf("users %v", users)
	}
	if status, _ := e.get(t, "/api/users?ids=1,x", nil); status != 400 {
		t.Errorf("invalid id: %d", status)
	}
	many := strings.TrimSuffix(strings.Repeat("1,", 101), ",")
	if status, _ := e.get(t, "/api/users?ids="+many, nil); status != 400 {
		t.Errorf("too many ids: %d", status)
	}
	if m := e.getJSON(t, "/api/users?ids=", nil); len(m["users"].(map[string]any)) != 0 {
		t.Errorf("empty ids: %v", m)
	}
}

func TestAPIRejectsWrites(t *testing.T) {
	e := newEnv(t, 8)
	res, err := http.Post(e.srv.URL+"/api/me", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status %d", res.StatusCode)
	}
}
