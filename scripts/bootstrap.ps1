param(
    [string]$Room = "{{.Room}}",
    [string]$ApiUrl = "{{.ApiURL}}",
    [string]$WsUrl = "{{.WsURL}}"
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

$temp = "$env:TEMP\termchat.exe"

Write-Host "Downloading $binary..."

Invoke-WebRequest `
    -Uri "$ApiUrl/bin/$binary" `
    -OutFile $temp

Write-Host "Launching room $Room..."

Start-Process `
    -FilePath $temp `
    -ArgumentList "--room $Room --server $WsUrl" `
    -NoNewWindow `
    -Wait
