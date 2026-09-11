import { useEffect, useRef, useState, type MouseEvent, type ReactNode } from 'react'
import {
  AlertCircle, Archive, ArrowDown, ArrowUp, Bookmark, Check, CheckCircle2, CheckCheck,
  CircleDashed, Download, FileText, Folder, FolderOpen, Grid2X2, Info, List, ListFilter,
  MoreHorizontal, Pause, Play, Plus, Radio, Search, ShieldCheck, SlidersHorizontal, Trash2, Upload, X,
} from 'lucide-react'
import { blank, call, demoSnapshot, native, type Settings, type Snapshot, type Torrent } from './api'
import './style.css'
import packageInfo from '../package.json'

type ViewMode = 'cards' | 'compact'
type DetailTab = 'files' | 'trackers' | 'peers' | 'info'
type ModalName = 'add' | 'settings' | 'create' | 'remove' | null

const statusText: Record<string, string> = {
  downloading: 'Загружается', seeding: 'Раздаётся', paused: 'На паузе', queued: 'В очереди',
  checking: 'Проверка данных', metadata: 'Получение метаданных', completed: 'Завершён', error: 'Ошибка',
}

const bytes = (value: number, digits = 1) => {
  if (!value) return '0 Б'
  const index = Math.min(4, Math.floor(Math.log(Math.max(1, value)) / Math.log(1024)))
  const precision = index === 0 ? 0 : digits
  return `${(value / 1024 ** index).toLocaleString('ru-RU', { maximumFractionDigits: precision })} ${['Б', 'КиБ', 'МиБ', 'ГиБ', 'ТиБ'][index]}`
}
const speed = (value: number) => `${bytes(value, 2)}/с`
const eta = (seconds: number) => seconds < 0 ? '—' : seconds < 60 ? `${seconds} с` : seconds < 3600 ? `${Math.ceil(seconds / 60)} мин` : `${Math.floor(seconds / 3600)} ч ${Math.ceil(seconds % 3600 / 60)} мин`
const progress = (torrent: Torrent) => torrent.Wanted > 0 ? Math.min(100, torrent.WantedCompleted / torrent.Wanted * 100) : 0
const percent = (value: number) => `${value.toLocaleString('ru-RU', { minimumFractionDigits: value === 100 ? 0 : 1, maximumFractionDigits: 1 })}%`
const dateTime = (value: number) => value ? new Date(value).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—'

function tagFor(name: string) {
  const lower = name.toLowerCase()
  if (/blender|libreoffice|windows|software|setup|application/.test(lower)) return { id: 'software', title: 'Софт', icon: FileText }
  if (/ubuntu|debian|archlinux|fedora|linux|\.iso/.test(lower)) return { id: 'distro', title: 'Дистрибутивы', icon: Download }
  if (/\.pbf|dataset|database|data|map|\.csv|\.json/.test(lower)) return { id: 'data', title: 'Данные', icon: CircleDashed }
  if (/\.tar|archive|mirror|backup/.test(lower)) return { id: 'archive', title: 'Архив', icon: Archive }
  return { id: 'software', title: 'Софт', icon: FileText }
}

function IconButton({ label, children, onClick, disabled = false, danger = false }: { label: string; children: ReactNode; onClick: (event: MouseEvent<HTMLButtonElement>) => void; disabled?: boolean; danger?: boolean }) {
  return <button type="button" className={`icon-button${danger ? ' danger' : ''}`} title={label} aria-label={label} onClick={onClick} disabled={disabled}>{children}</button>
}

function Modal({ title, subtitle, onClose, children }: { title: string; subtitle?: string; onClose: () => void; children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null)
  const latestClose = useRef(onClose)
  latestClose.current = onClose
  useEffect(() => {
    const previous = document.activeElement as HTMLElement
    const focusable = () => Array.from(ref.current?.querySelectorAll<HTMLElement>('button:not(:disabled),input,textarea,select') ?? [])
    focusable()[0]?.focus()
    const keydown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') latestClose.current()
      if (event.key !== 'Tab') return
      const items = focusable(), first = items[0], last = items[items.length - 1]
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
      if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
    }
    document.addEventListener('keydown', keydown)
    return () => { document.removeEventListener('keydown', keydown); previous?.focus() }
  }, [])
  return <div className="overlay" onMouseDown={event => event.target === event.currentTarget && onClose()}>
    <div className="modal" ref={ref} role="dialog" aria-modal="true" aria-label={title}>
      <div className="modal-heading"><div><h2>{title}</h2>{subtitle && <p>{subtitle}</p>}</div><IconButton label="Закрыть" onClick={onClose}><X size={17}/></IconButton></div>
      {children}
    </div>
  </div>
}

function StateButton({ torrent, busy, toggle }: { torrent: Torrent; busy: boolean; toggle: (torrent: Torrent) => void }) {
  const canStart = ['paused', 'queued', 'completed', 'error'].includes(torrent.Status)
  return <button className={`state-button ${torrent.Status}`} disabled={busy || torrent.Status === 'checking'} onClick={event => { event.stopPropagation(); toggle(torrent) }} title={canStart ? 'Продолжить' : 'Приостановить'}>
    {canStart ? <Play size={14} fill="currentColor"/> : <Pause size={14} fill="currentColor"/>}
  </button>
}

function StatusLine({ torrent }: { torrent: Torrent }) {
  return <div className="torrent-stats mono">
    <span className={`torrent-state ${torrent.Status}`}><i/>{torrent.Error || statusText[torrent.Status] || torrent.Status}</span>
    <span className="download-rate"><ArrowDown size={13}/>{speed(torrent.DownloadRate)}</span>
    <span className="upload-rate"><ArrowUp size={13}/>{speed(torrent.UploadRate)}</span>
    <span>{torrent.Peers} / {torrent.Seeds} пиров</span>
    <span>{bytes(torrent.Size, 2)}</span>
    <span>осталось {eta(torrent.ETA)}</span>
  </div>
}

function TorrentCard({ torrent, selected, busy, onSelect, toggle, openFolder }: { torrent: Torrent; selected: boolean; busy: boolean; onSelect: () => void; toggle: (torrent: Torrent) => void; openFolder: () => void }) {
  const tag = tagFor(torrent.Name), value = progress(torrent)
  return <article className={`torrent-card ${torrent.Status}${selected ? ' selected' : ''}`} onClick={onSelect}>
    <StateButton torrent={torrent} busy={busy} toggle={toggle}/>
    <div className="card-body">
      <div className="torrent-title"><h3 title={torrent.Name}>{torrent.Name}</h3><span className={`tag ${tag.id}`}>{tag.title}</span></div>
      <StatusLine torrent={torrent}/>
      <div className="progress-row"><div className="progress"><span style={{ width: `${value}%` }}/></div><span className="progress-value mono">{percent(value)}</span></div>
    </div>
    <div className="row-actions">
      <IconButton label="Открыть папку" onClick={event => { event.stopPropagation(); openFolder() }}><Folder size={16}/></IconButton>
      <IconButton label="Подробнее" onClick={event => { event.stopPropagation(); onSelect() }}><MoreHorizontal size={18}/></IconButton>
    </div>
  </article>
}

function CompactRow({ torrent, selected, busy, onSelect, toggle, openFolder }: { torrent: Torrent; selected: boolean; busy: boolean; onSelect: () => void; toggle: (torrent: Torrent) => void; openFolder: () => void }) {
  const tag = tagFor(torrent.Name), value = progress(torrent)
  return <article className={`compact-row ${torrent.Status}${selected ? ' selected' : ''}`} onClick={onSelect}>
    <div className="compact-main">
      <div className="torrent-title"><span className={`status-pin ${torrent.Status}`}/><h3 title={torrent.Name}>{torrent.Name}</h3><span className={`tag ${tag.id}`}>{tag.title}</span></div>
      <div className="compact-progress"><div className="progress"><span style={{ width: `${value}%` }}/></div><span className="mono">{percent(value)}</span><span className={`compact-status ${torrent.Status}`}>{torrent.Error || statusText[torrent.Status]}</span></div>
    </div>
    <div className="compact-metrics mono"><span className="download-rate"><ArrowDown size={13}/>{speed(torrent.DownloadRate)}</span><span className="upload-rate"><ArrowUp size={13}/>{speed(torrent.UploadRate)}</span><span>{torrent.Peers} / {torrent.Seeds}</span><span>{bytes(torrent.Size, 2)}</span><span>{eta(torrent.ETA)}</span></div>
    <div className="compact-actions"><StateButton torrent={torrent} busy={busy} toggle={toggle}/><IconButton label="Открыть папку" onClick={event => { event.stopPropagation(); openFolder() }}><Folder size={15}/></IconButton><IconButton label="Подробнее" onClick={event => { event.stopPropagation(); onSelect() }}><MoreHorizontal size={17}/></IconButton></div>
  </article>
}

function DetailsPanel({ torrent, tab, setTab, busy, close, setPriority }: { torrent: Torrent; tab: DetailTab; setTab: (tab: DetailTab) => void; busy: boolean; close: () => void; setPriority: (index: number, priority: number) => void }) {
  const value = progress(torrent)
  return <aside className="details-panel">
    <div className="details-head"><div><h2 title={torrent.Name}>{torrent.Name}</h2><span className={`torrent-state ${torrent.Status}`}><i/>{torrent.Error || statusText[torrent.Status]}</span></div><IconButton label="Закрыть панель" onClick={close}><X size={17}/></IconButton></div>
    <div className="details-summary">
      <div className="progress-row"><div className="progress"><span style={{ width: `${value}%` }}/></div><span className="progress-value mono">{percent(value)}</span></div>
      <div className="summary-grid"><span>Загрузка</span><strong className="download-rate mono">{speed(torrent.DownloadRate)}</strong><span>Отдача</span><strong className="upload-rate mono">{speed(torrent.UploadRate)}</strong><span>Пиры</span><strong className="mono">{torrent.Peers} / {torrent.Seeds}</strong><span>Осталось</span><strong className="mono">{eta(torrent.ETA)}</strong><span>Размер</span><strong className="mono">{bytes(torrent.Size, 2)}</strong><span>Рейтинг</span><strong className="mono">{torrent.Ratio.toLocaleString('ru-RU', { maximumFractionDigits: 2 })}</strong></div>
    </div>
    <nav className="detail-tabs">
      {([['files', 'Файлы'], ['trackers', 'Трекеры'], ['peers', 'Пиры'], ['info', 'Сведения']] as [DetailTab, string][]).map(([id, label]) => <button key={id} className={tab === id ? 'active' : ''} onClick={() => setTab(id)}>{label}</button>)}
    </nav>
    <div className="detail-content">
      {tab === 'files' && <div className="file-list">{torrent.Files.length ? torrent.Files.map(file => {
        const fileProgress = file.Size ? Math.min(100, file.Completed / file.Size * 100) : 100
        return <div className="file-item" key={file.Index}><div className="file-line"><input type="checkbox" checked={file.Priority !== 0} disabled={busy} onChange={event => setPriority(file.Index, event.target.checked ? 1 : 0)}/><strong title={file.Name}>{file.Name}</strong><select value={file.Priority} disabled={busy} onChange={event => setPriority(file.Index, Number(event.target.value))}><option value={0}>Не скачивать</option><option value={1}>Обычный</option><option value={2}>Высокий</option></select></div><div className="file-progress"><div className="progress"><span style={{ width: `${fileProgress}%` }}/></div><span className="mono">{percent(fileProgress)}</span><span className="mono">{bytes(file.Size, 2)}</span></div></div>
      }) : <EmptyDetails text="Список файлов появится после получения метаданных."/>}</div>}
      {tab === 'trackers' && <div className="tracker-list">{torrent.Trackers.length ? torrent.Trackers.map((tracker, index) => <div key={`${tracker}-${index}`}><Radio size={14}/><div><strong>{tracker}</strong><span>Источник пиров активен</span></div></div>) : <EmptyDetails text="Трекеры не указаны. Поиск пиров выполняется через DHT и PEX."/>}</div>}
      {tab === 'peers' && <div className="peer-list">{torrent.PeerList.length ? torrent.PeerList.map((peer, index) => <div className="peer-item" key={`${peer.Address}-${index}`}><div><strong className="mono">{peer.Address}</strong><span>{peer.Client}</span></div><div className="peer-rates mono"><span className="download-rate"><ArrowDown size={13}/>{speed(peer.DownloadRate)}</span><span className="upload-rate"><ArrowUp size={13}/>{speed(peer.UploadRate)}</span></div></div>) : <EmptyDetails text="Нет активных подключений к пирам."/>}</div>}
      {tab === 'info' && <div className="info-list"><InfoRow label="Хеш" value={torrent.Hash || 'Ожидание метаданных'}/><InfoRow label="Папка" value={torrent.Dir}/><InfoRow label="Размер" value={bytes(torrent.Size, 2)}/><InfoRow label="Части" value={torrent.Pieces ? `${torrent.Pieces.toLocaleString('ru-RU')} × ${bytes(torrent.PieceLength, 0)}` : '—'}/><InfoRow label="Загружено" value={bytes(torrent.Downloaded, 2)}/><InfoRow label="Отдано" value={bytes(torrent.Uploaded, 2)}/><InfoRow label="Рейтинг" value={torrent.Ratio.toLocaleString('ru-RU', { maximumFractionDigits: 2 })}/><InfoRow label="Добавлен" value={dateTime(torrent.Added)}/></div>}
    </div>
  </aside>
}

function EmptyDetails({ text }: { text: string }) { return <div className="detail-empty"><Info size={18}/><p>{text}</p></div> }
function InfoRow({ label, value }: { label: string; value: string }) { return <div className="info-row"><span>{label}</span><strong className="mono">{value}</strong></div> }

export default function App() {
  const [snapshot, setSnapshot] = useState<Snapshot>(blank)
  const [connected, setConnected] = useState(false)
  const [demo, setDemo] = useState(!native)
  const [filter, setFilter] = useState('all')
  const [search, setSearch] = useState('')
  const [sort, setSort] = useState('recent')
  const [view, setView] = useState<ViewMode>('cards')
  const [selectedID, setSelectedID] = useState<string | null>(null)
  const [detailTab, setDetailTab] = useState<DetailTab>('files')
  const [page, setPage] = useState<'torrents' | 'events'>('torrents')
  const [modal, setModal] = useState<ModalName>(null)
  const [removeTarget, setRemoveTarget] = useState<Torrent | null>(null)
  const [deleteData, setDeleteData] = useState(false)
  const [source, setSource] = useState('')
  const [directory, setDirectory] = useState('')
  const [paused, setPaused] = useState(false)
  const [trackers, setTrackers] = useState('')
  const [settings, setSettings] = useState<Settings>(blank.Settings)
  const [previewName, setPreviewName] = useState('')
  const [previewPending, setPreviewPending] = useState(false)
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')

  const refresh = async () => { const data = await call<Snapshot>('Snapshot'); setSnapshot(data); setConnected(true) }
  useEffect(() => {
    if (demo) { setSnapshot(demoSnapshot()); setConnected(true); return }
    if (!native) { setSnapshot(blank); setConnected(false); return }
    let stopped = false
    const poll = async () => { try { const data = await call<Snapshot>('Snapshot'); if (!stopped) { setSnapshot(data); setConnected(true) } } catch (reason) { if (!stopped) { setConnected(false); setError(String(reason)) } } }
    void poll(); const timer = window.setInterval(poll, 1000)
    return () => { stopped = true; window.clearInterval(timer) }
  }, [demo])
  useEffect(() => { if (!notice) return; const timer = window.setTimeout(() => setNotice(''), 3500); return () => window.clearTimeout(timer) }, [notice])
  useEffect(() => {
    if (modal !== 'add' || !source.trim() || demo || !native) { setPreviewName(''); setPreviewPending(false); return }
    let cancelled = false; setPreviewPending(true)
    const timer = window.setTimeout(() => void call<{ Name: string; Directory: string }>('Preview', source).then(result => { if (!cancelled) { setDirectory(result.Directory); setPreviewName(result.Name); setError('') } }).catch(reason => { if (!cancelled) { setPreviewName(''); setError(String(reason)) } }).finally(() => { if (!cancelled) setPreviewPending(false) }), 300)
    return () => { cancelled = true; window.clearTimeout(timer) }
  }, [source, modal, demo])

  const action = async (work: () => Promise<unknown>, message = '') => {
    if (demo) { setNotice('Демонстрационный режим: действие не выполняется.'); return }
    if (!native) { setError('Запустите Nocturne.exe для работы с торрентами.'); return }
    setBusy(true); setError('')
    try { await work(); await refresh(); if (message) setNotice(message) } catch (reason) { setError(String(reason)) } finally { setBusy(false) }
  }
  const pick = async (method: 'PickTorrent' | 'PickDirectory', setter: (value: string) => void) => action(async () => { const path = await call<string>(method); if (path) setter(path) })
  const toggle = (torrent: Torrent) => void action(() => call('Pause', torrent.ID, torrent.Status !== 'paused' && torrent.Status !== 'queued' && torrent.Status !== 'completed' && torrent.Status !== 'error'))
  const openFolder = (torrent: Torrent) => void action(() => call('OpenFolder', torrent.ID))
  const openRemove = (torrent: Torrent) => { setRemoveTarget(torrent); setDeleteData(false); setModal('remove') }
  const openAdd = () => { setSource(''); setDirectory(snapshot.Settings.DownloadDir); setPaused(false); setModal('add') }

  const torrents = snapshot.Torrents
  const selected = torrents.find(torrent => torrent.ID === selectedID) ?? null
  const tagCounts = torrents.reduce<Record<string, number>>((counts, torrent) => { const id = tagFor(torrent.Name).id; counts[id] = (counts[id] ?? 0) + 1; return counts }, {})
  const statusCount = (id: string) => torrents.filter(torrent => id === 'all' || id === 'downloading' && ['downloading', 'metadata', 'checking'].includes(torrent.Status) || id === 'completed' && ['completed', 'seeding'].includes(torrent.Status) || torrent.Status === id).length
  const visible = torrents.filter(torrent => {
    const matchesFilter = filter === 'all' || filter.startsWith('tag:') ? filter === 'all' || tagFor(torrent.Name).id === filter.slice(4) : filter === 'downloading' ? ['downloading', 'metadata', 'checking'].includes(torrent.Status) : filter === 'completed' ? ['completed', 'seeding'].includes(torrent.Status) : torrent.Status === filter
    return matchesFilter && torrent.Name.toLowerCase().includes(search.trim().toLowerCase())
  }).sort((a, b) => sort === 'name' ? a.Name.localeCompare(b.Name) : sort === 'progress' ? progress(b) - progress(a) : b.Added - a.Added)
  const totalDown = torrents.reduce((sum, torrent) => sum + torrent.DownloadRate, 0)
  const totalUp = torrents.reduce((sum, torrent) => sum + torrent.UploadRate, 0)

  const library = [
    { id: 'all', title: 'Все торренты', icon: Grid2X2 }, { id: 'downloading', title: 'Загружаются', icon: Download },
    { id: 'seeding', title: 'Раздаются', icon: Upload }, { id: 'completed', title: 'Завершённые', icon: CheckCheck },
    { id: 'paused', title: 'На паузе', icon: Pause }, { id: 'queued', title: 'В очереди', icon: List },
    { id: 'error', title: 'С ошибками', icon: AlertCircle },
  ]
  const tags = [{ id: 'distro', title: 'Дистрибутивы', color: 'blue' }, { id: 'software', title: 'Софт', color: 'violet' }, { id: 'data', title: 'Данные', color: 'green' }, { id: 'archive', title: 'Архив', color: 'amber' }]

  return <div className="app-shell">
    <aside className="sidebar">
      <button className="brand" onDoubleClick={() => setDemo(value => !value)} onClick={() => { setPage('torrents'); setFilter('all') }}><span className="moon"/><strong>nocturne</strong></button>
      <div className="side-section">БИБЛИОТЕКА</div>
      <nav className="side-nav">{library.map(item => <button key={item.id} className={page === 'torrents' && filter === item.id ? 'active' : ''} onClick={() => { setPage('torrents'); setFilter(item.id) }}><item.icon size={17}/><span>{item.title}</span><small>{statusCount(item.id)}</small></button>)}</nav>
      <div className="side-section tags-title">МЕТКИ</div>
      <nav className="side-nav tag-nav">{tags.map(tag => <button key={tag.id} className={page === 'torrents' && filter === `tag:${tag.id}` ? 'active' : ''} onClick={() => { setPage('torrents'); setFilter(`tag:${tag.id}`) }}><i className={tag.color}/><span>{tag.title}</span><small>{tagCounts[tag.id] ?? 0}</small></button>)}</nav>
      <nav className="side-bottom"><button onClick={() => { setSettings({ ...snapshot.Settings }); setModal('settings') }}><SlidersHorizontal size={17}/><span>Настройки</span></button><button className={page === 'events' ? 'active' : ''} onClick={() => setPage('events')}><Bookmark size={17}/><span>Журнал событий</span></button></nav>
    </aside>

    <section className={`workspace${selected ? ' has-details' : ''}`}>
      <header className="topbar">
        <div className="topbar-actions"><button className="primary-action" onClick={openAdd}><Plus size={17}/><span>Добавить торрент</span></button><button className="secondary-action" onClick={() => { setSource(''); setTrackers(''); setModal('create') }}><FileText size={16}/><span>Создать</span></button><span className="toolbar-divider"/><IconButton label={selected?.Status === 'paused' ? 'Продолжить' : 'Приостановить'} disabled={!selected || busy} onClick={() => selected && toggle(selected)}>{selected?.Status === 'paused' ? <Play size={16}/> : <Pause size={16}/>}</IconButton><IconButton label="Открыть папку" disabled={!selected} onClick={() => selected && openFolder(selected)}><Folder size={16}/></IconButton><IconButton label="Удалить" danger disabled={!selected} onClick={() => selected && openRemove(selected)}><Trash2 size={16}/></IconButton></div>
        <div className="topbar-filters"><label className="search"><Search size={16}/><input value={search} onChange={event => setSearch(event.target.value)} placeholder="Поиск по названию"/></label><label className="sort"><ListFilter size={16}/><select value={sort} onChange={event => setSort(event.target.value)}><option value="recent">Сначала новые</option><option value="name">По названию</option><option value="progress">По прогрессу</option></select></label><div className="view-switch"><button className={view === 'compact' ? 'active' : ''} onClick={() => setView('compact')} title="Компактный список"><List size={17}/></button><button className={view === 'cards' ? 'active' : ''} onClick={() => setView('cards')} title="Карточки"><Grid2X2 size={17}/></button></div></div>
      </header>

      <div className="content-area">
        {error && <div className="inline-alert"><AlertCircle size={15}/><span>{error}</span><button onClick={() => setError('')}><X size={14}/></button></div>}
        {snapshot.Error && <div className="inline-alert"><AlertCircle size={15}/><span>{snapshot.Error}</span></div>}
        {page === 'events' ? <section className="events-page"><div className="events-heading"><ShieldCheck size={24}/><div><h1>Журнал событий</h1><p>Состояние движка, проверка целостности и ошибки текущей сессии.</p></div></div>{snapshot.Events.length ? snapshot.Events.map((event, index) => <div className="event-row" key={`${event}-${index}`}><i/><span className="mono">{event}</span></div>) : <div className="empty-list">Событий пока нет</div>}</section> : visible.length ? <div className={view === 'cards' ? 'card-list' : 'compact-list'}>{visible.map(torrent => view === 'cards' ? <TorrentCard key={torrent.ID} torrent={torrent} selected={selectedID === torrent.ID} busy={busy} onSelect={() => { setSelectedID(torrent.ID); setDetailTab('files') }} toggle={toggle} openFolder={() => openFolder(torrent)}/> : <CompactRow key={torrent.ID} torrent={torrent} selected={selectedID === torrent.ID} busy={busy} onSelect={() => { setSelectedID(torrent.ID); setDetailTab('files') }} toggle={toggle} openFolder={() => openFolder(torrent)}/>)}</div> : <div className="empty-list"><Download size={27}/><h2>{torrents.length ? 'Ничего не найдено' : 'Добавьте первый торрент'}</h2><p>{torrents.length ? 'Измените фильтр или поисковый запрос.' : 'Поддерживаются .torrent-файлы и magnet-ссылки.'}</p><button className="primary-action" onClick={torrents.length ? () => { setFilter('all'); setSearch('') } : openAdd}>{torrents.length ? 'Сбросить фильтры' : 'Добавить торрент'}</button></div>}
      </div>
      {selected && (
        <DetailsPanel
          torrent={selected}
          tab={detailTab}
          setTab={setDetailTab}
          busy={busy}
          close={() => setSelectedID(null)}
          setPriority={(index, priority) => void action(() => call('SetPriority', selected.ID, index, priority))}
        />
      )}
      <footer className="statusbar"><div><span className="network-dot"/>DHT: {snapshot.DHTNodes || '—'} узлов<i/>Порт {snapshot.Port || '—'} открыт<i/>{snapshot.Encryption ? 'Шифрование включено' : 'Обычное соединение'}</div><div className="mono"><span className="download-rate"><ArrowDown size={13}/>{speed(totalDown)}</span><span className="upload-rate"><ArrowUp size={13}/>{speed(totalUp)}</span><i/><span>Nocturne {packageInfo.version}{demo ? ' · demo' : connected ? '' : ' · offline'}</span></div></footer>
    </section>

    {notice && <div className="toast"><CheckCircle2 size={16}/>{notice}</div>}
    {modal === 'add' && <Modal title="Добавить торрент" subtitle="Nocturne предложит папку по названию раздачи" onClose={() => !busy && setModal(null)}><form onSubmit={event => { event.preventDefault(); void action(async () => { await call('Add', source, directory, paused); setModal(null) }, 'Торрент добавлен') }}><label className="field">Magnet-ссылка или .torrent-файл<input required value={source} onChange={event => setSource(event.target.value)} placeholder="magnet:?xt=urn:btih:…"/></label><button type="button" className="secondary-action full" disabled={busy} onClick={() => void pick('PickTorrent', setSource)}><FolderOpen size={16}/>Выбрать .torrent-файл</button><label className="field">Папка загрузки<div className="input-action"><input required value={directory} onChange={event => setDirectory(event.target.value)}/><IconButton label="Выбрать папку" onClick={() => void pick('PickDirectory', setDirectory)}><Folder size={16}/></IconButton></div></label><p className="field-hint">{previewPending ? 'Анализируем название…' : previewName ? `Найдена раздача «${previewName}». Папку можно изменить.` : 'После выбора файла или ссылки будет предложена отдельная папка.'}</p><label className="check-field"><input type="checkbox" checked={paused} onChange={event => setPaused(event.target.checked)}/>Добавить на паузе для выбора файлов</label><div className="modal-actions"><button type="button" className="secondary-action" onClick={() => setModal(null)}>Отмена</button><button className="primary-action" disabled={busy || previewPending}>{busy ? 'Добавление…' : 'Добавить торрент'}</button></div></form></Modal>}
    {modal === 'settings' && <Modal title="Настройки" subtitle="Параметры загрузки и поведения Windows" onClose={() => !busy && setModal(null)}><form onSubmit={event => { event.preventDefault(); void action(async () => { await call('SaveSettings', settings); setModal(null) }, 'Настройки сохранены') }}><label className="field">Папка для новых загрузок<div className="input-action"><input required value={settings.DownloadDir} onChange={event => setSettings({ ...settings, DownloadDir: event.target.value })}/><IconButton label="Выбрать папку" onClick={() => void pick('PickDirectory', path => setSettings({ ...settings, DownloadDir: path }))}><Folder size={16}/></IconButton></div></label><div className="field-grid"><label className="field">Загрузка, КиБ/с<input type="number" min="0" max="10000000" value={settings.DownloadKiB} onChange={event => setSettings({ ...settings, DownloadKiB: Number(event.target.value) })}/></label><label className="field">Отдача, КиБ/с<input type="number" min="0" max="10000000" value={settings.UploadKiB} onChange={event => setSettings({ ...settings, UploadKiB: Number(event.target.value) })}/></label></div><p className="field-hint">0 — без ограничения скорости.</p><label className="field">Одновременные загрузки<input type="number" min="1" max="30" value={settings.MaxActive} onChange={event => setSettings({ ...settings, MaxActive: Number(event.target.value) })}/></label><label className="check-field"><input type="checkbox" checked={settings.Seed} onChange={event => setSettings({ ...settings, Seed: event.target.checked })}/>Продолжать раздачу после загрузки</label><label className="check-field"><input type="checkbox" checked={settings.Autostart} onChange={event => setSettings({ ...settings, Autostart: event.target.checked })}/>Запускать Nocturne при входе в Windows</label><p className="field-hint">Автозапуск открывает приложение в трее. Закрытие окна не останавливает загрузки.</p><div className="modal-actions"><button type="button" className="secondary-action" onClick={() => setModal(null)}>Отмена</button><button className="primary-action" disabled={busy}><Check size={16}/>Сохранить</button></div></form></Modal>}
    {modal === 'create' && <Modal title="Создать торрент" subtitle="Создание .torrent v1 из локальной папки" onClose={() => !busy && setModal(null)}><form onSubmit={event => { event.preventDefault(); void action(async () => { const path = await call<string>('CreateTorrent', source, trackers); setModal(null); setNotice(`Создан файл: ${path}`) }) }}><label className="field">Исходная папка<div className="input-action"><input required value={source} onChange={event => setSource(event.target.value)}/><IconButton label="Выбрать папку" onClick={() => void pick('PickDirectory', setSource)}><Folder size={16}/></IconButton></div></label><label className="field">Трекеры, по одному на строку<textarea rows={4} value={trackers} onChange={event => setTrackers(event.target.value)} placeholder="udp://tracker.example:6969/announce"/></label><p className="field-hint">Хеширование большой папки может занять несколько минут.</p><div className="modal-actions"><button type="button" className="secondary-action" onClick={() => setModal(null)}>Отмена</button><button className="primary-action" disabled={busy}>{busy ? 'Создание…' : 'Создать и сохранить'}</button></div></form></Modal>}
    {modal === 'remove' && removeTarget && <Modal title="Удалить торрент?" subtitle={removeTarget.Name} onClose={() => !busy && setModal(null)}><p className="remove-text">Задание будет удалено из списка.</p><label className="check-field"><input type="checkbox" checked={deleteData} onChange={event => setDeleteData(event.target.checked)}/>Также удалить скачанные файлы с диска</label>{deleteData && <p className="delete-warning">Файлы будут удалены без перемещения в корзину.</p>}<div className="modal-actions"><button className="secondary-action" onClick={() => setModal(null)}>Отмена</button><button className="danger-action" disabled={busy} onClick={() => void action(async () => { await call('Remove', removeTarget.ID, deleteData); setSelectedID(null); setModal(null) }, 'Торрент удалён')}><Trash2 size={16}/>Удалить</button></div></Modal>}
  </div>
}
