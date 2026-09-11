package main

import (
	"fmt"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"sync"
	"time"
)

func (s *TorrentService) setAutostart(enabled bool) error {
	if enabled {
		return s.app.Autostart.EnableWithOptions(application.AutostartOptions{Arguments: []string{"--background"}})
	}
	return s.app.Autostart.Disable()
}
func traySummary(snapshot Snapshot) string {
	active, paused, queued, seeding, errors := 0, 0, 0, 0, 0
	var down, up float64
	var done, total int64
	for _, t := range snapshot.Torrents {
		down += t.DownloadRate
		up += t.UploadRate
		done += t.WantedCompleted
		total += t.Wanted
		switch t.Status {
		case "downloading", "metadata", "checking":
			active++
		case "paused":
			paused++
		case "queued":
			queued++
		case "seeding":
			seeding++
		case "error":
			errors++
		}
	}
	progress := 0.
	if total > 0 {
		progress = 100 * float64(done) / float64(total)
	}
	return fmt.Sprintf("Nocturne · %.0f%%\nЗагрузка: %d · раздача: %d · пауза: %d\nОчередь: %d · ошибки: %d\n↓ %.1f МиБ/с · ↑ %.1f МиБ/с", progress, active, seeding, paused, queued, errors, down/(1<<20), up/(1<<20))
}
func (s *TorrentService) setupDesktop(window *application.WebviewWindow, icon []byte) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	var once sync.Once
	stopUpdates := func() { once.Do(func() { close(stop) }); <-done }
	if enabled, e := s.app.Autostart.IsEnabled(); e == nil {
		s.mu.Lock()
		s.settings.Autostart = enabled
		s.mu.Unlock()
	}
	tray := s.app.SystemTray.New()
	tray.SetIcon(icon)
	show := func() { window.Show(); window.Focus() }
	tray.OnClick(show)
	menu := s.app.NewMenu()
	menu.Add("Открыть Nocturne").OnClick(func(*application.Context) { show() })
	menu.AddSeparator()
	menu.Add("Выход").OnClick(func(*application.Context) { go func() { stopUpdates(); s.app.Quit() }() })
	tray.SetMenu(menu)
	tray.SetTooltip(traySummary(s.Snapshot()))
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) { e.Cancel(); window.Hide() })
	go func() {
		defer close(done)
		timer := time.NewTicker(2 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-stop:
				return
			case <-timer.C:
				tray.SetTooltip(traySummary(s.Snapshot()))
			}
		}
	}()
	return stopUpdates
}
