$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'go-vendor.ps1')

Invoke-VendoredGo {
    go run .\tools\icon-art
    if ($LASTEXITCODE -ne 0) { throw 'Icon generation failed.' }
    go tool rsrc -arch amd64 -manifest .\assets\smart-desktop.manifest -ico .\assets\smart-desktop.ico -o .\cmd\smart-desktop\resource_windows_amd64.syso
    if ($LASTEXITCODE -ne 0) { throw 'Windows resource generation failed.' }
    if (-not (Test-Path .\dist)) { New-Item -ItemType Directory -Path .\dist | Out-Null }
    go build -trimpath -ldflags '-H=windowsgui' -o .\dist\SmartDesktop.exe .\cmd\smart-desktop
    if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
    Write-Host 'Built dist\SmartDesktop.exe'
}
