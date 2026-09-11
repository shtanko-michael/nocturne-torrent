package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestPreviewUsesMetadataAndAvoidsExistingFolder(t *testing.T) {
	mi, _, _ := fixture(t, "v1")
	file := filepath.Join(t.TempDir(), "unrelated-name.torrent")
	f, e := os.Create(file)
	if e != nil {
		t.Fatal(e)
	}
	if e = mi.Write(f); e != nil {
		t.Fatal(e)
	}
	f.Close()
	base := t.TempDir()
	s := &TorrentService{settings: Settings{DownloadDir: base}}
	preview, e := s.Preview(file)
	if e != nil {
		t.Fatal(e)
	}
	if preview.Name != "dataset" || preview.Directory != filepath.Join(base, "dataset") {
		t.Fatalf("%+v", preview)
	}
	if e = os.Mkdir(preview.Directory, 0700); e != nil {
		t.Fatal(e)
	}
	preview, e = s.Preview(file)
	if e != nil {
		t.Fatal(e)
	}
	if filepath.Base(preview.Directory) != "dataset (2)" {
		t.Fatal(preview.Directory)
	}
}
func TestMagnetSuggestionAndSafeDirectoryClaim(t *testing.T) {
	name, e := sourceName("magnet:?xt=urn:btih:0123456789012345678901234567890123456789&dn=Ubuntu+Desktop")
	if e != nil || name != "Ubuntu Desktop" {
		t.Fatalf("%s %v", name, e)
	}
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "user.txt")
	os.WriteFile(sentinel, []byte("keep"), 0600)
	if claimDirectory(dir, "abc") == nil {
		t.Fatal("accepted nonempty directory")
	}
	raw, _ := os.ReadFile(sentinel)
	if string(raw) != "keep" {
		t.Fatal("user data changed")
	}
	empty := filepath.Join(t.TempDir(), "Ubuntu Desktop")
	if e = claimDirectory(empty, "abc"); e != nil {
		t.Fatal(e)
	}
	if e = claimDirectory(empty, "another"); e == nil {
		t.Fatal("directory claimed twice")
	}
}
func TestTraySummaryIncludesLiveState(t *testing.T) {
	text := traySummary(Snapshot{Torrents: []TorrentView{{Status: "downloading", Wanted: 100, WantedCompleted: 50, DownloadRate: 2 << 20}, {Status: "paused"}, {Status: "seeding", UploadRate: 1 << 20}}})
	for _, want := range []string{"50%", "Загрузка: 1", "пауза: 1", "раздача: 1", "2.0 МиБ/с", "1.0 МиБ/с"} {
		if !strings.Contains(text, want) {
			t.Fatalf("%s missing from %s", want, text)
		}
	}
	if len(utf16.Encode([]rune(text))) > 127 {
		t.Fatal("tooltip would be truncated")
	}
}
