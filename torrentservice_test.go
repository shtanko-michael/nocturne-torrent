package main

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func isolated(c *torrent.ClientConfig) {
	c.NoDHT = true
	c.DisableTrackers = true
	c.NoDefaultPortForwarding = true
	c.DisablePEX = true
	c.DisableWebtorrent = true
	c.DisableIPv6 = true
	c.ListenHost = func(string) string { return "127.0.0.1" }
}
func eventually(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("condition did not become true within 20 seconds")
}
func fixture(t *testing.T, kind string) (*metainfo.MetaInfo, []byte, string) {
	t.Helper()
	data := bytes.Repeat([]byte("Nocturne test!  "), 1024)
	dir := t.TempDir()
	root := filepath.Join(dir, "dataset")
	if e := os.MkdirAll(root, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(root, "payload.bin"), data, 0600); e != nil {
		t.Fatal(e)
	}
	info := metainfo.Info{Name: "dataset", PieceLength: 16384}
	if kind != "v2" {
		hash := sha1.Sum(data)
		info.Pieces = hash[:]
		info.Files = []metainfo.FileInfo{{Path: []string{"payload.bin"}, Length: int64(len(data))}}
	}
	if kind != "v1" {
		hash := sha256.Sum256(data)
		info.MetaVersion = 2
		info.FileTree = metainfo.FileTree{Dir: map[string]metainfo.FileTree{"payload.bin": {File: metainfo.FileTreeFile{Length: int64(len(data)), PiecesRoot: string(hash[:])}}}}
	}
	raw, e := bencode.Marshal(&info)
	if e != nil {
		t.Fatal(e)
	}
	return &metainfo.MetaInfo{InfoBytes: raw}, data, dir
}
func TestLocalTransferAndRestore(t *testing.T) {
	for _, kind := range []string{"v1", "v2", "hybrid"} {
		for _, transport := range []string{"tcp", "utp"} {
			t.Run(kind+"/"+transport, func(t *testing.T) {
				mi, data, seedDir := fixture(t, kind)
				config := torrent.NewDefaultClientConfig()
				isolated(config)
				config.ListenPort = 42100
				config.DataDir = seedDir
				config.Seed = true
				config.DefaultStorage = &rootStorage{dir: seedDir}
				if transport == "tcp" {
					config.DisableUTP = true
				} else {
					config.DisableTCP = true
				}
				seed, e := torrent.NewClient(config)
				if e != nil {
					t.Fatal(e)
				}
				defer seed.Close()
				seedTorrent, e := seed.AddTorrent(mi)
				if e != nil {
					t.Fatal(e)
				}
				if e = seedTorrent.VerifyData(); e != nil {
					t.Fatal(e)
				}
				state := t.TempDir()
				configure := func(c *torrent.ClientConfig) {
					isolated(c)
					if transport == "tcp" {
						c.DisableUTP = true
					} else {
						c.DisableTCP = true
					}
				}
				service, e := newService(state, configure)
				if e != nil {
					t.Fatal(e)
				}
				defer func() {
					if service != nil {
						service.close()
					}
				}()
				source := ""
				if kind == "v1" {
					file := filepath.Join(t.TempDir(), "test.torrent")
					f, e := os.Create(file)
					if e != nil {
						t.Fatal(e)
					}
					if e = mi.Write(f); e != nil {
						t.Fatal(e)
					}
					f.Close()
					source = file
				} else {
					m, e := mi.MagnetV2()
					if e != nil {
						t.Fatal(e)
					}
					source = m.String()
				}
				id, e := service.Add(source, t.TempDir(), true)
				if e != nil {
					t.Fatal(e)
				}
				service.mu.Lock()
				l, _ := service.find(id)
				service.mu.Unlock()
				l.t.AddClientPeer(seed)
				eventually(t, func() bool { return len(service.Snapshot().Torrents[0].Files) > 0 })
				if service.Snapshot().Torrents[0].Completed != 0 {
					t.Fatal("paused job downloaded payload")
				}
				if e = service.Pause(id, false); e != nil {
					t.Fatal(e)
				}
				eventually(t, func() bool { return service.Snapshot().Torrents[0].Completed == int64(len(data)) })
				view := service.Snapshot().Torrents[0]
				if view.Version != kind {
					t.Fatalf("version %s != %s", view.Version, kind)
				}
				actual, e := os.ReadFile(filepath.Join(view.Dir, "payload.bin"))
				if e != nil {
					t.Fatal(e)
				}
				if !bytes.Equal(actual, data) {
					t.Fatal("payload differs")
				}
				if e = service.SetPriority(id, 0, 0); e != nil {
					t.Fatal(e)
				}
				if e = service.Pause(id, true); e != nil {
					t.Fatal(e)
				}
				service.close()
				service = nil
				service, e = newService(state, configure)
				if e != nil {
					t.Fatal(e)
				}
				eventually(t, func() bool { return len(service.Snapshot().Torrents[0].Files) > 0 })
				restored := service.Snapshot().Torrents[0]
				if restored.Status != "paused" || restored.Files[0].Priority != 0 {
					t.Fatal("pause or file priority lost")
				}
				if e = service.Remove(id, false); e != nil {
					t.Fatal(e)
				}
				if _, e = os.Stat(filepath.Join(view.Dir, "payload.bin")); e != nil {
					t.Fatal("remove without delete removed data")
				}
			})
		}
	}
}
func TestRejectUnsafeNames(t *testing.T) {
	for _, name := range []string{"..", "../escape", "a/b", "a\\b", "C:evil", "NUL.txt", "COM1", "name.", "name ", "a\x00b"} {
		t.Run(name, func(t *testing.T) {
			if safeComponent(name) == nil {
				t.Fatalf("accepted %q", name)
			}
		})
	}
	for _, name := range []string{"Ubuntu.iso", "файл.txt", ".pad", "a b"} {
		if e := safeComponent(name); e != nil {
			t.Fatal(e)
		}
	}
}
func TestRejectMalformedV2AndDuplicatePaths(t *testing.T) {
	info := metainfo.Info{Name: "safe", PieceLength: 16384, MetaVersion: 2, FileTree: metainfo.FileTree{Dir: map[string]metainfo.FileTree{"payload": {File: metainfo.FileTreeFile{Length: 1, PiecesRoot: "bad"}}}}}
	if validateInfo(&info) == nil {
		t.Fatal("accepted invalid root")
	}
	info = metainfo.Info{Name: "safe", PieceLength: 16384, Files: []metainfo.FileInfo{{Path: []string{"A.txt"}, Length: 1}, {Path: []string{"a.txt"}, Length: 1}}}
	if validateInfo(&info) == nil {
		t.Fatal("accepted case-insensitive duplicate")
	}
}
