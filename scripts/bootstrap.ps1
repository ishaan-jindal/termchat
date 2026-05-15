param(
    [string]$Room = "{{.Room}}",
    [string]$ApiUrl = "{{.ApiURL}}",
    [string]$WsUrl = "{{.WsURL}}"
)

$arch = $env:PROCESSOR_ARCHITECTURE

if ($arch -eq "AMD64") {
    $binary = "termchat-windows-amd64.exe"
}
elseif ($arch -eq "ARM64") {
    $binary = "termchat-windows-arm64.exe"
}
else {
    Write-Host "Unsupported architecture"
    exit
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
