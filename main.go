package main

import (
	"embed"
	"github.com/wailsapp/wails/v3/pkg/application"
	"log"
	"os"
	"slices"
	"sync/atomic"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var trayIcon []byte

func main() {
	var activeWindow atomic.Pointer[application.WebviewWindow]
	app := application.New(application.Options{
		Name: "Nocturne", Description: "BitTorrent client",
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		SingleInstance: &application.SingleInstanceOptions{UniqueID: "org.nocturne.torrent", OnSecondInstanceLaunch: func(application.SecondInstanceData) {
			if w := activeWindow.Load(); w != nil {
				w.Show()
				w.Focus()
			}
		}},
	})
	service, err := newService("")
	if err != nil {
		log.Fatal(err)
	}
	defer service.close()
	service.app = app
	app.RegisterService(application.NewService(service))
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Hidden: slices.Contains(os.Args, "--background"),
		Title:  "Nocturne — Torrent client", Width: 1713, Height: 969, MinWidth: 900, MinHeight: 620,
		BackgroundColour: application.NewRGB(15, 17, 21), URL: "/",
	})
	activeWindow.Store(window)
	stopDesktop := service.setupDesktop(window, trayIcon)
	defer stopDesktop()
	if err := app.Run(); err != nil {
		log.Print(err)
	}
}
