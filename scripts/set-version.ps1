[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string] $Version
)

$ErrorActionPreference = 'Stop'
$versionNumber = $Version.Trim().TrimStart('v')
if ($versionNumber -notmatch '^\d+\.\d+\.\d+$') {
    throw "Version must use stable SemVer format (for example 1.2.3), got '$Version'."
}

$projectRoot = Split-Path -Parent $PSScriptRoot

function Read-Utf8([string] $RelativePath) {
    return [IO.File]::ReadAllText((Join-Path $projectRoot $RelativePath))
}

function Write-Utf8([string] $RelativePath, [string] $Content) {
    $path = Join-Path $projectRoot $RelativePath
    [IO.File]::WriteAllText($path, $Content, [Text.UTF8Encoding]::new($false))
}

function Replace-Required([string] $RelativePath, [string] $Pattern, [string] $Replacement) {
    $content = Read-Utf8 $RelativePath
    $regex = [regex]::new($Pattern, [Text.RegularExpressions.RegexOptions]::Multiline)
    if (-not $regex.IsMatch($content)) {
        throw "Could not find the version field in $RelativePath."
    }
    Write-Utf8 $RelativePath ($regex.Replace($content, $Replacement, 1))
}

Replace-Required 'build/config.yml' '(?m)^(  version: )"[^"]+"( # Updated from the release tag.*)$' "`$1`"$versionNumber`"`$2"
Replace-Required 'build/linux/nfpm/nfpm.yaml' '(?m)^(version: )"[^"]+"$' "`$1`"$versionNumber`""
Replace-Required 'build/windows/nsis/wails_tools.nsh' '(?m)^([ ]*!define INFO_PRODUCTVERSION )"[^"]+"$' "`$1`"$versionNumber`""
Replace-Required 'build/windows/wails.exe.manifest' '(?m)^(\s*<assemblyIdentity[^>]* version=")[^"]+("[^>]*/>)$' ('${1}' + $versionNumber + '.0${2}')

$windowsInfoPath = Join-Path $projectRoot 'build/windows/info.json'
$windowsInfo = Get-Content -LiteralPath $windowsInfoPath -Raw | ConvertFrom-Json
$windowsInfo.fixed.file_version = $versionNumber
$windowsInfo.info.'0000'.ProductVersion = $versionNumber
$windowsInfo | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $windowsInfoPath -Encoding utf8

foreach ($plist in @('build/darwin/Info.plist', 'build/darwin/Info.dev.plist')) {
    $content = Read-Utf8 $plist
    foreach ($key in @('CFBundleVersion', 'CFBundleShortVersionString')) {
        $pattern = "(?s)(<key>$key</key>\s*<string>)[^<]+(</string>)"
        $regex = [regex]::new($pattern)
        if (-not $regex.IsMatch($content)) {
            throw "Could not find $key in $plist."
        }
        $content = $regex.Replace($content, ('${1}' + $versionNumber + '${2}'), 1)
    }
    Write-Utf8 $plist $content
}

foreach ($jsonFile in @('frontend/package.json', 'frontend/package-lock.json')) {
    $path = Join-Path $projectRoot $jsonFile
    $package = Get-Content -LiteralPath $path -Raw | ConvertFrom-Json -AsHashtable
    $package['version'] = $versionNumber
    if ($jsonFile -like '*package-lock.json' -and $package['packages'].ContainsKey('')) {
        $package['packages']['']['version'] = $versionNumber
    }
    $package | ConvertTo-Json -Depth 100 | Set-Content -LiteralPath $path -Encoding utf8
}

Write-Host "Nocturne version set to $versionNumber"
