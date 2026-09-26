package place

import (
	"encoding/json"
	"fmt"
	"image/color"
	"net/http"
	"path"
	"strconv"

	log "github.com/sirupsen/logrus"
)

// Server answers the historical endpoints routed by _filters.txt:
// /place.png, /stat and /ws.
type Server struct {
	canvas *Canvas
	hub    *Hub
}

func NewServer(c *Canvas, h *Hub) *Server {
	return &Server{canvas: c, hub: h}
}

func (sv *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch path.Base(req.URL.Path) {
	case "place.png":
		sv.HandleGetImage(w, req)
	case "stat":
		sv.HandleGetStat(w, req)
	case "ws":
		sv.hub.ServeWS(w, req)
	default:
		http.NotFound(w, req)
	}
}

func (sv *Server) HandleGetImage(w http.ResponseWriter, r *http.Request) {
	log.WithField("ip", r.RemoteAddr).WithField("endpoint", "Image").Trace("Image requested")
	b := sv.canvas.ImagePNG()
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	w.Header().Set("Cache-Control", "no-cache, no-store")
	if _, err := w.Write(b); err != nil {
		log.WithField("ip", r.RemoteAddr).WithField("endpoint", "Image").Error(err)
	}
}

func (sv *Server) HandleGetStat(w http.ResponseWriter, r *http.Request) {
	log.WithField("ip", r.RemoteAddr).WithField("endpoint", "Stat").Trace("Stats requested")
	count, total := sv.hub.Online()
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(map[string]interface{}{
		"connections": count,
		"slots":       total,
	})
	if err != nil {
		log.WithField("ip", r.RemoteAddr).WithField("endpoint", "Stat").Error(err)
	}
}

func toHex(c color.NRGBA) string {
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}
