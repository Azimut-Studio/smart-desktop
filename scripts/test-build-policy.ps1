$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'go-vendor.ps1')

$names = @('GOENV', 'GOTOOLCHAIN', 'GOWORK', 'GOPROXY', 'GOSUMDB', 'GOFLAGS',
    'GOOS', 'GOARCH', 'GOAMD64', 'GOEXPERIMENT', 'CGO_ENABLED', 'SMART_DESKTOP_INTEGRATION')
$callerEnvironment = @{}
foreach ($name in $names) {
    $callerEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
$temporary = Join-Path ([IO.Path]::GetTempPath()) ('smart-desktop-policy-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $temporary | Out-Null
Push-Location (Join-Path $PSScriptRoot '..')
try {
    Set-GoProcessEnvironment @{ GOENV = $null; GOFLAGS = '-mod=mod'; GOARCH = 'arm64' }
    $baseline = @{}
    foreach ($name in $names) {
        $baseline[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
    }
    $location = (Get-Location).Path

    function Assert-Restored {
        foreach ($name in $names) {
            if ([Environment]::GetEnvironmentVariable($name, 'Process') -cne $baseline[$name]) {
                throw "Environment was not restored: $name."
            }
        }
        if ((Get-Location).Path -ne $location) { throw 'Working directory was not restored.' }
    }

    function Assert-Failure([scriptblock] $Action, [string] $Expected) {
        $message = $null
        try { & $Action } catch { $message = $_.ToString() }
        if ($null -eq $message -or $message -notmatch $Expected) {
            throw "Expected failure '$Expected', received '$message'."
        }
        Assert-Restored
        Write-Host "Verified expected failure: $Expected"
    }

    Invoke-VendoredGo {
        if ($env:GOFLAGS -ne '-mod=vendor -trimpath -buildvcs=false' -or
            $env:GOPROXY -ne 'off' -or $env:GOTOOLCHAIN -ne 'local' -or
            $env:GOARCH -ne 'amd64' -or $env:GOENV -ne 'off') {
            throw 'Vendored build settings were not enforced.'
        }
    }
    Assert-Restored
    Assert-Failure { Invoke-VendoredGo { throw 'Intentional action failure' } } 'Intentional action failure'

    Copy-Item -LiteralPath .\go.mod, .\go.sum, .\scripts -Destination $temporary -Recurse
    $build = Join-Path $temporary 'scripts\build.ps1'
    Assert-Failure { & $build } 'vendor.*modules.txt is missing'

    $moduleFile = Join-Path $temporary 'go.mod'
    $module = [IO.File]::ReadAllText($moduleFile)
    $version = [regex]::Match($module, '(?m)^go (\d+\.\d+\.\d+)\r?$').Groups[1].Value
    $otherVersion = if ($version -eq '1.25.0') { '1.26.0' } else { '1.25.0' }
    [IO.File]::WriteAllText($moduleFile, $module.Replace("go $version", "go $otherVersion"))
    Assert-Failure { & $build } ([regex]::Escape("go$otherVersion is required"))
    [IO.File]::WriteAllText($moduleFile, $module)

    Copy-Item -LiteralPath .\vendor, .\tools -Destination $temporary -Recurse
    $manifestFile = Join-Path $temporary 'vendor\modules.txt'
    $manifest = [IO.File]::ReadAllText($manifestFile)
    $moduleLine = [regex]::Match($manifest, '(?m)^# github.com/akavel/rsrc v[^\r\n]+').Value
    if (-not $moduleLine) { throw 'rsrc is missing from the vendor manifest.' }
    [IO.File]::WriteAllText($manifestFile, $manifest.Replace($moduleLine, '# github.com/akavel/rsrc v0.0.0'))
    Assert-Failure { & $build } 'inconsistent vendoring|Icon generation failed'

    $env:SMART_DESKTOP_INTEGRATION = '1'
    try {
        $message = $null
        try { & .\scripts\verify-reproducibility.ps1 } catch { $message = $_.ToString() }
        if ($null -eq $message -or $message -notmatch 'Unset SMART_DESKTOP_INTEGRATION') {
            throw "Interactive verification was not refused: $message"
        }
    } finally {
        Set-GoProcessEnvironment @{ SMART_DESKTOP_INTEGRATION = $baseline['SMART_DESKTOP_INTEGRATION'] }
    }
    Assert-Restored
    Write-Host 'Build policy and environment restoration checks passed.'
} finally {
    Set-GoProcessEnvironment $callerEnvironment
    Pop-Location
    Remove-Item -LiteralPath $temporary -Recurse -Force
}
