$ErrorActionPreference = 'Stop'
Push-Location (Join-Path $PSScriptRoot '..')
try {
    go run .\tools\icon-art
    if ($LASTEXITCODE -ne 0) { throw 'Icon generation failed.' }
    go tool rsrc -arch amd64 -manifest .\assets\smart-desktop.manifest -ico .\assets\smart-desktop.ico -o .\cmd\smart-desktop\resource_windows_amd64.syso
    if ($LASTEXITCODE -ne 0) { throw 'Windows resource generation failed.' }
    if (-not (Test-Path .\dist)) { New-Item -ItemType Directory -Path .\dist | Out-Null }
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'
    go build -trimpath -ldflags '-H=windowsgui' -o .\dist\SmartDesktop.exe .\cmd\smart-desktop
    if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
    Write-Host 'Built dist\SmartDesktop.exe'
} finally {
    Pop-Location
}
