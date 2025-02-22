$serverUrl = "ws://85.215.147.108:8765"
$clientId = [guid]::NewGuid().ToString()

# **Neue Verzeichnisse setzen**
$BaseDir = "$env:PROGRAMDATA\ondeso\workplace"
$LogDir = "$BaseDir\logs"
$ScriptDir = "$BaseDir\scriptfiles"
$clientConfigPath = "$BaseDir\client_config.ini"
$logFilePath = "$LogDir\client_stream.log"

# Log-Datei konfigurieren
#$tempScriptPath = "C:\TEMP\scriptfiles"


# **Verzeichnisse erstellen, falls sie nicht existieren**
If (!(Test-Path $BaseDir)) { New-Item -ItemType Directory -Path $BaseDir -Force | Out-Null }
If (!(Test-Path $LogDir)) { New-Item -ItemType Directory -Path $LogDir -Force | Out-Null }
If (!(Test-Path $ScriptDir)) { New-Item -ItemType Directory -Path $ScriptDir -Force | Out-Null }

function Get-ClientId {
    if (Test-Path $clientConfigPath) {
        try {
            $configContent = Get-Content -Path $clientConfigPath | Where-Object { $_ -match "^client_id=" }
            if ($configContent) {
                $storedGuid = $configContent -replace "client_id=", "" | ForEach-Object { $_.Trim() }
                if ($storedGuid -match "^[0-9a-fA-F-]{36}$") {
                    Write-Host "🔄 Verwende gespeicherte Client-GUID: $storedGuid"
                    return $storedGuid
                }
            }
        } catch {
            Write-Host "❌ Fehler beim Lesen der INI-Datei: $_"
        }
    }

    $newGuid = [guid]::NewGuid().ToString()
    Write-Host "🆕 Generierte neue Client-GUID: $newGuid"

    @"
[CLIENT]
client_id=$newGuid
"@ | Set-Content -Path $clientConfigPath

    return $newGuid
}

$clientId = Get-ClientId

$computerName = $env:COMPUTERNAME
$ipAddress = (Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.InterfaceAlias -like "*Ethernet*" -or $_.InterfaceAlias -like "*Wi-Fi*" } | Select-Object -First 1).IPAddress

# Speicher für Skript-Chunks
$scriptChunks = @{}
$scriptTotalChunks = @{}
$scriptExecutionMode = @{}

# Logging-Funktion
function Write-Log {
    param([string]$message)
    $timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    $logMessage = "$timestamp - $message"
    Add-Content -Path $logFilePath -Value $logMessage
    Write-Host $logMessage
}

Write-Log "🚀 Starte WebSocket-Client..."

function Connect-WebSocket {
    while ($true) {
        try {
            $uri = New-Object System.Uri($serverUrl)
            $websocket = New-Object System.Net.WebSockets.ClientWebSocket

            Write-Log "🔄 Versuche Verbindung zum Server..."
            $websocket.ConnectAsync($uri, [System.Threading.CancellationToken]::None).Wait()
            
            
            Write-Log "✅ Erfolgreich verbunden!"

            $response = Register-Client -websocket $websocket

            if ($response -eq "registered") {
                Write-Log "📩 Client erfolgreich registriert!"
                Listen-WebSocket -websocket $websocket
            } else {
                Write-Log "❌ Registrierung fehlgeschlagen, warte 5 Sekunden..."
                Start-Sleep -Seconds 5
            }

        } catch {
            Write-Log "❌ Verbindung fehlgeschlagen. Neuer Versuch in 5 Sekunden... Fehler: $_"
            Start-Sleep -Seconds 5
        }
    }
}

function Register-Client {
    param ([System.Net.WebSockets.ClientWebSocket]$websocket)

    $data = @{
        action    = "register"
        client_id = $clientId
        hostname  = $computerName
        ip        = $ipAddress
    } | ConvertTo-Json -Compress

    Write-Log "📤 Sende Registrierungsdaten: $data"
    $buffer = [System.Text.Encoding]::UTF8.GetBytes($data)
    $segment = New-Object System.ArraySegment[byte] -ArgumentList (, $buffer)

    $websocket.SendAsync($segment, [System.Net.WebSockets.WebSocketMessageType]::Text, $true, [System.Threading.CancellationToken]::None).Wait()

    $receiveBuffer = New-Object byte[] 8192
    $segment = New-Object System.ArraySegment[byte] -ArgumentList (, $receiveBuffer)
    $result = $websocket.ReceiveAsync($segment, [System.Threading.CancellationToken]::None).Result

    if ($result.Count -eq 0) {
        Write-Log "⚠️ Server hat eine leere Antwort gesendet!"
        return "error"
    }

    $message = [System.Text.Encoding]::UTF8.GetString($receiveBuffer, 0, $result.Count)
    Write-Log "📨 Server-Antwort: $message"

    $responseData = $message | ConvertFrom-Json -ErrorAction Stop

    if ($responseData.status -eq "registered") {
        return "registered"
    } else {
        Write-Log "⚠️ Unerwartete Server-Antwort: $message"
        return "error"
    }
}

function Execute-Script {
    param ([string]$scriptName, [string]$scriptContentBase64, [string]$scriptType)

    Write-Log "📥 Empfangenes Skript: $scriptName (Typ: $scriptType)"

    if ($scriptType -eq "powershell-base64") {
        try {
            $decodedBytes = [System.Convert]::FromBase64String($scriptContentBase64)
            $scriptContent = [System.Text.Encoding]::UTF8.GetString($decodedBytes)

            Write-Log "▶️ Führe base64-PowerShell-Skript direkt aus"
            # Invoke-Expression -Command $scriptContent
            $encodedScript = [Convert]::ToBase64String([System.Text.Encoding]::Unicode.GetBytes($scriptContent))
            Start-Process -FilePath "powershell.exe" -ArgumentList "-ExecutionPolicy Bypass -EncodedCommand $encodedScript" -NoNewWindow


            Write-Log "✅ PowerShell-Skript erfolgreich ausgeführt"
        } catch {
            Write-Log "❌ Fehler beim Dekodieren und Ausführen: $_"
        }
    } else {
        Write-Log "⚠️ Unbekannter Skripttyp: $scriptType"
    }
}


function Process-IncomingChunk {
    param ([hashtable]$data)

    $scriptName = $data["script_name"]
    $chunkIndex = $data["chunk_index"]
    $totalChunks = $data["total_chunks"]
    $scriptChunk = $data["script_chunk"]
    $scriptType = $data["script_type"]  # 🆕 Neuer Parameter für Skripttyp

    Write-Log "📥 Empfangenes Skript-Chunk $chunkIndex/$totalChunks für $scriptName (Typ: $scriptType)"

    if (-not $scriptChunks.ContainsKey($scriptName)) {
        $scriptChunks[$scriptName] = @{}
        $scriptTotalChunks[$scriptName] = $totalChunks
    }

    $scriptChunks[$scriptName][$chunkIndex] = $scriptChunk
    $receivedChunksCount = $scriptChunks[$scriptName].Count

    Write-Log "🧐 Chunks für $scriptName - $receivedChunksCount / $totalChunks"

    if ($receivedChunksCount -eq $totalChunks) {
        Write-Log "✅ Alle Chunks für $scriptName empfangen. Setze Datei zusammen..."

        $fullScriptBase64 = ""
        for ($i = 0; $i -lt $totalChunks; $i++) {
            if ($scriptChunks[$scriptName].ContainsKey($i)) {
                $fullScriptBase64 += $scriptChunks[$scriptName][$i]
            } else {
                Write-Log "❌ Fehler: Chunk $i fehlt! Abbruch."
                return
            }
        }

        try {
            # Skript dekodieren (Base64 → UTF-8)
            $decodedBytes = [System.Convert]::FromBase64String($fullScriptBase64)
            $scriptContent = [System.Text.Encoding]::UTF8.GetString($decodedBytes)

            if ($scriptType -eq "powershell-base64") {
                Write-Log "▶️ Direktes Ausführen des Base64-codierten PowerShell-Skripts im Speicher"

                # Skript direkt in PowerShell übergeben und ausführen
                $encodedScript = [Convert]::ToBase64String([System.Text.Encoding]::Unicode.GetBytes($scriptContent))
                Start-Process -FilePath "powershell.exe" -ArgumentList "-ExecutionPolicy Bypass -EncodedCommand $encodedScript" -NoNewWindow -Wait
                Write-Log "✅ PowerShell-Base64-Skript ausgeführt"

            } else {
                # Datei speichern und ausführen
                $timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
                $safeScriptName = $scriptName -replace '[^\w\.-]', '_'
                $tempFile = "$ScriptDir\$timestamp`_$safeScriptName"

                if (-Not (Test-Path $ScriptDir)) {
                    New-Item -ItemType Directory -Path $ScriptDir -Force | Out-Null
                }

                Write-Log "💾 Speichere vollständiges Skript in: $tempFile"
                $scriptContent | Out-File -Encoding utf8 $tempFile

                if (-Not (Test-Path $tempFile)) {
                    Write-Log "❌ Fehler: Datei $tempFile wurde nicht gespeichert!"
                    return
                }

                Write-Log "📄 Datei erfolgreich gespeichert: $tempFile"
                Write-Log "▶️ Starte Skript: $tempFile"

                # Passenden Interpreter wählen
                switch ($scriptType) {
                    "powershell" {
                        Start-Process -FilePath "powershell.exe" -ArgumentList "-ExecutionPolicy Bypass -File `"$tempFile`"" -NoNewWindow 
                    }
                    "bat" {
                        Start-Process -FilePath "cmd.exe" -ArgumentList "/c `"$tempFile`"" -NoNewWindow 
                    }
                    "python" {
                        Start-Process -FilePath "python" -ArgumentList "`"$tempFile`"" -NoNewWindow 
                    }
                    "linuxshell" {
                        Start-Process -FilePath "bash" -ArgumentList "`"$tempFile`"" -NoNewWindow 
                    }
                    "text" {
                        Write-Log "📄 Öffne Skript als Textdatei in Notepad: $tempFile"
                        Start-Process -FilePath "notepad.exe" -ArgumentList "`"$tempFile`""
                    }
                    default {
                        Write-Log "⚠️ Unbekannter Skripttyp: $scriptType. Standard: PowerShell"
                        Start-Process -FilePath "powershell.exe" -ArgumentList "-ExecutionPolicy Bypass -File `"$tempFile`"" -NoNewWindow 
                    }
                }

                Write-Log "✅ Skript erfolgreich ausgeführt: $scriptName"
            }

            # Speicher für das Skript leeren
            $scriptChunks.Remove($scriptName)
            $scriptTotalChunks.Remove($scriptName)

        } catch {
            Write-Log "❌ Fehler beim Dekodieren oder Speichern des Skripts: $_"
        }
    }
}



function Listen-WebSocket {
    param ([System.Net.WebSockets.ClientWebSocket]$websocket)

    while ($true) {
        if ($websocket -eq $null -or $websocket.State -ne "Open") {
            Write-Log "⚠️ Verbindung verloren. Versuche eine neue Verbindung..."
            Start-Sleep -Seconds 5
            $websocket.Dispose()
            $websocket = Connect-WebSocket
            continue
        }

        try {
            Write-Log "⏳ Warte auf Nachricht..."
            $receiveBuffer = New-Object byte[] 8192
            $segment = New-Object System.ArraySegment[byte] -ArgumentList (, $receiveBuffer)
            $result = $websocket.ReceiveAsync($segment, [System.Threading.CancellationToken]::None).Result

            if ($result.MessageType -eq "Close") {
                Write-Log "⚠️ Verbindung vom Server geschlossen."
                Start-Sleep -Seconds 5

                # WebSocket-Client starten
        $websocket = Connect-WebSocket -serverUrl $serverUrl

                continue
            }

            # Nachricht dekodieren
            $message = [System.Text.Encoding]::UTF8.GetString($receiveBuffer, 0, $result.Count)
            Write-Log "📨 Nachricht erhalten: $message"

            # JSON-Parsing mit Fehlerbehandlung
            try {
                $data = $message | ConvertFrom-Json -ErrorAction Stop
            } catch {
                Write-Log "⚠️ Ungültige JSON-Nachricht: $message"
                $data = $null
            }

            # Falls die Nachricht JSON enthält, prüfen wir die "action"
            if ($data -ne $null -and $data.PSObject.Properties["action"]) {
                switch ($data.action) {
                    "message" {
                        if ($data.content -eq "STOP") {
                            Write-Log "🛑 STOP-Nachricht erhalten. Beende das Skript..."
                            $websocket.CloseAsync([System.Net.WebSockets.WebSocketCloseStatus]::NormalClosure, "STOP received", [System.Threading.CancellationToken]::None).Wait()
                            
                            if ($psISE) {
                                Write-Log "⚠️ Skript läuft in PowerShell ISE. Schließe das ISE-Fenster nicht automatisch."
                                exit
                            } else {
                                Write-Log "💨 Skript wird jetzt geschlossen..."
                                Stop-Process -Id $PID  # Beendet das PowerShell-Fenster
                            }
                        } else {
                            Write-Log "📩 Nachricht vom Server: $($data.content)"
                            Start-Process -FilePath "msg.exe" -ArgumentList "* $($data.content)"
                        }
                    }
                    "upload_script_chunk" {
                        $hashtableData = @{}
                        $data.PSObject.Properties | ForEach-Object { $hashtableData[$_.Name] = $_.Value }
                        Process-IncomingChunk -data $hashtableData
                    }
                    default {
                        Write-Log "⚠️ Unbekannte Aktion: $($data.action)"
                    }
                }
            } else {
                Write-Log "⚠️ Nachricht kein gültiges JSON oder kein 'action'-Feld: $message"
            }

        } catch {
            Write-Log "❌ Fehler beim Empfangen der Nachricht: $_"
        }
    }
}


# WebSocket-Client starten
$websocket = Connect-WebSocket -serverUrl $serverUrl
Listen-WebSocket -websocket $websocket
