package place

// The showcase of a profile (« Ma vitrine »): one of the player's artworks, or a small
// drawing made in /moi/profil. users.vitrine is "", "dessin" or "oeuvre:<id>".

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

const (
	maxDessinSide  = 64
	maxDessinBytes = 64 << 10
)

func init() {
	ProfileExtras = append(ProfileExtras, func(api *API, u *User, viewer *User) (string, any) {
		var vitrine, dessin string
		api.store.db.QueryRow(`SELECT vitrine, dessin_url FROM users WHERE id = ?`, u.ID).Scan(&vitrine, &dessin)
		res := map[string]any{}
		if viewer != nil && viewer.ID == u.ID {
			res["choix"], res["dessin_url"] = vitrine, dessin // the editor starts from these
		}
		switch {
		case vitrine == "dessin" && dessin != "":
			res["type"], res["url"] = "dessin", dessin
		case strings.HasPrefix(vitrine, "oeuvre:"):
			id, _ := strconv.ParseInt(strings.TrimPrefix(vitrine, "oeuvre:"), 10, 64)
			if o, _ := api.store.Oeuvre(id); o != nil && o.Statut == "active" {
				res["type"], res["id"], res["titre"], res["w"], res["h"] = "oeuvre", o.ID, o.Titre, o.W, o.H
				res["url"], res["href"] = fmt.Sprintf("/api/oeuvres/%d/image.png", o.ID), fmt.Sprintf("/oeuvre/%d", o.ID)
			}
		}
		if len(res) == 0 {
			return "", nil
		}
		return "vitrine", res
	})

	ReportTargets["dessin"] = func(api *API, rep *Report, id int64) bool {
		var url string
		if api.store.db.QueryRow(`SELECT dessin_url FROM users WHERE id = ?`, id).Scan(&url) != nil || url == "" {
			return false
		}
		// reported from the profile (user:<id>): kept apart so that hiding it reaches HideHandlers
		rep.uid, rep.Contenu, rep.Cible = uint32(id), url, fmt.Sprintf("dessin:%d", id)
		return true
	}
	HideHandlers["dessin"] = func(api *API, rep *Report, rec moderationRecord) (moderationRecord, error) {
		id := idOf(rep.Cible)
		var url string
		api.store.db.QueryRow(`SELECT dessin_url FROM users WHERE id = ?`, id).Scan(&url)
		_, err := api.store.db.Exec(`UPDATE users SET dessin_url = '', vitrine = CASE vitrine WHEN 'dessin' THEN '' ELSE vitrine END WHERE id = ?`, id)
		rec.Field, rec.Value = "dessin_url", url
		return rec, err
	}
	UnhideHandlers["dessin_url"] = func(api *API, rec moderationRecord, cible string) error {
		_, err := api.store.db.Exec(`UPDATE users SET dessin_url = ?, vitrine = CASE WHEN vitrine = '' AND ? != '' THEN 'dessin' ELSE vitrine END WHERE id = ?`, rec.Value, rec.Value, idOf(cible))
		return err
	}
}

// checkVitrine validates a showcase choice for u.
func (s *Store) checkVitrine(u *User, v string) error {
	switch {
	case v == "":
		return nil
	case v == "dessin":
		var url string
		s.db.QueryRow(`SELECT dessin_url FROM users WHERE id = ?`, u.ID).Scan(&url)
		if url == "" {
			return &FieldError{"vitrine", "Dessine d'abord ton dessin."}
		}
		return nil
	case strings.HasPrefix(v, "oeuvre:"):
		id, err := strconv.ParseInt(strings.TrimPrefix(v, "oeuvre:"), 10, 64)
		var n int
		if err == nil {
			s.db.QueryRow(`SELECT COUNT(*) FROM oeuvre_auteurs a JOIN oeuvres o ON o.id = a.oeuvre_id
				WHERE a.oeuvre_id = ? AND a.user_id = ? AND o.statut = 'active'`, id, u.ID).Scan(&n)
		}
		if n == 0 {
			return &FieldError{"vitrine", "Tu ne peux exposer que tes œuvres."}
		}
		return nil
	}
	return &FieldError{"vitrine", "Choix inconnu."}
}

// processDessin checks a drawing: a PNG of 8 to 64 pixels per side, re-encoded opaque.
func processDessin(r io.Reader) ([]byte, error) {
	errDessin := errors.New("dessin illisible : un PNG de 64 × 64 pixels au plus")
	img, err := png.Decode(io.LimitReader(r, maxDessinBytes+1))
	if err != nil {
		return nil, errDessin
	}
	b := img.Bounds()
	if b.Dx() < 8 || b.Dy() < 8 || b.Dx() > maxDessinSide || b.Dy() > maxDessinSide {
		return nil, errDessin
	}
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			c.A = 255
			out.SetNRGBA(x, y, c)
		}
	}
	return encodePNG(out), nil
}

// POST /api/me/dessin (multipart field "dessin"): saves the drawing and shows it in the showcase.
func (api *API) handleMeDessin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
		return
	}
	u := api.auth.User(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "Connecte-toi pour dessiner.")
		return
	}
	if api.mediaDir == "" {
		writeError(w, http.StatusServiceUnavailable, "Les imports sont désactivés sur ce serveur.")
		return
	}
	if u.WriteBlock(api.store.now()) != "" {
		writeError(w, http.StatusForbidden, "Ton compte ne peut pas importer d'image pour l'instant.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxDessinBytes+16<<10)
	file, _, err := r.FormFile("dessin")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Dessin manquant.")
		return
	}
	defer file.Close()
	data, err := processDessin(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	url, err := api.saveMedia("dessins", u.ID, data)
	if err != nil {
		log.WithField("endpoint", "API").Error("Dessin: ", err)
		writeError(w, http.StatusInternalServerError, "Enregistrement impossible.")
		return
	}
	if _, err := api.store.db.Exec(`UPDATE users SET dessin_url = ?, vitrine = 'dessin' WHERE id = ?`, url, u.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "Enregistrement impossible.")
		return
	}
	if api.Settings().AvatarCheck {
		api.store.AddReport(Report{Type: "dessin", Cible: fmt.Sprintf("dessin:%d", u.ID), uid: u.ID, Source: "filtre", Raison: "Nouveau dessin de vitrine à vérifier", Contenu: url})
	}
	writeJSON(w, map[string]string{"dessin_url": url})
}
