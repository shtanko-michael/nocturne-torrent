package main

import (
	"bytes"
	"fmt"
	"github.com/anacrolix/torrent/metainfo"
	"os"
	"path/filepath"
	"strings"
)

type TorrentPreview struct {
	Name      string
	Directory string
}

func sourceName(source string) (string, error) {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(source, "magnet:?") {
		m, e := metainfo.ParseMagnetV2Uri(source)
		if e != nil {
			return "", e
		}
		if m.DisplayName != "" {
			return m.DisplayName, nil
		}
		if m.InfoHash.Ok {
			return "Торрент-" + m.InfoHash.Value.HexString()[:12], nil
		}
		if m.V2InfoHash.Ok {
			return fmt.Sprintf("Торрент-%x", m.V2InfoHash.Value[:6]), nil
		}
		return "", fmt.Errorf("magnet не содержит infohash")
	}
	st, e := os.Stat(source)
	if e != nil {
		return "", e
	}
	if st.Size() > 16<<20 {
		return "", fmt.Errorf("torrent-файл больше 16 МиБ")
	}
	raw, e := os.ReadFile(source)
	if e != nil {
		return "", e
	}
	mi, e := metainfo.Load(bytes.NewReader(raw))
	if e != nil {
		return "", e
	}
	info, e := mi.UnmarshalInfo()
	if e != nil {
		return "", e
	}
	if e = validateInfo(&info); e != nil {
		return "", e
	}
	return info.BestName(), nil
}
func folderName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune("/\\:<>\"|?*", r) {
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(name, ". ")
	chars := []rune(name)
	if len(chars) > 120 {
		name = string(chars[:120])
	}
	if safeComponent(name) != nil {
		name = "Торрент"
	}
	return name
}
func unusedDirectory(base, name string) string {
	p := filepath.Join(base, folderName(name))
	for i := 2; ; i++ {
		if _, e := os.Lstat(p); e != nil {
			return p
		}
		p = filepath.Join(base, fmt.Sprintf("%s (%d)", folderName(name), i))
	}
}
func (s *TorrentService) Preview(source string) (TorrentPreview, error) {
	name, e := sourceName(source)
	if e != nil {
		return TorrentPreview{}, e
	}
	s.mu.Lock()
	base := s.settings.DownloadDir
	s.mu.Unlock()
	return TorrentPreview{Name: name, Directory: unusedDirectory(base, name)}, nil
}
func claimDirectory(dir, id string) error {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	st, e := os.Lstat(dir)
	if e != nil {
		return e
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("выберите обычную папку")
	}
	entries, e := os.ReadDir(dir)
	if e != nil {
		return e
	}
	if len(entries) != 0 {
		return fmt.Errorf("выберите новую или пустую папку: %s", dir)
	}
	f, e := os.OpenFile(filepath.Join(dir, ".nocturne-owner"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.WriteString(id)
	ce := f.Close()
	if e != nil {
		return e
	}
	return ce
}
