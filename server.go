package place

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"path"
	"strconv"
	"sync"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	Error: func(w http.ResponseWriter, req *http.Request, status int, err error) {
		log.Error(err)
		http.Error(w, "Error while trying to make websocket connection.", status)
	},
}

type PixelColor struct {
	X     int         `json:"x"`
	Y     int         `json:"y"`
	Color color.NRGBA `json:"color"`
}

type Server struct {
	sync.RWMutex
	msgs    chan PixelColor
	close   chan int
	clients []chan PixelColor
	img     draw.Image
	imgBuf  []byte
}

func NewServer(img draw.Image, count int) *Server {
	sv := &Server{
		RWMutex: sync.RWMutex{},
		msgs:    make(chan PixelColor),
		close:   make(chan int),
		clients: make([]chan PixelColor, count),
		img:     img,
	}
	go sv.broadcastLoop()
	return sv
}

func (sv *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch path.Base(req.URL.Path) {
	case "place.png":
		sv.HandleGetImage(w, req)
	case "stat":
		sv.HandleGetStat(w, req)
	case "ws":
		sv.HandleSocket(w, req)
	default:
		http.NotFound(w, req)
	}
}

func (sv *Server) HandleGetImage(w http.ResponseWriter, r *http.Request) {
	log.WithField("endpoint", "Image").WithField("ip", r.RemoteAddr).Trace("Image requested")
	b := sv.GetImageBytes() //not thread safe but it won't do anything bad
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	w.Header().Set("Cache-Control", "no-cache, no-store")
	_, err := w.Write(b)
	if err != nil {
		log.WithField("endpoint", "Image").WithField("ip", r.RemoteAddr).Error(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (sv *Server) HandleGetStat(w http.ResponseWriter, r *http.Request) {
	log.WithField("endpoint", "Stat").WithField("ip", r.RemoteAddr).Trace("Stats requested")
	count := 0
	for _, ch := range sv.clients {
		if ch != nil {
			count++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(map[string]interface{}{
		"connections": count,
	})
	if err != nil {
		log.WithField("endpoint", "Stat").WithField("ip", r.RemoteAddr).Error(err)
		http.Error(w, err.Error(), 500)
	}
}

func (sv *Server) HandleSocket(w http.ResponseWriter, r *http.Request) {
	log.WithField("endpoint", "Socket").WithField("ip", r.RemoteAddr).Trace("WebSocket requested")
	sv.Lock()
	defer sv.Unlock()
	i := sv.getConnIndex()
	if i == -1 {
		log.WithField("endpoint", "Socket").Warning("Server full")
		http.Error(w, "Server full", 509)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.WithField("endpoint", "Socket").Error(err)
		return
	}
	log.WithField("endpoint", "Socket").WithField("ip", r.RemoteAddr).Info("Connected")
	ch := make(chan PixelColor)
	sv.clients[i] = ch
	go sv.readLoop(conn, r, i)
	go sv.writeLoop(conn, r, ch)
}

func (sv *Server) getConnIndex() int {
	for i, client := range sv.clients {
		if client == nil {
			return i
		}
	}
	return -1
}

func (sv *Server) readLoop(conn *websocket.Conn, r *http.Request, i int) {
	for {
		var p PixelColor
		err := conn.ReadJSON(&p)

		if err != nil {
			var closeError *websocket.CloseError
			if _, ok := err.(*websocket.CloseError); ok {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.WithField("endpoint", "Socket").WithField("ip", r.RemoteAddr).Error("Unexpected close error, ", closeError)
				} else {
					log.WithField("endpoint", "Socket").WithField("ip", r.RemoteAddr).Info(closeError)
				}
			} else {
				log.WithField("endpoint", "Socket").WithField("ip", r.RemoteAddr).Error("Error decoding message, ", err)
			}
			break
		}

		err = sv.handleMessage(p)
		if err == nil {
			log.WithField("endpoint", "Socket").WithField("ip", r.RemoteAddr).Debug("Pixel (" + strconv.Itoa(p.X) + ", " + strconv.Itoa(p.Y) + ") changed to " + toHex(p.Color))
		} else {
			log.WithField("endpoint", "Socket").WithField("ip", r.RemoteAddr).Error("Client kicked for bad message", err)
			break
		}
	}
	sv.close <- i
	log.WithField("endpoint", "Socket").WithField("ip", r.RemoteAddr).Info("Disconnected")
}

func toHex(c color.NRGBA) string {
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}

func (sv *Server) writeLoop(conn *websocket.Conn, r *http.Request, ch chan PixelColor) {
	for {
		if p, ok := <-ch; ok {
			err := conn.WriteJSON(p)
			if err != nil {
				log.WithField("endpoint", "Socket").WithField("ip", r.RemoteAddr).Error(err)
				break
			}
		} else {
			break
		}
	}
	err := conn.Close()
	if err != nil {
		log.WithField("endpoint", "Socket").WithField("ip", r.RemoteAddr).Error(err)
	}
}

func (sv *Server) handleMessage(response PixelColor) error {
	if !sv.setPixel(response.X, response.Y, response.Color) {
		return errors.New("invalid placement")
	}
	sv.msgs <- response
	return nil
}

func (sv *Server) broadcastLoop() {
	for {
		select {
		case i := <-sv.close:
			if sv.clients[i] != nil {
				close(sv.clients[i])
				sv.clients[i] = nil
			}
		case p := <-sv.msgs:
			for i, ch := range sv.clients {
				if ch != nil {
					select {
					case ch <- p:
					default:
						close(ch)
						sv.clients[i] = nil
					}
				}
			}
		}
	}
}

func (sv *Server) GetImageBytes() []byte {
	if sv.imgBuf == nil {
		buf := bytes.NewBuffer(nil)
		if err := png.Encode(buf, sv.img); err != nil {
			log.Error(err)
		}
		sv.imgBuf = buf.Bytes()
	}
	return sv.imgBuf
}

func (sv *Server) setPixel(x, y int, c color.Color) bool {
	rect := sv.img.Bounds()
	width := rect.Max.X - rect.Min.X
	height := rect.Max.Y - rect.Min.Y
	if 0 > x || x >= width || 0 > y || y >= height {
		return false
	}
	sv.img.Set(x, y, c)
	sv.imgBuf = nil
	return true
}
