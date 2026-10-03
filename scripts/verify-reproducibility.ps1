$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'go-vendor.ps1')

Invoke-VendoredGo {
    if ($env:SMART_DESKTOP_INTEGRATION -eq '1' -or $env:SMART_DESKTOP_ALLOW_MOVE -eq '1') {
        throw 'Unset SMART_DESKTOP_INTEGRATION and SMART_DESKTOP_ALLOW_MOVE before automated verification.'
    }

    & (Join-Path $PSScriptRoot 'test-build-policy.ps1')

    $source = (Get-Location).Path
    $temporary = Join-Path ([IO.Path]::GetTempPath()) ('smart-desktop-repro-' + [Guid]::NewGuid().ToString('N'))
    $cacheNames = @('GOCACHE', 'GOMODCACHE')
    $previousCaches = @{}
    foreach ($name in $cacheNames) {
        $previousCaches[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
    }
    New-Item -ItemType Directory -Path $temporary | Out-Null
    try {
        $hashes = @()
        foreach ($iteration in 1..2) {
            $work = Join-Path $temporary "build-$iteration"
            New-Item -ItemType Directory -Path $work | Out-Null
            foreach ($entry in @('go.mod', 'go.sum', 'vendor', 'assets', 'cmd', 'internal', 'tools', 'scripts')) {
                Copy-Item -LiteralPath (Join-Path $source $entry) -Destination $work -Recurse
            }
            $env:GOCACHE = Join-Path $temporary "build-cache-$iteration"
            $env:GOMODCACHE = Join-Path $temporary "module-cache-$iteration"
            New-Item -ItemType Directory -Path $env:GOCACHE, $env:GOMODCACHE | Out-Null

            & (Join-Path $work 'scripts\build.ps1')
            $hashes += (Get-FileHash -LiteralPath (Join-Path $work 'dist\SmartDesktop.exe') -Algorithm SHA256).Hash
            if ($iteration -eq 1) {
                & (Join-Path $work 'scripts\test.ps1')
            }
            if (@(Get-ChildItem -LiteralPath $env:GOMODCACHE -Recurse -File -Force).Count -ne 0) {
                throw "Build $iteration unexpectedly populated the module cache."
            }
        }
        if ($hashes[0] -ne $hashes[1]) {
            throw "Builds are not reproducible: $($hashes[0]) != $($hashes[1])."
        }
        Write-Host "Offline builds and tests succeeded. Identical SHA-256: $($hashes[0])"
    } finally {
        Set-GoProcessEnvironment $previousCaches
        Remove-Item -LiteralPath $temporary -Recurse -Force
    }
}
