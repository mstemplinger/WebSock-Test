$serverUrl = "ws://85.215.147.108:8765"
$clientId = [guid]::NewGuid().ToString()
$computerName = $env:COMPUTERNAME
$ipAddress = (Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.InterfaceAlias -like "*Ethernet*" -or $_.InterfaceAlias -like "*Wi-Fi*" } | Select-Object -First 1).IPAddress

function Connect-WebSocket {
    param (
        [string]$serverUrl
    )

    $websocket = New-Object System.Net.WebSockets.ClientWebSocket
    $uri = New-Object System.Uri($serverUrl)

    while ($true) {
        try {
            Write-Host "🔄 Versuche Verbindung zum Server..." -ForegroundColor Yellow
            $websocket.ConnectAsync($uri, [System.Threading.CancellationToken]::None).Wait()
            Write-Host "✅ Erfolgreich verbunden!" -ForegroundColor Green

            # 📤 **Client jetzt registrieren, nachdem die Verbindung erfolgreich ist**
            Register-Client -websocket $websocket

            return $websocket  # Erfolgreiche Verbindung wird zurückgegeben
        } catch {
            Write-Host "❌ Verbindung fehlgeschlagen. Neuer Versuch in 5 Sekunden..." -ForegroundColor Red
            Start-Sleep -Seconds 5
        }
    }
}

function Register-Client {
    param (
        [System.Net.WebSockets.ClientWebSocket]$websocket
    )

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
}

function Listen-WebSocket {
    param (
        [System.Net.WebSockets.ClientWebSocket]$websocket
    )

    while ($true) {
        if ($websocket -eq $null -or $websocket.State -ne "Open") {
            Write-Host "⚠️ Verbindung verloren. Versuche eine neue Verbindung..." -ForegroundColor Yellow
            Start-Sleep -Seconds 5
            $websocket = Connect-WebSocket -serverUrl $serverUrl
            continue
        }

        try {
            Write-Host "⏳ Warte auf Nachricht..."
            $receiveBuffer = New-Object byte[] 1024
            $segment = New-Object System.ArraySegment[byte] -ArgumentList (, $receiveBuffer)
            $result = $websocket.ReceiveAsync($segment, [System.Threading.CancellationToken]::None).Result

            if ($result.MessageType -eq "Close") {
                Write-Host "⚠️ Verbindung vom Server geschlossen." -ForegroundColor Yellow
                $websocket.CloseAsync([System.Net.WebSockets.WebSocketCloseStatus]::NormalClosure, "Connection closed", [System.Threading.CancellationToken]::None).Wait()
                Start-Sleep -Seconds 5
                continue
            }

            $message = [System.Text.Encoding]::UTF8.GetString($receiveBuffer, 0, $result.Count)
            Write-Host "📨 Nachricht erhalten: $message" -ForegroundColor Cyan

            if ($message -match "registered") {
                Write-Host "✅ Erfolgreich registriert am Server!" -ForegroundColor Green
            } elseif ($message -eq "STOP") {
                Write-Host "🛑 STOP-Nachricht erhalten. Beende das Skript..." -ForegroundColor Red
                $websocket.CloseAsync([System.Net.WebSockets.WebSocketCloseStatus]::NormalClosure, "STOP received", [System.Threading.CancellationToken]::None).Wait()
                
                if ($psISE) {
                    Write-Host "⚠️ Skript läuft in PowerShell ISE. Schließe das ISE-Fenster nicht automatisch." -ForegroundColor Yellow
                    exit
                } else {
                    Write-Host "💨 Skript wird jetzt geschlossen..." -ForegroundColor Red
                    Stop-Process -Id $PID  # Beendet das PowerShell-Fenster
                }
            } else {
                Start-Process -FilePath "msg.exe" -ArgumentList "* $message"
            }
        } catch {
            Write-Host "❌ Fehler beim Empfangen der Nachricht. Verbindung wird beendet. Versuche erneut zu verbinden..." -ForegroundColor Red
            Start-Sleep -Seconds 5
        }
    }
}

# Starte den WebSocket-Client mit endlosem Wiederverbindungsversuch
$websocket = Connect-WebSocket -serverUrl $serverUrl
Listen-WebSocket -websocket $websocket
