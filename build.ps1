$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
function Check-Exit { if ($LASTEXITCODE -ne 0) { throw "Build step failed: $LASTEXITCODE" } }
go mod download
Check-Exit
npm ci --prefix frontend
Check-Exit
go run ./cmd/icon
Check-Exit
wails3 generate bindings -ts
Check-Exit
npm run build --prefix frontend
Check-Exit
go test ./...
Check-Exit
wails3 generate syso -arch amd64 -icon build/windows/icon.ico -manifest build/windows/wails.exe.manifest -info build/windows/info.json -out wails_windows_amd64.syso
Check-Exit
go build -tags production -trimpath -ldflags '-s -w -H windowsgui' -o bin/Nocturne.exe .
Check-Exit
Write-Host "Built: $PSScriptRoot/bin/Nocturne.exe"
