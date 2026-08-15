param(
    [string]$Room = "{{.Room}}",
    [string]$ApiUrl = "{{.ApiURL}}"
)

$arch = $env:PROCESSOR_ARCHITECTURE

if ($env:PROCESSOR_ARCHITEW6432) {
    $arch = $env:PROCESSOR_ARCHITEW6432
}

switch ($arch) {

    "AMD64" {
        $binary = "termchat-windows-amd64.exe"
    }

    "x86" {
        $binary = "termchat-windows-amd64.exe"
    }

    "ARM64" {
        $binary = "termchat-windows-arm64.exe"
    }

    default {
        Write-Host "Unsupported architecture: $arch"
        exit
    }
}

$cacheDir = "$env:LOCALAPPDATA\termchat"

New-Item `
    -ItemType Directory `
    -Force `
    -Path $cacheDir | Out-Null

$binaryPath = "$cacheDir\termchat.exe"
$versionFile = "$cacheDir\version.txt"

$needsDownload = $true

if ((Test-Path $binaryPath) -and (Test-Path $versionFile)) {

    $currentVersion = Get-Content $versionFile

    if ($currentVersion -eq "{{.Version}}") {
        $needsDownload = $false
    }
}

if ($needsDownload) {

    Write-Host "Downloading $binary..."

    Invoke-WebRequest `
        -Uri "$ApiUrl/bin/$binary" `
        -OutFile $binaryPath

    Set-Content `
        -Path $versionFile `
        -Value "{{.Version}}"
} else {
    Write-Host "Using cached $binary..."
}

Write-Host "Launching room $Room..."

Start-Process `
    -FilePath $binaryPath `
    -ArgumentList "$Room" `
    -NoNewWindow `
    -Wait
