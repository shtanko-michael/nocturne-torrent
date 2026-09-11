export type Settings = { Autostart: boolean; DownloadDir: string; DownloadKiB: number; UploadKiB: number; MaxActive: number; Seed: boolean }
export type TorrentFile = { Index: number; Name: string; Size: number; Completed: number; Priority: number }
export type TorrentPeer = { Address: string; Client: string; Progress: number; DownloadRate: number; UploadRate: number }
export type Torrent = {
  ID: string; Name: string; Status: string; Error: string; Dir: string; Added: number
  Size: number; Completed: number; Wanted: number; WantedCompleted: number; DownloadRate: number; UploadRate: number
  Uploaded: number; Downloaded: number; Peers: number; Seeds: number; Ratio: number; ETA: number; Version: string
  Files: TorrentFile[]; Trackers: string[]; History: number[]; BadPieces: number
  Hash: string; Pieces: number; PieceLength: number; PeerList: TorrentPeer[]
}
export type Snapshot = {
  Torrents: Torrent[]; Settings: Settings; Events: string[]; Error: string
  DHTNodes: number; Port: number; Encryption: boolean
}

export const blank: Snapshot = {
  Torrents: [], Settings: { Autostart: false, DownloadDir: '', DownloadKiB: 0, UploadKiB: 0, MaxActive: 3, Seed: true },
  Events: [], Error: '', DHTNodes: 0, Port: 0, Encryption: true,
}

export const native = location.protocol !== 'http:' && location.protocol !== 'https:' || location.hostname === 'wails.localhost' || !!(window as any)._wails

export async function call<T>(method: string, ...args: unknown[]): Promise<T> {
  const service = await import('../bindings/nocturne/torrentservice')
  return (service as unknown as Record<string, (...args: unknown[]) => Promise<T>>)[method](...args)
}

const GiB = 1024 ** 3
const MiB = 1024 ** 2
type DemoSpec = [string, string, number, number, number, number, number, number]

export function demoSnapshot(): Snapshot {
  const specs: DemoSpec[] = [
    ['ubuntu-24.04.3-desktop-amd64.iso', 'downloading', 5.79 * GiB, .684, 12.4 * MiB, 1.1 * MiB, 24, 312],
    ['blender-4.5.2-windows-x64.zip', 'downloading', 874.3 * MiB, .121, 3.26 * MiB, 0, 9, 47],
    ['archlinux-2026.09.01-x86_64.iso', 'seeding', 1.18 * GiB, 1, 0, 4.82 * MiB, 0, 86],
    ['debian-13.0.0-amd64-DVD-1.iso', 'paused', 3.72 * GiB, .417, 0, 0, 0, 128],
    ['openstreetmap-planet-2026-09-07.osm.pbf', 'queued', 82.4 * GiB, 0, 0, 0, 0, 41],
    ['libreoffice-25.2.1-x86_64.tar.gz', 'checking', 342.6 * MiB, .87, 0, 0, 0, 19],
    ['fedora-workstation-43-x86_64.iso', 'completed', 2.41 * GiB, 1, 0, 0, 0, 0],
    ['gutenberg-mirror-2026-08.tar', 'error', 11.6 * GiB, .239, 0, 0, 0, 0],
  ]
  const peers: TorrentPeer[] = [
    { Address: '81.19.44.107:51413', Client: 'qBittorrent 5.1.2 · 100%', Progress: 100, DownloadRate: 4.1 * MiB, UploadRate: 0 },
    { Address: '176.32.9.211:6881', Client: 'Transmission 4.1 · 84%', Progress: 84, DownloadRate: 3.28 * MiB, UploadRate: 212 * 1024 },
    { Address: '92.240.118.6:24810', Client: 'Deluge 2.2.0 · 61%', Progress: 61, DownloadRate: 2.44 * MiB, UploadRate: 128 * 1024 },
    { Address: '46.238.7.152:51413', Client: 'libtorrent 2.0.11 · 97%', Progress: 97, DownloadRate: 1.96 * MiB, UploadRate: 0 },
    { Address: '213.87.160.44:6889', Client: 'Nocturne 1.0.0 · 39%', Progress: 39, DownloadRate: 640 * 1024, UploadRate: 804 * 1024 },
  ]
  return {
    ...blank, DHTNodes: 328, Port: 51413,
    Events: ['14:02:09  Данные проверены: ubuntu-24.04.3-desktop-amd64.iso', '14:02:03  Добавлена раздача: ubuntu-24.04.3-desktop-amd64.iso'],
    Torrents: specs.map(([Name, Status, Size, progress, DownloadRate, UploadRate, Peers, Seeds], i) => {
      const completed = Size * progress
      const fileNames = ['casper/vmlinuz', 'casper/initrd', 'casper/ubuntu-server-minimal.manifest', 'casper/ubuntu-desktop.squashfs', 'EFI/boot/bootx64.efi', 'boot/grub/grub.cfg', 'dists/noble/Release', 'md5sum.txt']
      const fileSizes = [12.4 * MiB, 118.7 * MiB, 1.24 * GiB, 2.86 * GiB, 966 * 1024, 2.1 * MiB, 284.5 * 1024, 1.4 * MiB]
      const fileProgress = [1, 1, .92, .61, 1, 1, 0, 0]
      return {
        ID: `demo-${i}`, Name, Status, Error: Status === 'error' ? 'Нет связи с трекером' : '',
        Dir: i === 0 ? 'D:\\Torrents\\Linux' : 'D:\\Torrents', Added: Date.now() - i * 3600000,
        Size, Completed: completed, Wanted: Size, WantedCompleted: completed, DownloadRate, UploadRate,
        Uploaded: i === 0 ? 712.4 * MiB : UploadRate * 530, Downloaded: completed, Peers, Seeds,
        Ratio: i === 0 ? .18 : completed ? UploadRate * 530 / completed : 0,
        ETA: Status === 'downloading' ? (i ? 240 : 360) : Status === 'checking' ? 12 : -1,
        Version: i === 4 ? 'v2' : i === 1 ? 'v1' : 'hybrid', BadPieces: 0,
        Hash: i === 0 ? '9f2c4a1e7b8d0356fa41c9e2d7b6083514ac9d2e' : `7a1b54c083d22f31400d948441a68bf84d4f11${i}`,
        Pieces: Math.ceil(Size / (512 * 1024)), PieceLength: 512 * 1024,
        Trackers: ['udp://tracker.opentrackr.org:1337/announce', 'https://tracker.torrent.eu.org/announce'], PeerList: i === 0 ? peers : [],
        History: Array.from({ length: 50 }, (_, n) => DownloadRate ? DownloadRate * (.72 + Math.sin(n * .27) * .13 + n / 180) : 0),
        Files: i === 0 ? fileNames.map((fileName, n) => ({ Index: n, Name: fileName, Size: fileSizes[n], Completed: fileSizes[n] * fileProgress[n], Priority: 1 })) : [{ Index: 0, Name, Size, Completed: completed, Priority: 1 }],
      }
    }),
  }
}
