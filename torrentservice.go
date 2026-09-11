package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/time/rate"
	"math"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Settings struct {
	Autostart   bool
	DownloadDir string
	DownloadKiB int
	UploadKiB   int
	MaxActive   int
	Seed        bool
}
type Job struct {
	NamedDirectory bool
	ID             string
	Source         string
	Dir            string
	Name           string
	Added          int64
	Paused         bool
	Priorities     map[int]int
	Meta           []byte
	Uploaded       int64
	Downloaded     int64
	Error          string
}
type FileView struct {
	Index     int
	Name      string
	Size      int64
	Completed int64
	Priority  int
}
type PeerView struct {
	Address      string
	Client       string
	Progress     float64
	DownloadRate float64
	UploadRate   float64
}
type TorrentView struct {
	ID              string
	Name            string
	Status          string
	Error           string
	Dir             string
	Added           int64
	Size            int64
	Completed       int64
	Wanted          int64
	WantedCompleted int64
	DownloadRate    float64
	UploadRate      float64
	Uploaded        int64
	Downloaded      int64
	Peers           int
	Seeds           int
	Ratio           float64
	ETA             int64
	Version         string
	Files           []FileView
	Trackers        []string
	History         []float64
	BadPieces       int64
	Hash            string
	Pieces          int
	PieceLength     int64
	PeerList        []PeerView
}
type Snapshot struct {
	Torrents   []TorrentView
	Settings   Settings
	Events     []string
	Error      string
	DHTNodes   int
	Port       int
	Encryption bool
}

func jsonNumber(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

type liveJob struct {
	job              *Job
	t                *torrent.Torrent
	store            *guardedStorage
	ready            bool
	queued           bool
	verifying        bool
	down, up         int64
	downRate, upRate float64
	history          []float64
}
type diskState struct {
	Settings Settings
	Jobs     []*Job
}
type TorrentService struct {
	mu                             sync.Mutex
	app                            *application.App
	db                             *sql.DB
	client                         *torrent.Client
	settings                       Settings
	jobs                           []*liveJob
	events                         []string
	stop                           chan struct{}
	done                           chan struct{}
	ctx                            context.Context
	cancel                         context.CancelFunc
	wg                             sync.WaitGroup
	lastError                      string
	downloadLimiter, uploadLimiter *rate.Limiter
}

func newService(stateDir string, configure ...func(*torrent.ClientConfig)) (*TorrentService, error) {
	if stateDir == "" {
		d, e := os.UserConfigDir()
		if e != nil {
			return nil, e
		}
		stateDir = filepath.Join(d, "Nocturne")
	}
	if e := os.MkdirAll(stateDir, 0700); e != nil {
		return nil, e
	}
	db, e := sql.Open("sqlite", filepath.Join(stateDir, "session.sqlite"))
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	if _, e = db.Exec("PRAGMA journal_mode=WAL; CREATE TABLE IF NOT EXISTS session (id INTEGER PRIMARY KEY CHECK(id=1), body BLOB NOT NULL)"); e != nil {
		db.Close()
		return nil, e
	}
	home, _ := os.UserHomeDir()
	s := &TorrentService{db: db, settings: Settings{DownloadDir: filepath.Join(home, "Downloads", "Nocturne"), MaxActive: 3, Seed: true}, stop: make(chan struct{}), done: make(chan struct{})}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	var raw []byte
	var saved diskState
	e = db.QueryRow("SELECT body FROM session WHERE id=1").Scan(&raw)
	if e == nil {
		if e = json.Unmarshal(raw, &saved); e != nil {
			db.Close()
			return nil, fmt.Errorf("повреждена сессия: %w", e)
		}
		s.settings = saved.Settings
	} else if e != sql.ErrNoRows {
		db.Close()
		return nil, e
	}
	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = s.settings.DownloadDir
	cfg.DefaultStorage = &rootStorage{dir: s.settings.DownloadDir}
	cfg.ListenPort = 42069
	cfg.Seed = true
	cfg.EstablishedConnsPerTorrent = 40
	cfg.TotalHalfOpenConns = 30
	s.downloadLimiter = rate.NewLimiter(rate.Inf, 1<<20)
	s.uploadLimiter = rate.NewLimiter(rate.Inf, 1<<20)
	cfg.DownloadRateLimiter = s.downloadLimiter
	cfg.UploadRateLimiter = s.uploadLimiter
	s.applyLimits()
	for _, apply := range configure {
		apply(cfg)
	}
	for attempt := 0; attempt < 10; attempt++ {
		s.client, e = torrent.NewClient(cfg)
		if e == nil {
			break
		}
		cfg.ListenPort++
	}
	if e != nil {
		db.Close()
		return nil, e
	}
	for _, j := range saved.Jobs {
		l, e := s.attach(j)
		if e != nil {
			j.Error = e.Error()
			l = &liveJob{job: j}
			s.log("Не удалось восстановить " + j.Name + ": " + e.Error())
		}
		s.jobs = append(s.jobs, l)
	}
	s.log("Сессия открыта. Проверка целостности включена.")
	go s.loop()
	return s, nil
}

func (s *TorrentService) log(message string) {
	s.events = append([]string{time.Now().Format("15:04:05") + "  " + message}, s.events...)
	if len(s.events) > 100 {
		s.events = s.events[:100]
	}
}
func (s *TorrentService) persist() error {
	state := diskState{Settings: s.settings, Jobs: make([]*Job, 0, len(s.jobs))}
	for _, l := range s.jobs {
		state.Jobs = append(state.Jobs, l.job)
	}
	raw, e := json.Marshal(state)
	if e != nil {
		return e
	}
	_, e = s.db.Exec("INSERT INTO session(id,body) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", raw)
	if e != nil {
		s.lastError = "Ошибка сохранения сессии: " + e.Error()
	}
	return e
}
func (s *TorrentService) applyLimits() {
	d, u := rate.Inf, rate.Inf
	if s.settings.DownloadKiB > 0 {
		d = rate.Limit(s.settings.DownloadKiB * 1024)
	}
	if s.settings.UploadKiB > 0 {
		u = rate.Limit(s.settings.UploadKiB * 1024)
	}
	s.downloadLimiter.SetLimit(d)
	s.uploadLimiter.SetLimit(u)
}
func (s *TorrentService) attach(j *Job) (*liveJob, error) {
	var spec *torrent.TorrentSpec
	var e error
	if len(j.Meta) > 0 {
		mi, err := metainfo.Load(bytes.NewReader(j.Meta))
		if err != nil {
			return nil, err
		}
		info, err := mi.UnmarshalInfo()
		if err != nil {
			return nil, err
		}
		if err = validateInfo(&info); err != nil {
			return nil, err
		}
		spec, e = torrent.TorrentSpecFromMetaInfoErr(mi)
	} else {
		spec, e = torrent.TorrentSpecFromMagnetUri(j.Source)
	}
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(j.Dir, 0700); e != nil {
		return nil, e
	}
	st := &guardedStorage{ClientImplCloser: &rootStorage{dir: j.Dir, stripName: j.NamedDirectory}, dir: j.Dir}
	spec.Storage = st
	spec.DisallowDataDownload = true
	spec.DisallowDataUpload = true
	t, fresh, e := s.client.AddTorrentSpec(spec)
	if e != nil {
		st.Close()
		return nil, e
	}
	if !fresh {
		st.Close()
		return nil, fmt.Errorf("этот торрент уже добавлен")
	}
	return &liveJob{job: j, t: t, store: st}, nil
}
func (s *TorrentService) Add(source, dir string, paused bool) (string, error) {
	name, err := sourceName(source)
	if err != nil {
		return "", err
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return "", fmt.Errorf("укажите magnet или .torrent")
	}
	j := &Job{Source: source, Added: time.Now().UnixMilli(), Paused: paused, Priorities: map[int]int{}}
	if !strings.HasPrefix(source, "magnet:?") {
		f, e := os.Open(source)
		if e != nil {
			return "", e
		}
		defer f.Close()
		stat, e := f.Stat()
		if e != nil {
			return "", e
		}
		if stat.Size() > 16<<20 {
			return "", fmt.Errorf("torrent-файл больше 16 МиБ")
		}
		j.Meta, e = os.ReadFile(source)
		if e != nil {
			return "", e
		}
		j.Name = filepath.Base(source)
	} else {
		j.Name = "Получение метаданных…"
	}
	id := make([]byte, 12)
	if _, e := rand.Read(id); e != nil {
		return "", e
	}
	j.ID = hex.EncodeToString(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	if dir == "" {
		dir = unusedDirectory(s.settings.DownloadDir, name)
	}
	abs, e := filepath.Abs(dir)
	if e != nil {
		return "", e
	}
	j.Dir = abs
	j.Name = name
	j.NamedDirectory = true
	if e = claimDirectory(j.Dir, j.ID); e != nil {
		return "", e
	}
	l, e := s.attach(j)
	if e != nil {
		_ = os.Remove(filepath.Join(j.Dir, ".nocturne-owner"))
		return "", e
	}
	s.jobs = append(s.jobs, l)
	if e = s.persist(); e != nil {
		s.jobs = s.jobs[:len(s.jobs)-1]
		l.t.Drop()
		l.store.Close()
		return "", e
	}
	s.log("Добавлена раздача: " + j.Name)
	return j.ID, nil
}
func (s *TorrentService) find(id string) (*liveJob, error) {
	for _, l := range s.jobs {
		if l.job.ID == id {
			return l, nil
		}
	}
	return nil, fmt.Errorf("раздача не найдена")
}
func (s *TorrentService) Pause(id string, paused bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, e := s.find(id)
	if e != nil {
		return e
	}
	old := l.job.Paused
	l.job.Paused = paused
	if e = s.persist(); e != nil {
		l.job.Paused = old
		return e
	}
	s.schedule()
	return nil
}
func (s *TorrentService) SetPriority(id string, index, priority int) error {
	if priority < 0 || priority > 2 {
		return fmt.Errorf("неверный приоритет")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	l, e := s.find(id)
	if e != nil {
		return e
	}
	if l.t == nil || l.t.Info() == nil {
		return fmt.Errorf("метаданные пока не получены")
	}
	files := l.t.Files()
	if index < 0 || index >= len(files) {
		return fmt.Errorf("файл не найден")
	}
	if l.job.Priorities == nil {
		l.job.Priorities = map[int]int{}
	}
	old, had := l.job.Priorities[index]
	l.job.Priorities[index] = priority
	if e = s.persist(); e != nil {
		if had {
			l.job.Priorities[index] = old
		} else {
			delete(l.job.Priorities, index)
		}
		return e
	}
	files[index].SetPriority(filePriority(priority))
	return nil
}
func filePriority(p int) torrent.PiecePriority {
	if p == 0 {
		return torrent.PiecePriorityNone
	}
	if p == 2 {
		return torrent.PiecePriorityHigh
	}
	return torrent.PiecePriorityNormal
}
func (s *TorrentService) Remove(id string, deleteData bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, e := s.find(id)
	if e != nil {
		return e
	}
	if l.verifying {
		return fmt.Errorf("дождитесь завершения проверки")
	}
	idx := -1
	for i, v := range s.jobs {
		if v == l {
			idx = i
			break
		}
	}
	old := append([]*liveJob(nil), s.jobs...)
	s.jobs = append(s.jobs[:idx], s.jobs[idx+1:]...)
	if e = s.persist(); e != nil {
		s.jobs = old
		return e
	}
	if l.t != nil {
		l.t.Drop()
	}
	if l.store != nil {
		l.store.Close()
	}
	if deleteData {
		owned := false
		if l.job.NamedDirectory {
			marker, err := os.ReadFile(filepath.Join(l.job.Dir, ".nocturne-owner"))
			owned = err == nil && string(marker) == id
		} else {
			owned = filepath.Base(l.job.Dir) == id
		}
		if !owned || len(id) != 24 {
			return fmt.Errorf("небезопасная папка удаления")
		}
		if e = os.RemoveAll(l.job.Dir); e != nil {
			return fmt.Errorf("задание удалено, но файлы остались: %w", e)
		}
	}
	s.log("Удалена раздача: " + l.job.Name)
	return nil
}
func (s *TorrentService) Verify(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, e := s.find(id)
	if e != nil {
		return e
	}
	if l.t == nil || l.t.Info() == nil {
		return fmt.Errorf("нет метаданных")
	}
	if l.verifying {
		return nil
	}
	l.verifying = true
	l.t.DisallowDataDownload()
	l.t.DisallowDataUpload()
	s.log("Начата проверка: " + l.job.Name)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		e := l.t.VerifyDataContext(s.ctx)
		s.mu.Lock()
		defer s.mu.Unlock()
		l.verifying = false
		if e != nil {
			l.job.Error = e.Error()
			s.log("Ошибка проверки: " + e.Error())
		} else {
			s.log("Проверка завершена: " + l.job.Name)
		}
	}()
	return nil
}
func (s *TorrentService) SaveSettings(settings Settings) error {
	if settings.MaxActive < 1 || settings.MaxActive > 30 || settings.DownloadKiB < 0 || settings.UploadKiB < 0 || settings.DownloadKiB > 10000000 || settings.UploadKiB > 10000000 {
		return fmt.Errorf("проверьте ограничения")
	}
	if !filepath.IsAbs(settings.DownloadDir) {
		return fmt.Errorf("укажите абсолютный путь")
	}
	if e := os.MkdirAll(settings.DownloadDir, 0700); e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.settings
	if settings.Autostart != old.Autostart {
		if s.app == nil {
			return fmt.Errorf("автозапуск доступен в настольном приложении")
		}
		if e := s.setAutostart(settings.Autostart); e != nil {
			return e
		}
	}
	s.settings = settings
	if e := s.persist(); e != nil {
		s.settings = old
		if settings.Autostart != old.Autostart {
			_ = s.setAutostart(old.Autostart)
		}
		return e
	}
	s.applyLimits()
	s.schedule()
	return nil
}
func (s *TorrentService) schedule() {
	active := 0
	for _, l := range s.jobs {
		if l.t == nil {
			continue
		}
		l.queued = false
		if l.job.Paused || l.verifying || l.job.Error != "" {
			l.t.DisallowDataDownload()
			l.t.DisallowDataUpload()
			continue
		}
		if !l.ready {
			continue
		}
		var wanted, left int64
		for _, f := range l.t.Files() {
			if f.Priority() != torrent.PiecePriorityNone {
				wanted += f.Length()
				left += f.Length() - f.BytesCompleted()
			}
		}
		if left > 0 && wanted > 0 {
			if active >= s.settings.MaxActive {
				l.queued = true
				l.t.DisallowDataDownload()
				l.t.DisallowDataUpload()
				continue
			}
			active++
			l.t.AllowDataDownload()
			l.t.AllowDataUpload()
		} else {
			l.t.DisallowDataDownload()
			if s.settings.Seed && wanted > 0 {
				l.t.AllowDataUpload()
			} else {
				l.t.DisallowDataUpload()
			}
		}
	}
}
func (s *TorrentService) loop() {
	defer close(s.done)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	last := time.Now()
	n := 0
	for {
		select {
		case <-s.stop:
			return
		case now := <-tick.C:
			s.mu.Lock()
			dt := now.Sub(last).Seconds()
			last = now
			for _, l := range s.jobs {
				if l.t == nil {
					continue
				}
				if failure := l.store.failure.Load(); failure != nil && l.job.Error == "" {
					l.job.Error = failure.(string)
					s.log("Отклонены метаданные: " + l.job.Error)
				}
				if !l.ready && l.t.Info() != nil {
					if e := validateInfo(l.t.Info()); e != nil {
						l.job.Error = e.Error()
						l.t.DisallowDataDownload()
						l.t.DisallowDataUpload()
						continue
					}
					l.ready = true
					l.job.Name = l.t.Name()
					mi := l.t.Metainfo()
					var b bytes.Buffer
					if e := mi.Write(&b); e == nil {
						l.job.Meta = b.Bytes()
					}
					for i, f := range l.t.Files() {
						p, ok := l.job.Priorities[i]
						if !ok {
							p = 1
						}
						f.SetPriority(filePriority(p))
					}
				}
				stats := l.t.Stats()
				d, u := stats.BytesReadData.Int64(), stats.BytesWrittenData.Int64()
				l.downRate = jsonNumber(float64(d-l.down) / dt)
				l.upRate = jsonNumber(float64(u-l.up) / dt)
				l.job.Downloaded += d - l.down
				l.job.Uploaded += u - l.up
				l.down = d
				l.up = u
				l.history = append(l.history, l.downRate)
				if len(l.history) > 60 {
					l.history = l.history[1:]
				}
			}
			s.schedule()
			n++
			if n%5 == 0 {
				_ = s.persist()
			}
			s.mu.Unlock()
		}
	}
}
func (s *TorrentService) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Snapshot{Torrents: []TorrentView{}, Settings: s.settings, Events: append([]string{}, s.events...), Error: s.lastError, Encryption: true}
	if s.client != nil {
		out.Port = s.client.LocalPort()
		for _, server := range s.client.DhtServers() {
			if stats, ok := server.Stats().(dht.ServerStats); ok {
				out.DHTNodes += stats.GoodNodes
			}
		}
	}
	for _, l := range s.jobs {
		j := l.job
		history := append([]float64{}, l.history...)
		for i := range history {
			history[i] = jsonNumber(history[i])
		}
		v := TorrentView{ID: j.ID, Name: j.Name, Dir: j.Dir, Added: j.Added, Status: "metadata", Error: j.Error, Uploaded: j.Uploaded, Downloaded: j.Downloaded, History: history, Files: []FileView{}, Trackers: []string{}, PeerList: []PeerView{}, ETA: -1, Version: "—"}
		if l.t != nil && l.ready {
			v.Size = l.t.Length()
			v.Completed = l.t.BytesCompleted()
			v.DownloadRate = jsonNumber(l.downRate)
			v.UploadRate = jsonNumber(l.upRate)
			st := l.t.Stats()
			v.Peers = st.ActivePeers
			v.Seeds = st.ConnectedSeeders
			v.BadPieces = st.PiecesDirtiedBad.Int64()
			info := l.t.Info()
			v.Hash = l.t.InfoHash().HexString()
			v.Pieces = info.NumPieces()
			v.PieceLength = info.PieceLength
			v.Version = "v1"
			if info.HasV2() {
				v.Version = "v2"
				if info.HasV1() {
					v.Version = "hybrid"
				}
			}
			for i, f := range l.t.Files() {
				p := 1
				if f.Priority() == torrent.PiecePriorityNone {
					p = 0
				} else if f.Priority() == torrent.PiecePriorityHigh {
					p = 2
				}
				fv := FileView{Index: i, Name: f.DisplayPath(), Size: f.Length(), Completed: f.BytesCompleted(), Priority: p}
				v.Files = append(v.Files, fv)
				if p != 0 {
					v.Wanted += fv.Size
					v.WantedCompleted += fv.Completed
				}
			}
			for _, peer := range l.t.PeerConns() {
				ps := peer.Stats()
				clientName := "BitTorrent peer"
				if value := peer.PeerClientName.Load(); value != nil {
					if name, ok := value.(string); ok && strings.TrimSpace(name) != "" {
						clientName = name
					}
				}
				progress := float64(0)
				if v.Pieces > 0 {
					progress = float64(ps.RemotePieceCount) / float64(v.Pieces) * 100
				}
				progress = max(0, min(100, jsonNumber(progress)))
				v.PeerList = append(v.PeerList, PeerView{Address: peer.RemoteAddr.String(), Client: clientName, Progress: progress, DownloadRate: jsonNumber(ps.DownloadRate), UploadRate: jsonNumber(ps.LastWriteUploadRate)})
			}
			v.Status = "downloading"
			if v.WantedCompleted >= v.Wanted {
				v.Status = "completed"
				if s.settings.Seed && v.Wanted > 0 {
					v.Status = "seeding"
				}
			}
			if l.queued {
				v.Status = "queued"
			}
			if l.verifying {
				v.Status = "checking"
			}
			if v.DownloadRate > 0 {
				v.ETA = int64(float64(v.Wanted-v.WantedCompleted) / v.DownloadRate)
			}
			mi := l.t.Metainfo()
			for _, tier := range mi.UpvertedAnnounceList() {
				v.Trackers = append(v.Trackers, tier...)
			}
		}
		if j.Paused {
			v.Status = "paused"
		}
		if j.Error != "" {
			v.Status = "error"
		}
		if v.Downloaded > 0 {
			v.Ratio = jsonNumber(float64(v.Uploaded) / float64(v.Downloaded))
		}
		out.Torrents = append(out.Torrents, v)
	}
	sort.Slice(out.Torrents, func(i, j int) bool { return out.Torrents[i].Added > out.Torrents[j].Added })
	return out
}
func (s *TorrentService) PickTorrent() (string, error) {
	return s.app.Dialog.OpenFile().SetTitle("Открыть торрент").AddFilter("BitTorrent", "*.torrent").PromptForSingleSelection()
}
func (s *TorrentService) PickDirectory() (string, error) {
	return s.app.Dialog.OpenFile().SetTitle("Папка загрузки").CanChooseFiles(false).CanChooseDirectories(true).CanCreateDirectories(true).PromptForSingleSelection()
}
func (s *TorrentService) OpenFolder(id string) error {
	s.mu.Lock()
	l, e := s.find(id)
	var dir string
	if e == nil {
		dir = l.job.Dir
	}
	s.mu.Unlock()
	if e != nil {
		return e
	}
	return s.app.Env.OpenFileManager(dir, false)
}
func (s *TorrentService) CreateTorrent(source, trackers string) (string, error) {
	if source == "" {
		return "", fmt.Errorf("выберите исходную папку")
	}
	if e := filepath.WalkDir(source, func(_ string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("символические ссылки не поддерживаются")
		}
		return nil
	}); e != nil {
		return "", e
	}
	path, e := s.app.Dialog.SaveFile().SetFilename(filepath.Base(source)+".torrent").AddFilter("BitTorrent", "*.torrent").PromptForSingleSelection()
	if e != nil || path == "" {
		return "", e
	}
	info := metainfo.Info{PieceLength: 1 << 20}
	if e = info.BuildFromFilePath(source); e != nil {
		return "", e
	}
	if e = validateInfo(&info); e != nil {
		return "", e
	}
	raw, e := bencode.Marshal(&info)
	if e != nil {
		return "", e
	}
	mi := metainfo.MetaInfo{InfoBytes: raw, CreatedBy: "Nocturne", CreationDate: time.Now().Unix()}
	for _, tr := range strings.Fields(trackers) {
		mi.AnnounceList = append(mi.AnnounceList, []string{tr})
	}
	f, e := os.Create(path)
	if e != nil {
		return "", e
	}
	e = mi.Write(f)
	ce := f.Close()
	if e != nil {
		return "", e
	}
	return path, ce
}
func (s *TorrentService) close() {
	close(s.stop)
	<-s.done
	s.cancel()
	s.wg.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.persist()
	s.client.Close()
	for _, l := range s.jobs {
		if l.store != nil {
			l.store.Close()
		}
	}
	s.db.Close()
}
