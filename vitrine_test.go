package place

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestVitrineChoices(t *testing.T) {
	st := newTestStore(t)
	a, b := mustUser(t, st, "Brindille"), mustUser(t, st, "Zozo42")
	st.db.Exec(`INSERT INTO oeuvres (id, titre, masque, x, y, w, h, pixels, origine, creee_le) VALUES (7, 'Le chat', '', 0, 0, 4, 4, 16, 'test', 1)`)
	st.db.Exec(`INSERT INTO oeuvre_auteurs (oeuvre_id, user_id, role) VALUES (7, ?, 'auteur')`, a.ID)
	for _, c := range []struct {
		u  *User
		v  string
		ok bool
	}{{a, "", true}, {a, "oeuvre:7", true}, {b, "oeuvre:7", false}, {a, "oeuvre:x", false}, {a, "dessin", false}, {a, "autre", false}} {
		if err := st.checkVitrine(c.u, c.v); (err == nil) != c.ok {
			t.Errorf("%s %q: err = %v", c.u.Pseudo, c.v, err)
		}
	}
	st.db.Exec(`UPDATE users SET dessin_url = '/media/dessins/x.png' WHERE id = ?`, a.ID)
	if err := st.checkVitrine(a, "dessin"); err != nil {
		t.Errorf("dessin once drawn: %v", err)
	}
}

func TestProcessDessinBounds(t *testing.T) {
	enc := func(w, h int) *bytes.Reader {
		var buf bytes.Buffer
		png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, w, h)))
		return bytes.NewReader(buf.Bytes())
	}
	if _, err := processDessin(enc(32, 32)); err != nil {
		t.Fatal(err)
	}
	for _, s := range [][2]int{{4, 32}, {65, 32}, {32, 100}} {
		if _, err := processDessin(enc(s[0], s[1])); err == nil {
			t.Errorf("%v accepted", s)
		}
	}
}
