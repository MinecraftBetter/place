package main

import (
	"crypto/tls"
	"errors"
	"flag"
	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/mattn/go-colorable"
	log "github.com/sirupsen/logrus"
	"image"
	"image/draw"
	"image/png"
	"net/http"
	"os"
	"time"

	"github.com/minecraftbetter/place"
	"github.com/rbxb/httpfilter"
)

var port string
var root string
var loadPath string
var savePath string
var width int
var height int
var count int
var saveInterval int

func init() {
	flag.StringVar(&port, "port", ":8080", "The address and port the fileserver listens at.")
	flag.StringVar(&root, "root", "./web/root", "The directory serving files.")
	flag.StringVar(&loadPath, "load", "", "The png to load as the canvas.")
	flag.StringVar(&savePath, "save", "./place.png", "The path to save the canvas.")
	flag.IntVar(&width, "width", 1024, "The width to create the canvas.")
	flag.IntVar(&height, "height", 1024, "The height to create the canvas.")
	flag.IntVar(&count, "count", 64, "The maximum number of connections.")
	flag.IntVar(&saveInterval, "saveInterval", 180, "Save interval in seconds.")
}

func main() {
	flag.Parse()

	// Logging
	log.SetFormatter(&nested.Formatter{
		HideKeys: true,
	})
	log.SetLevel(log.DebugLevel)
	log.SetOutput(colorable.NewColorableStdout())

	// Load image
	var img draw.Image = nil
	if loadPath != "" {
		img = loadImage(loadPath)
	}
	if img == nil {
		nrgba := image.NewNRGBA(image.Rect(0, 0, width, height))
		for i := range nrgba.Pix {
			nrgba.Pix[i] = 255
		}
		img = nrgba
	}

	// Start the place server
	placeSv := place.NewServer(img, count)
	defer os.WriteFile(savePath, placeSv.GetImageBytes(), 0644)
	go func() {
		for {
			os.WriteFile(savePath, placeSv.GetImageBytes(), 0644)
			time.Sleep(time.Second * time.Duration(saveInterval))
		}
	}()
	fs := httpfilter.NewServer(root, "", map[string]httpfilter.OpFunc{
		"place": func(w http.ResponseWriter, req *http.Request, args ...string) {
			placeSv.ServeHTTP(w, req)
		},
	})
	server := http.Server{
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)), //disable HTTP/2
		Addr:         port,
		Handler:      fs,
	}
	log.Fatal(server.ListenAndServe())
}

// Loads an image
func loadImage(loadPath string) draw.Image {
	f, err := os.Open(loadPath)
	defer f.Close()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Warning(err)
			return nil
		} else {
			panic(err)
		}
	}

	pngImg, err := png.Decode(f)
	if err != nil {
		panic(err)
	}

	// We copy the PNG image into a Bitmap image, which allows us to remove the palette that causes colour problems
	b := pngImg.Bounds()
	m := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(m, m.Bounds(), pngImg, b.Min, draw.Src)
	return m
}
