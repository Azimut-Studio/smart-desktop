$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'go-vendor.ps1')

Invoke-VendoredGo {
    go vet .\...
    if ($LASTEXITCODE -ne 0) { throw 'Go vet failed.' }
    go test .\... -count=1
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed.' }
}
