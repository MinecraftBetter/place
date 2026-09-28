package place

// The wall of a player's museum can show more than its theme: the pixel landscapes,
// any capture of the canvas history (all of them, to the minute), an artwork, or an
// image of the player's own (checked by the team when image checking is on).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type MuseeFond struct {
	Type   string `json:"type"`   // theme | paysage | capture | oeuvre | image
	Value  string `json:"value"`  // paysage: desert | nuit · oeuvre: id · image: /media/fonds/… URL
	T      int64  `json:"t"`      // capture: unix time of the capture (stable even if the index grows)
	Frame  int    `json:"frame"`  // capture: index in the backups (resolved from T; sent by the editor)
	Flou   int    `json:"flou"`   // blur, 0–12 px
	Sombre int    `json:"sombre"` // darkening, 0–85 %
	Mode   string `json:"mode"`   // remplir | mosaique
	URL    string `json:"url,omitempty"`
	Label  string `json:"label,omitempty"` // « 11 septembre 2022 à 23 h 04 », the artwork's title…
}

var fondNameRE = regexp.MustCompile(`^\d+-[0-9a-f]{12}\.(png|jpg)$`)

var paysages = map[string]string{"desert": "Désert au couchant", "nuit": "Nuit étoilée"}

const (
	maxFondBytes = 8 << 20
	maxFondSide  = 6000
	fondMaxW     = 1600
	fondMaxH     = 1000
)

// frameAt finds the capture taken at unix time t (or the last one before).
func frameAt(idx *BackupIndex, t int64) int {
	k := sort.Search(len(idx.Frames), func(i int) bool { return idx.Frames[i].T > t }) - 1
	return max(k, 0)
}

// sanitizeFond keeps a background the player may use; nil means the theme's wall.
func (api *API) sanitizeFond(u *User, f *MuseeFond) *MuseeFond {
	if f == nil {
		return nil
	}
	out := &MuseeFond{Type: f.Type, Flou: max(0, min(f.Flou, 12)), Sombre: max(0, min(f.Sombre, 85)), Mode: f.Mode}
	if out.Mode != "mosaique" {
		out.Mode = "remplir"
	}
	switch f.Type {
	case "paysage":
		if _, ok := paysages[f.Value]; !ok {
			return nil
		}
		out.Value = f.Value
	case "capture":
		// the editor sends the index of the picked capture; its time is what is kept
		if idx := api.index(); idx != nil && f.Frame >= 0 && f.Frame < len(idx.Frames) {
			out.T = idx.Frames[f.Frame].T
		} else if f.T > 0 {
			out.T = f.T
		} else {
			return nil
		}
	case "oeuvre":
		id, _ := strconv.ParseInt(f.Value, 10, 64)
		if o, _ := api.store.Oeuvre(id); o == nil || o.Statut != "active" {
			return nil
		}
		out.Value = strconv.FormatInt(id, 10)
	case "image":
		if !strings.HasPrefix(f.Value, fmt.Sprintf("/media/fonds/%d-", u.ID)) || strings.ContainsAny(f.Value, "\"'()\\ ") {
			return nil
		}
		out.Value = f.Value
	default:
		return nil
	}
	return out
}

// resolveFond fills the URL (and the label) shown on the wall.
func (api *API) resolveFond(f *MuseeFond) {
	if f == nil {
		return
	}
	f.URL, f.Label = "", ""
	switch f.Type {
	case "paysage":
		f.URL, f.Label = "/img/"+f.Value+".png", paysages[f.Value]
	case "capture":
		if idx := api.index(); idx != nil && len(idx.Frames) > 0 {
			f.Frame = frameAt(idx, f.T)
			f.URL = fmt.Sprintf("/api/snapshots/%d.png", f.Frame)
			f.Label = frenchLaunchDate(time.Unix(idx.Frames[f.Frame].T, 0))
		}
	case "oeuvre":
		id, _ := strconv.ParseInt(f.Value, 10, 64)
		if o, _ := api.store.Oeuvre(id); o != nil && o.Statut == "active" {
			f.URL, f.Label = cropURL(o.X, o.Y, o.W, o.H, 480), o.Titre
		}
	case "image":
		f.URL = f.Value
	}
}

// fondCSS: the wall as a CSS background (the directory cards, the public museum page).
func (api *API) fondCSS(m *Musee) string {
	if m.Fond != nil {
		f := *m.Fond
		api.resolveFond(&f)
		if f.URL != "" {
			size := "center / cover no-repeat"
			if f.Mode == "mosaique" {
				size = "0 0 / auto repeat"
			}
			a := float64(f.Sombre) / 100
			return fmt.Sprintf(`linear-gradient(rgba(0,0,0,%.2f), rgba(0,0,0,%.2f)), url("%s") %s`, a, a, f.URL, size)
		}
	}
	wall := themeWalls[m.Theme]
	if m.Mur != "" {
		wall = "linear-gradient(180deg, " + m.Mur + ", " + m.Mur + "d0)"
	}
	return wall
}

// processFond: a PNG, JPEG or GIF of the player, re-encoded (no metadata), at most
// 1600 × 1000. Small pictures (pixel art) stay PNG and sharp, photos become JPEG.
func processFond(r io.Reader) ([]byte, string, error) {
	errFond := errors.New("image illisible : un PNG, JPEG ou GIF de 8 Mo au plus")
	data, err := io.ReadAll(io.LimitReader(r, maxFondBytes+1))
	if err != nil || len(data) > maxFondBytes {
		return nil, "", errFond
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 16 || cfg.Height < 16 || cfg.Width > maxFondSide || cfg.Height > maxFondSide {
		return nil, "", errors.New("image illisible ou trop grande (6000 pixels de côté au plus)")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", errFond
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	scale := min(1, min(float64(fondMaxW)/float64(w), float64(fondMaxH)/float64(h)))
	nw, nh := max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
	out := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		y0, y1 := y*h/nh, max((y+1)*h/nh, y*h/nh+1)
		for x := 0; x < nw; x++ {
			x0, x1 := x*w/nw, max((x+1)*w/nw, x*w/nw+1)
			var rs, gs, bs, n uint32
			for yy := y0; yy < y1; yy += max(1, (y1-y0)/4) { // box filter, sampled
				for xx := x0; xx < x1; xx += max(1, (x1-x0)/4) {
					c := color.NRGBAModel.Convert(img.At(b.Min.X+xx, b.Min.Y+yy)).(color.NRGBA)
					rs, gs, bs, n = rs+uint32(c.R), gs+uint32(c.G), bs+uint32(c.B), n+1
				}
			}
			out.SetNRGBA(x, y, color.NRGBA{uint8(rs / n), uint8(gs / n), uint8(bs / n), 255})
		}
	}
	if w <= 512 && h <= 512 {
		return encodePNG(out), "png", nil
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: 86}); err != nil {
		return nil, "", errFond
	}
	return buf.Bytes(), "jpg", nil
}

// POST /api/me/musee/fond (multipart « fichier ») — an image for the museum's wall.
func (api *API) handleMyFond(w http.ResponseWriter, r *http.Request) {
	u := api.auth.User(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "Connecte-toi.")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
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
	r.Body = http.MaxBytesReader(w, r.Body, maxFondBytes+64<<10)
	file, _, err := r.FormFile("fichier")
	if err != nil {
		writeFieldError(w, &FieldError{"fichier", "Image trop lourde ou illisible (8 Mo au plus)."})
		return
	}
	defer file.Close()
	data, ext, err := processFond(file)
	if err != nil {
		writeFieldError(w, &FieldError{"fichier", err.Error()})
		return
	}
	url, err := api.saveMediaAs("fonds", u.ID, data, ext)
	if err != nil {
		api.serverError(w, "Fond", err)
		return
	}
	if api.Settings().AvatarCheck {
		api.store.AddReport(Report{Type: "fond", Cible: fmt.Sprintf("musee:%d", u.ID), uid: u.ID, Source: "filtre", Raison: "Nouveau fond de musée à vérifier", Contenu: url})
	}
	writeJSON(w, map[string]any{"ok": true, "url": url})
}

func init() {
	ReportTargets["fond"] = func(api *API, rep *Report, id int64) bool {
		r, _ := api.store.Musee(uint32(id))
		if r == nil || r.m.Fond == nil {
			return false
		}
		f := *r.m.Fond
		api.resolveFond(&f)
		rep.uid, rep.Contenu = uint32(id), f.URL
		return true
	}
	HideHandlers["fond"] = func(api *API, rep *Report, rec moderationRecord) (moderationRecord, error) {
		uid := uint32(idOf(rep.Cible))
		r, _ := api.store.Musee(uid)
		if r == nil {
			return rec, fmt.Errorf("museum not found")
		}
		old, _ := json.Marshal(r.m.Fond)
		r.m.Fond = nil
		_, err := api.store.SaveMusee(uid, r.m, r.publie)
		rec.Field, rec.Value = "musee_fond", string(old)
		return rec, err
	}
	UnhideHandlers["musee_fond"] = func(api *API, rec moderationRecord, cible string) error {
		uid := uint32(idOf(cible))
		r, _ := api.store.Musee(uid)
		if r == nil {
			return nil
		}
		var f *MuseeFond
		json.Unmarshal([]byte(rec.Value), &f)
		r.m.Fond = f
		_, err := api.store.SaveMusee(uid, r.m, r.publie)
		return err
	}
}
