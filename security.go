package main

import (
	"context"
	"fmt"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

func safeComponent(s string) error {
	if strings.EqualFold(s, ".nocturne-owner") {
		return fmt.Errorf("зарезервированное имя приложения")
	}
	if s == "" || s == "." || s == ".." || strings.ContainsAny(s, "/\\:<>\"|?*") || strings.TrimRight(s, ". ") != s {
		return fmt.Errorf("недопустимое имя файла: %q", s)
	}
	for _, c := range s {
		if c < 32 {
			return fmt.Errorf("управляющий символ в имени")
		}
	}
	base := strings.ToUpper(strings.Split(s, ".")[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9') {
		return fmt.Errorf("зарезервированное имя Windows: %s", s)
	}
	return nil
}

func validateInfo(info *metainfo.Info) error {
	if err := safeComponent(info.BestName()); err != nil {
		return err
	}
	if info.PieceLength <= 0 || info.PieceLength > 64<<20 {
		return fmt.Errorf("недопустимый размер части")
	}
	if info.HasV2() {
		count := 0
		var checkTree func(*metainfo.FileTree, int) error
		checkTree = func(tree *metainfo.FileTree, depth int) error {
			count++
			if count > 100000 || depth > 64 {
				return fmt.Errorf("слишком большое дерево файлов")
			}
			if !tree.IsDir() && tree.File.Length > 0 && len(tree.File.PiecesRoot) != 32 {
				return fmt.Errorf("неверный SHA-256 root")
			}
			for name, child := range tree.Dir {
				if e := safeComponent(name); e != nil {
					return e
				}
				if e := checkTree(&child, depth+1); e != nil {
					return e
				}
			}
			return nil
		}
		if e := checkTree(&info.FileTree, 0); e != nil {
			return e
		}
	}
	files := info.UpvertedFiles()
	if len(files) > 100000 {
		return fmt.Errorf("слишком много файлов")
	}
	seen := map[string]bool{}
	var total int64
	for _, f := range files {
		if f.Length < 0 || f.Length > 16<<40 {
			return fmt.Errorf("недопустимый размер файла")
		}
		total += f.Length
		if total > 64<<40 {
			return fmt.Errorf("раздача превышает 64 ТиБ")
		}
		for _, c := range f.BestPath() {
			if err := safeComponent(c); err != nil {
				return err
			}
		}
		key := strings.ToLower(strings.Join(f.BestPath(), "/"))
		if seen[key] {
			return fmt.Errorf("повторяющийся путь: %s", key)
		}
		seen[key] = true
	}
	return nil
}

// Validate again when magnet metadata arrives, before the file storage opens it.
type guardedStorage struct {
	storage.ClientImplCloser
	dir     string
	failure atomic.Value
}

func (s *guardedStorage) OpenTorrent(ctx context.Context, info *metainfo.Info, hash metainfo.Hash) (result storage.TorrentImpl, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("некорректные метаданные: %v", r)
		}
		if err != nil {
			s.failure.Store(err.Error())
		}
	}()
	if err := validateInfo(info); err != nil {
		return storage.TorrentImpl{}, err
	}
	// Reject pre-existing symlinks/reparse paths inside the dedicated job directory.
	err = filepath.WalkDir(s.dir, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("ссылки в папке загрузки запрещены: %s", path)
		}
		return nil
	})
	if err != nil {
		return storage.TorrentImpl{}, err
	}
	return s.ClientImplCloser.OpenTorrent(ctx, info, hash)
}
