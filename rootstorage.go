package main

import (
	"context"
	"fmt"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Rooted I/O confines all file access to the selected job directory, including
// symlink resolution. Every operation closes its file handle, including Windows.
// Completion is deliberately reverified after restart instead of trusting a cache.
type rootStorage struct {
	dir       string
	stripName bool
}

func (r *rootStorage) Close() error { return nil }

type diskFile struct {
	path          string
	start, length int64
}
type rootTorrent struct {
	root     *os.Root
	files    []diskFile
	mu       sync.Mutex
	complete map[int]storage.Completion
}

func (r *rootStorage) OpenTorrent(_ context.Context, info *metainfo.Info, _ metainfo.Hash) (storage.TorrentImpl, error) {
	if e := validateInfo(info); e != nil {
		return storage.TorrentImpl{}, e
	}
	root, e := os.OpenRoot(r.dir)
	if e != nil {
		return storage.TorrentImpl{}, e
	}
	rt := &rootTorrent{root: root, complete: map[int]storage.Completion{}}
	for _, f := range info.UpvertedFiles() {
		parts := append([]string{info.BestName()}, f.BestPath()...)
		if r.stripName && len(f.BestPath()) > 0 {
			parts = f.BestPath()
		}
		p := filepath.Join(parts...)
		rt.files = append(rt.files, diskFile{p, f.TorrentOffset, f.Length})
		if f.Length == 0 {
			if e = root.MkdirAll(filepath.Dir(p), 0700); e != nil {
				root.Close()
				return storage.TorrentImpl{}, e
			}
			file, err := root.OpenFile(p, os.O_CREATE|os.O_WRONLY, 0600)
			if err != nil {
				root.Close()
				return storage.TorrentImpl{}, err
			}
			file.Close()
		}
	}
	return storage.TorrentImpl{Piece: func(p metainfo.Piece) storage.PieceImpl { return &rootPiece{rt, p} }, Close: root.Close}, nil
}

type rootPiece struct {
	t *rootTorrent
	p metainfo.Piece
}

func (p *rootPiece) ReadAt(b []byte, off int64) (int, error)  { return p.transfer(b, off, false) }
func (p *rootPiece) WriteAt(b []byte, off int64) (int, error) { return p.transfer(b, off, true) }
func (p *rootPiece) transfer(b []byte, off int64, write bool) (int, error) {
	if off < 0 || off > p.p.Length() {
		return 0, fmt.Errorf("piece offset out of bounds")
	}
	original := len(b)
	if int64(len(b)) > p.p.Length()-off {
		b = b[:p.p.Length()-off]
	}
	absolute := p.p.Offset() + off
	done := 0
	for done < len(b) {
		position := absolute + int64(done)
		var target *diskFile
		next := absolute + int64(len(b))
		first := sort.Search(len(p.t.files), func(i int) bool { return p.t.files[i].start+p.t.files[i].length > position })
		for i := first; i < len(p.t.files); i++ {
			f := &p.t.files[i]
			if position >= f.start && position < f.start+f.length {
				target = f
				break
			}
			if f.start > position && f.start < next {
				next = f.start
			}
		}
		if target == nil {
			n := int(next - position)
			if n <= 0 {
				return done, io.ErrUnexpectedEOF
			}
			if !write {
				clear(b[done : done+n])
			}
			done += n
			continue
		}
		n := min(len(b)-done, int(target.start+target.length-position))
		var f *os.File
		var e error
		if write {
			if e = p.t.root.MkdirAll(filepath.Dir(target.path), 0700); e != nil {
				return done, e
			}
			f, e = p.t.root.OpenFile(target.path, os.O_CREATE|os.O_WRONLY, 0600)
		} else {
			f, e = p.t.root.Open(target.path)
		}
		if e != nil {
			return done, e
		}
		var count int
		if write {
			count, e = f.WriteAt(b[done:done+n], position-target.start)
		} else {
			count, e = f.ReadAt(b[done:done+n], position-target.start)
		}
		ce := f.Close()
		done += count
		if e != nil {
			return done, e
		}
		if ce != nil {
			return done, ce
		}
		if count != n {
			return done, io.ErrUnexpectedEOF
		}
	}
	if done < original {
		if write {
			return done, io.ErrShortWrite
		}
		return done, io.EOF
	}
	return done, nil
}
func (p *rootPiece) MarkComplete() error {
	for _, f := range p.t.files {
		if f.start >= p.p.Offset()+p.p.Length() || f.start+f.length <= p.p.Offset() {
			continue
		}
		handle, e := p.t.root.OpenFile(f.path, os.O_RDWR, 0600)
		if e != nil {
			return e
		}
		e = handle.Sync()
		ce := handle.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
	}
	p.t.mu.Lock()
	p.t.complete[p.p.Index()] = storage.Completion{Ok: true, Complete: true}
	p.t.mu.Unlock()
	return nil
}
func (p *rootPiece) MarkNotComplete() error {
	p.t.mu.Lock()
	p.t.complete[p.p.Index()] = storage.Completion{Ok: true}
	p.t.mu.Unlock()
	return nil
}
func (p *rootPiece) Completion() storage.Completion {
	p.t.mu.Lock()
	defer p.t.mu.Unlock()
	return p.t.complete[p.p.Index()]
}
