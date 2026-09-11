# Contributing to Nocturne

## Development setup

Install Go 1.25, Node.js 24, and Wails CLI `v3.0.0-alpha.96`.

```shell
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-alpha.96
npm ci --prefix frontend
wails3 generate bindings -ts
```

Run `wails3 dev` for the desktop application. Run the checks before opening a pull request:

```shell
npm run build --prefix frontend
go test ./...
go vet ./...
```

Keep torrent fixtures synthetic and small. Do not commit downloaded content, session databases, credentials, private tracker URLs, or user-specific paths.

## Pull requests

Describe the user-visible change and how it was verified. Keep generated Wails bindings in sync with exported Go methods. Changes that handle metadata, paths, peer input, or persistence should include a focused regression test.
