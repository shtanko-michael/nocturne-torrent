# Third-party components

Nocturne uses unmodified external Go/npm libraries. Versions and integrity hashes are pinned in go.mod/go.sum and frontend/package-lock.json.

- anacrolix/torrent — MPL-2.0. Source for the pinned version: https://github.com/anacrolix/torrent/tree/5561014ea28f . License included at notices/anacrolix-MPL-2.0.txt. Go fetches the corresponding source with go mod download.
- Wails v3 — MIT. https://github.com/wailsapp/wails/tree/v3.0.0-alpha.96/v3
- React / React DOM — MIT. https://github.com/facebook/react
- lucide-react — ISC. https://github.com/lucide-icons/lucide
- modernc.org/sqlite — see its LICENSE, included in notices/.

The complete dependency inventory is obtained using go list -m all and npm ls --all --prefix frontend. This file does not relicense third-party code.
