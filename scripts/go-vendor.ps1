function Set-GoProcessEnvironment {
    param(
        [Parameter(Mandatory = $true)]
        [hashtable] $Values
    )

    foreach ($name in $Values.Keys) {
        if ($null -eq $Values[$name]) {
            $path = "Env:\$name"
            if (Test-Path -LiteralPath $path) {
                Remove-Item -LiteralPath $path
            }
        } else {
            [Environment]::SetEnvironmentVariable($name, $Values[$name], 'Process')
        }
    }
}

function Invoke-VendoredGo {
    param(
        [Parameter(Mandatory = $true)]
        [scriptblock] $Action
    )

    $settings = @{
        GOENV = 'off'
        GOTOOLCHAIN = 'local'
        GOWORK = 'off'
        GOPROXY = 'off'
        GOSUMDB = 'off'
        GOFLAGS = '-mod=vendor -trimpath -buildvcs=false'
        GOOS = 'windows'
        GOARCH = 'amd64'
        GOAMD64 = 'v1'
        GOEXPERIMENT = $null
        CGO_ENABLED = '0'
    }
    $previous = @{}
    foreach ($name in $settings.Keys) {
        $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
    }

    Push-Location (Join-Path $PSScriptRoot '..')
    try {
        Set-GoProcessEnvironment $settings
        $directive = @(Get-Content -LiteralPath .\go.mod | Where-Object { $_ -match '^go \d+\.\d+\.\d+$' })
        if ($directive.Count -ne 1) {
            throw 'go.mod must specify exactly one Go version including its patch number.'
        }
        $expected = 'go' + ($directive[0] -replace '^go ', '')
        $actual = go env GOVERSION
        if ($LASTEXITCODE -ne 0) { throw 'Unable to determine the installed Go version.' }
        if ($actual -ne $expected) {
            throw "Go $expected is required; installed version is $actual. Install the exact SDK before building."
        }
        if (-not (Test-Path -LiteralPath .\vendor\modules.txt -PathType Leaf)) {
            throw 'vendor\modules.txt is missing. Restore the committed vendor directory; no module download is permitted.'
        }
        & $Action
    } finally {
        Set-GoProcessEnvironment $previous
        Pop-Location
    }
}
