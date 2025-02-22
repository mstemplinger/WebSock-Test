$serviceName = "WebSocketClientService"
$serviceExePath = "C:\Program Files\MyService\client_script.ps1"

# Prüfen, ob Dienst existiert
if (Get-Service -Name $serviceName -ErrorAction SilentlyContinue) {
    Write-Host "⚠️ Dienst existiert bereits! Stoppe und lösche ihn..."
    Stop-Service -Name $serviceName -Force
    sc.exe delete $serviceName
    Start-Sleep -Seconds 3
}

Write-Host "📌 Erstelle neuen Dienst: $serviceName"
New-Service -Name $serviceName -BinaryPathName "powershell.exe -WindowStyle Hidden -ExecutionPolicy Bypass -File $serviceExePath" -DisplayName "WebSocket Client Service" -Description "Dieser Dienst hält die WebSocket-Verbindung zum Server aufrecht." -StartupType Automatic

Write-Host "✅ Dienst erfolgreich erstellt!"
