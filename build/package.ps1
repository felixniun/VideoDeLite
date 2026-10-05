# VideoDelite release packaging (Phase 11)
#
# Output: build/dist/VideoDelite-<version>-setup.exe
#
# Usage (from project root):
#   powershell -ExecutionPolicy Bypass -File build\package.ps1
#
# Prereqs: go, node, wails, makensis, ffmpeg/ffprobe (scoop shims supported)
$ErrorActionPreference = "Stop"
$version = "1.0.0"

Write-Host "== 1/5 wails build ==" -ForegroundColor Cyan
# -s -w strips debug symbols: ~30% smaller exe, no functional change
wails build -ldflags "-s -w"
if ($LASTEXITCODE -ne 0) { throw "wails build failed" }

Write-Host "== 2/5 staging release files ==" -ForegroundColor Cyan
$stage = "build\dist\app"
if (Test-Path $stage) { Remove-Item -Recurse -Force $stage }
New-Item -ItemType Directory -Force -Path "$stage\bin" | Out-Null
# makensis needs an absolute stage path
$stageAbs = (Resolve-Path "build\dist").Path + "\app"

Copy-Item "build\bin\VideoDelite.exe" $stage

function Find-Tool($name) {
    # Source priority:
    #   1. cached gyan ESSENTIALS build (build/cache/ffmpeg-essentials)
    #      - same encoders, ~half the size of full; quality identical
    #   2. scoop real binary (apps/ffmpeg/current/bin) - validated by running
    #   3. scoop shims - validated by running (rejected if launcher only)
    $cacheDir = "build\cache\ffmpeg-essentials"
    $cacheFile = Join-Path (Join-Path $cacheDir "bin") $name
    if (Test-Path $cacheFile) { return $cacheFile }
    $cacheFile2 = Join-Path $cacheDir $name
    if (Test-Path $cacheFile2) { return $cacheFile2 }

    $candidates = @(
        "D:\scoop\apps\ffmpeg\current\bin\$name",
        "$env:USERPROFILE\scoop\apps\ffmpeg\current\bin\$name",
        "C:\scoop\apps\ffmpeg\current\bin\$name",
        "D:\scoop\shims\$name",
        "$env:USERPROFILE\scoop\shims\$name",
        "C:\scoop\shims\$name"
    )
    foreach ($p in $candidates) {
        if (Test-Path $p) {
            & $p -version *> $null
            if ($LASTEXITCODE -eq 0) { return $p }
        }
    }
    $c = Get-Command $name -ErrorAction SilentlyContinue
    if ($c) {
        & $c.Source -version *> $null
        if ($LASTEXITCODE -eq 0) { return $c.Source }
    }
    throw "$name not found or not runnable in known locations"
}

# FFmpeg distribution (plan section 80: source and license must ship along)
$ff = Find-Tool "ffmpeg.exe"
$fp = Find-Tool "ffprobe.exe"
$ffDir = Split-Path $ff
Write-Host "   ffmpeg: $ff"
Copy-Item $ff "$stage\bin\"
Copy-Item $fp "$stage\bin\"
# DLLs the full build depends on
Get-ChildItem $ffDir -Filter *.dll | ForEach-Object { Copy-Item $_.FullName "$stage\bin\" }

Write-Host "== 3/5 license files ==" -ForegroundColor Cyan
Copy-Item "installer\THIRD-PARTY-NOTICES.txt" $stage

Write-Host "== 4/5 NSIS installer ==" -ForegroundColor Cyan
makensis /DVERSION=$version "/DSTAGE=$stageAbs" "installer\installer.nsi"
if ($LASTEXITCODE -ne 0) { throw "makensis failed" }

Write-Host "== 5/5 done ==" -ForegroundColor Green
Get-ChildItem "build\dist" -Filter *.exe | Format-Table Name, Length
