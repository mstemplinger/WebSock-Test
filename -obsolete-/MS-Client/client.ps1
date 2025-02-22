$serverUrl = "ws://localhost:8765"
$clientId = [guid]::NewGuid().ToString()
$computerName = $env:COMPUTERNAME
$ipAddress = (Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.InterfaceAlias -like "*Ethernet*" -or $_.InterfaceAlias -like "*Wi-Fi*" } | Select-Object -First 1).IPAddress

# WebSocket-Client initialisieren
$websocket = New-Object System.Net.WebSockets.ClientWebSocket
$uri = New-Object System.Uri($serverUrl)

# Verbindung mit Wiederholungslogik
$maxRetries = 5
$retryCount = 0
$connected = $false

while (-not $connected -and $retryCount -lt $maxRetries) {
    try {
        Write-Host "🔄 Versuche Verbindung zum Server..." -ForegroundColor Yellow
        $websocket.ConnectAsync($uri, [System.Threading.CancellationToken]::None).Wait()
        $connected = $true
        Write-Host "✅ Erfolgreich verbunden!" -ForegroundColor Green
    } catch {
        Write-Host "❌ Verbindung fehlgeschlagen, neuer Versuch in 5 Sekunden..." -ForegroundColor Red
        Start-Sleep -Seconds 5
        $retryCount++
    }
}

if (-not $connected) {
    Write-Host "🚨 Fehler: Konnte keine Verbindung zum Server herstellen." -ForegroundColor Red
    exit
}

# Registrierungsdaten senden
$data = @{
    action = "register"
    client_id = $clientId
    hostname = $computerName
    ip = $ipAddress
} | ConvertTo-Json -Compress

Write-Host "📤 Sende Registrierungsdaten: $data" -ForegroundColor Cyan
$buffer = [System.Text.Encoding]::UTF8.GetBytes($data)
$segment = New-Object System.ArraySegment[byte] -ArgumentList (, $buffer)

try {
    $websocket.SendAsync($segment, [System.Net.WebSockets.WebSocketMessageType]::Text, $true, [System.Threading.CancellationToken]::None).Wait()
    Write-Host "📩 Registrierung erfolgreich gesendet!" -ForegroundColor Green
} catch {
    Write-Host "❌ Fehler beim Senden der Registrierungsdaten: $_" -ForegroundColor Red
}

# Nachricht empfangen und anzeigen
while ($websocket.State -eq "Open") {
    try {
        Write-Host "⏳ Warte auf Nachricht..."
        $receiveBuffer = New-Object byte[] 1024
        $segment = New-Object System.ArraySegment[byte] -ArgumentList (, $receiveBuffer)
        $result = $websocket.ReceiveAsync($segment, [System.Threading.CancellationToken]::None).Result

        if ($result.MessageType -eq "Close") {
            Write-Host "⚠️ Verbindung vom Server geschlossen." -ForegroundColor Yellow
            $websocket.CloseAsync([System.Net.WebSockets.WebSocketCloseStatus]::NormalClosure, "Connection closed", [System.Threading.CancellationToken]::None).Wait()
            break
        }

        $message = [System.Text.Encoding]::UTF8.GetString($receiveBuffer, 0, $result.Count)
        Write-Host "📨 Nachricht erhalten: $message" -ForegroundColor Cyan

        # Prüfen, ob es eine Registrierungsbestätigung ist
        if ($message -match "registered") {
            Write-Host "✅ Erfolgreich registriert am Server!" -ForegroundColor Green
        } elseif ($message -eq "STOP") {
            Write-Host "🛑 STOP-Nachricht erhalten. Beende das Skript..." -ForegroundColor Red
            $websocket.CloseAsync([System.Net.WebSockets.WebSocketCloseStatus]::NormalClosure, "STOP received", [System.Threading.CancellationToken]::None).Wait()
            
            # Prüfen, ob das Skript in der PowerShell ISE läuft
            if ($psISE) {
                Write-Host "⚠️ Skript läuft in PowerShell ISE. Schließe das ISE-Fenster nicht automatisch." -ForegroundColor Yellow
                exit
            } else {
                Write-Host "💨 Skript wird jetzt geschlossen..." -ForegroundColor Red
                Stop-Process -Id $PID  # Beendet das PowerShell-Fenster
            }
        } else {
            # Falls die Nachricht NICHT "registered" oder "STOP" enthält, wird sie als Popup angezeigt
            Start-Process -FilePath "msg.exe" -ArgumentList "* $message"
        }
    } catch {
        Write-Host "❌ Fehler beim Empfangen der Nachricht. Verbindung wird beendet." -ForegroundColor Red
        break
    }
}

Write-Host "🔌 Skript wurde beendet." -ForegroundColor Cyan
exit
