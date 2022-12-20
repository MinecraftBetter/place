package place

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"net/http"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  64,
	WriteBufferSize: 64,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	Error: func(w http.ResponseWriter, req *http.Request, status int, err error) {
		log.Println(err)
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
		http.Error(w, "Not found.", 404)
	}
}

func (sv *Server) HandleGetImage(w http.ResponseWriter, req *http.Request) {
	b := sv.GetImageBytes() //not thread safe but it won't do anything bad
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Write(b)
}

func (sv *Server) HandleGetStat(w http.ResponseWriter, req *http.Request) {
	count := 0
	for _, ch := range sv.clients {
		if ch != nil {
			count++
		}
	}
	fmt.Fprint(w, count)
}

func (sv *Server) HandleSocket(w http.ResponseWriter, req *http.Request) {
	sv.Lock()
	defer sv.Unlock()
	i := sv.getConnIndex()
	if i == -1 {
		log.Println("Server full.")
		http.Error(w, "Server full.", 503)
		return
	}
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Println(err)
		return
	}
	ch := make(chan PixelColor)
	sv.clients[i] = ch
	go sv.readLoop(conn, i)
	go sv.writeLoop(conn, ch)
}

func (sv *Server) getConnIndex() int {
	for i, client := range sv.clients {
		if client == nil {
			return i
		}
	}
	return -1
}

func rateLimiter() func() bool {
	const rate = 80  // per second average
	const min = 0.01 // kick threshold

	// Minimum time difference between messages
	// Network sometimes delivers two messages in quick succession
	const minDif = int64(time.Millisecond * 50)

	last := time.Now().UnixNano()
	var v float32 = 1.0
	return func() bool {
		now := time.Now().UnixNano()
		dif := now - last
		if dif < minDif {
			dif = minDif
		}
		v *= float32(rate*dif) / float32(time.Second)
		if v > 1.0 {
			v = 1.0
		}
		last = now
		return v > min
	}
}

func (sv *Server) readLoop(conn *websocket.Conn, i int) {
	limiter := rateLimiter()
	for {
		var p PixelColor
		err := conn.ReadJSON(&p)
		if err != nil {
			break
		}
		if !limiter() {
			log.Println("Client kicked for high rate.")
			break
		}
		if sv.handleMessage(p) != nil {
			log.Println("Client kicked for bad message.")
			break
		}
	}
	sv.close <- i
}

func (sv *Server) writeLoop(conn *websocket.Conn, ch chan PixelColor) {
	for {
		if p, ok := <-ch; ok {
			conn.WriteJSON(p)
		} else {
			break
		}
	}
	conn.Close()
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
			log.Println(err)
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
