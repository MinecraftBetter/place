package place

// The zones already taken, for /revendiquer: the artworks and the claims waiting for a
// decision. /api/zones lists them; /api/zones.png is a map of the canvas where each
// pixel of a zone holds its number in the list (R = high byte, G = low byte, starting
// at 1), transparent elsewhere. The page draws the outlines and the titles from both.

import (
	"image"
	"net/http"
	"sort"
	"strconv"
)

type claimedZone struct {
	Type  string        `json:"type"` // "oeuvre" or "attente"
	ID    int64         `json:"id"`
	Titre string        `json:"titre"`
	X     int           `json:"x"`
	Y     int           `json:"y"`
	W     int           `json:"w"`
	H     int           `json:"h"`
	Par   []*PublicUser `json:"par"`
	pos   []int32
}

func (api *API) claimedZones() ([]*claimedZone, error) {
	cw, ch := api.canvas.Size()
	var out []*claimedZone
	os, err := api.store.Oeuvres(0, false)
	if err != nil {
		return nil, err
	}
	for _, o := range os {
		z := &claimedZone{Type: "oeuvre", ID: o.ID, Titre: o.Titre, X: o.X, Y: o.Y, W: o.W, H: o.H, Par: []*PublicUser{}, pos: maskPositions(o.Mask, cw, ch)}
		for _, a := range o.Auteurs {
			if a.User != nil {
				z.Par = append(z.Par, a.User)
			}
		}
		out = append(out, z)
	}
	cs, err := api.store.Claims("attente", 0, 0)
	if err != nil {
		return nil, err
	}
	for _, c := range cs {
		z := &claimedZone{Type: "attente", ID: c.ID, Titre: c.Titre, X: c.X, Y: c.Y, W: c.W, H: c.H, Par: []*PublicUser{}, pos: maskPositions(c.Mask, cw, ch)}
		if c.Requester != nil {
			z.Par = append(z.Par, c.Requester)
		}
		for _, a := range c.Coauthors {
			if a.User != nil {
				z.Par = append(z.Par, a.User)
			}
		}
		out = append(out, z)
	}
	// the big ones first: painted first, a small zone inside a big one stays visible
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].pos) > len(out[j].pos) })
	return out, nil
}

// GET /api/zones
func (api *API) handleZones(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	zs, err := api.claimedZones()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Zones indisponibles.")
		return
	}
	if zs == nil {
		zs = []*claimedZone{}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"zones": zs})
}

// GET /api/zones.png
func (api *API) handleZonesPNG(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	zs, err := api.claimedZones()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Zones indisponibles.")
		return
	}
	cw, ch := api.canvas.Size()
	img := image.NewNRGBA(image.Rect(0, 0, cw, ch))
	for i, z := range zs {
		n := i + 1
		for _, p := range z.pos {
			if int(p) >= cw*ch {
				continue
			}
			o := int(p) * 4
			img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = uint8(n>>8), uint8(n), 0, 255
		}
	}
	b := encodePNG(img)
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	w.Header().Set("Cache-Control", "no-store")
	w.Write(b)
}
