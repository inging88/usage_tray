#Requires -Version 5.1
# Windows 빌드. 결과: dist\usage-tray.exe (단일 실행 파일, 설치 불필요)
#
# -H windowsgui 는 콘솔 창이 뜨지 않게 한다 — 트레이 앱이므로 필수다.
# -s -w 는 디버그 심볼을 빼 용량을 줄인다(약 8MB → 6MB).
#
# macOS 바이너리는 여기서 만들 수 없다. macOS 의 메뉴바 API 는 cgo(Cocoa)를 거치므로
# 맥에서 `./build.sh` 를 돌려야 한다. 자세한 것은 README 의 '맥에서 쓰기'.

param([switch]$Run)

$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

$dist = Join-Path $PSScriptRoot 'dist'
New-Item -ItemType Directory -Force -Path $dist | Out-Null

$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'

go build -trimpath -ldflags '-s -w -H windowsgui' -o (Join-Path $dist 'usage-tray.exe') .
if ($LASTEXITCODE -ne 0) { throw 'build 실패' }

$exe = Get-Item (Join-Path $dist 'usage-tray.exe')
"빌드 완료: $($exe.FullName)  ($([math]::Round($exe.Length / 1MB, 1)) MB)"

if ($Run) {
    Start-Process $exe.FullName
    '실행했다 — 트레이를 확인한다'
}
