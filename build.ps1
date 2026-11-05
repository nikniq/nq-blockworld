# build.ps1 - build and package Blockworld on Windows from PowerShell.
# (cx.sh does the same from Git Bash or MSYS2, and on macOS and Linux.)
#
#   .\build.ps1                  test, build and package into dist\
#   .\build.ps1 -BuildOnly       just compile blockworld.exe here: no tests, no package
#   .\build.ps1 -Version 1.2     version stamped into the game and the package name
#   .\build.ps1 -SkipTests       package without running the test suite
#   .\build.ps1 -Run             launch the game afterwards
#   .\build.ps1 -Clean           remove dist\ and build outputs
#
# Needs Go and a GCC toolchain on PATH (w64devkit, or MSYS2 mingw-w64).
# If scripts are blocked, run build.bat instead, which passes the same options through.
param(
    [string]$Version = "",
    [switch]$BuildOnly,
    [switch]$SkipTests,
    [switch]$Run,
    [switch]$Clean
)

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

function Say($msg) { Write-Host "==> $msg" -ForegroundColor Cyan }
function Fail($msg) { Write-Host "error: $msg" -ForegroundColor Red; exit 1 }
function Need($tool, $hint) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) { Fail "$tool is required: $hint" }
}
# Check stops the script when the program that just ran reported a failure.
function Check($what) { if ($LASTEXITCODE -ne 0) { Fail "$what failed" } }

if ($Clean) {
    Remove-Item -Recurse -Force dist, blockworld.exe, *.syso -ErrorAction SilentlyContinue
    Write-Host "cleaned"
    exit 0
}

Need go "install Go 1.25 or newer from https://go.dev/dl/"
Need gcc "install w64devkit (https://github.com/skeeto/w64devkit) or MSYS2 mingw-w64 and put gcc on PATH"

if ($Version -eq "") {
    $tag = ""
    if (Get-Command git -ErrorAction SilentlyContinue) {
        $tag = (& git describe --tags --abbrev=0 2>$null)
    }
    if ($tag) { $Version = "$tag".TrimStart("v") } else { $Version = "1.0" }
}

$env:CGO_ENABLED = "1"
$ldflags = "-s -w -H windowsgui -X main.gameVersion=$Version"

if ($BuildOnly) {
    Say "building blockworld.exe"
    & go build -trimpath -ldflags="$ldflags" -o blockworld.exe .
    Check "the build"
    Say "done: .\blockworld.exe"
    if ($Run) { & .\blockworld.exe }
    exit 0
}

$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "x64" }
$pkg = "blockworld-$Version-windows-$arch"
Say "Blockworld $Version for Windows/$arch"
New-Item -ItemType Directory -Force dist | Out-Null

Say "generating icon"
& go run ./cmd/mkicon
Check "the icon generator"

if (-not $SkipTests) {
    Say "running tests"
    & go test ./... *> dist\test.log
    if ($LASTEXITCODE -ne 0) {
        Get-Content dist\test.log -Tail 40
        Fail "tests failed (full log in dist\test.log)"
    }
    Remove-Item dist\test.log
}

if (-not (Get-Command go-winres -ErrorAction SilentlyContinue)) {
    Say "installing go-winres (embeds the icon into the .exe)"
    & go install github.com/tc-hib/go-winres@latest
    Check "installing go-winres"
    $env:PATH += ";" + (Join-Path (& go env GOPATH) "bin")
    Need go-winres "add the bin folder of 'go env GOPATH' to PATH"
}

Say "embedding icon and building"
# Windows wants a purely numeric file version; a tag such as 1.2-beta keeps its number.
$numver = ($Version -replace "[^0-9.].*$", "")
if ($numver -eq "") { $numver = "0.0" }
& go-winres simply --icon icon.png --product-name Blockworld --file-description "Blockworld" `
    --product-version $numver --file-version $numver
Check "go-winres"

Remove-Item -Recurse -Force "dist\$pkg", "dist\$pkg.zip" -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force "dist\$pkg" | Out-Null
& go build -trimpath -ldflags="$ldflags" -o "dist\$pkg\blockworld.exe" .
Check "the build"
Remove-Item *.syso -ErrorAction SilentlyContinue

Compress-Archive -Force -Path "dist\$pkg" -DestinationPath "dist\$pkg.zip"

Say "checksums"
$sums = Get-ChildItem dist -Filter *.zip | ForEach-Object {
    "{0}  {1}" -f (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLower(), $_.Name
}
$sums | Set-Content -Encoding ascii dist\SHA256SUMS
$sums

Say "done"
Get-ChildItem dist | ForEach-Object { "    dist\" + $_.Name }

if ($Run) {
    Say "launching"
    & "dist\$pkg\blockworld.exe"
}
