# PowerShell-Skript zur Erfassung von Benutzerinformationen und API-Upload

# Verzeichnis für temporäre Speicherung
$directoryPath = "$env:TEMP"
$jsonFile = "$directoryPath\usr_user_info.json"

# Lokalen Rechnernamen abrufen
$clientName = $env:COMPUTERNAME

# Generiere eine eindeutige Vorgangsnummer (GUID)
$transactionID = [guid]::NewGuid().ToString()

# Benutzerinformationen abrufen
# $userList = Get-WmiObject Win32_UserAccount | Where-Object { $_.LocalAccount -eq $true }
$userList = Get-WmiObject Win32_UserAccount | Where-Object { $_.LocalAccount -eq $true }


# Anzahl der Benutzer
$userCount = $userList.Count

# Alle lokalen Gruppen abrufen
$localGroups = Get-LocalGroup | Select-Object -ExpandProperty Name

# JSON-Datenstruktur erstellen
$jsonData = @{
    MetaData = @{
        ContentType  = "db-import"
        Name         = "Windows User Import"
        Description  = "Collect User Information"
        Version      = "1.0"
        Creator      = "FL"
        Vendor       = "ondeso GmbH"
        Preview      = ""
        Schema       = ""
    }
    Content = @{
        TableName = "usr_client_users"
        Mappings  = @(
            @{ TargetField = "id"; Expression = "NewGUID()" }
            @{ TargetField = "transaction_id"; Expression = "{TransactionID}" }
            @{ TargetField = "username"; Expression = "{UserName}" }
            @{ TargetField = "client"; Expression = "{ClientName}" }
            @{ TargetField = "usercount"; Expression = "{UserCount}" }
            @{ TargetField = "permissions"; Expression = "{Permissions}" }
            @{ TargetField = "sid"; Expression = "{SID}" }
            @{ TargetField = "full_name"; Expression = "{FullName}" }
            @{ TargetField = "account_status"; Expression = "{AccountStatus}" }
            @{ TargetField = "last_logon"; Expression = "{LastLogon}" }
            @{ TargetField = "description"; Expression = "{Description}" }
        )
        Fields = @(
            @{ Field = "TransactionID"; Type = "string" }
            @{ Field = "UserName"; Type = "string" }
            @{ Field = "ClientName"; Type = "string" }
            @{ Field = "UserCount"; Type = "string" }
            @{ Field = "Permissions"; Type = "string" }
            @{ Field = "SID"; Type = "string" }
            @{ Field = "FullName"; Type = "string" }
            @{ Field = "AccountStatus"; Type = "string" }
            @{ Field = "LastLogon"; Type = "datetime" }
            @{ Field = "Description"; Type = "string" }
        )
        Data = @(
            foreach ($user in $userList) {
                # Alle Gruppenmitgliedschaften für den Benutzer abrufen
                $userGroups = @()
                foreach ($group in $localGroups) {
                    try {
                        $members = Get-LocalGroupMember -Group $group -ErrorAction SilentlyContinue
                        if ($members) {
                            foreach ($member in $members) {
                                if ($member.Name -match $user.Name) {
                                    $userGroups += $group
                                }
                            }
                        }
                    } catch {
                        # Fehler ignorieren, falls eine Gruppe nicht ausgelesen werden kann
                    }
                }

                # Gruppen als CSV speichern (falls keine Gruppen, bleibt es leer)
                $permissions = $userGroups -join ","

                # Letzter Logon-Zeitstempel abrufen mit Get-WinEvent
                $lastLogonEvent = Get-WinEvent -LogName Security -FilterXPath "*[System[EventID=4624]]" -MaxEvents 1 -ErrorAction SilentlyContinue | Select-Object -ExpandProperty TimeCreated
                $lastLogonFormatted = if ($lastLogonEvent) { $lastLogonEvent.ToString("yyyy-MM-dd HH:mm:ss") } else { $null }

                @{
                    TransactionID = $transactionID
                    UserName      = $user.Name
                    ClientName    = $clientName
                    UserCount     = "$userCount"
                    Permissions   = $permissions  # 🔄 Hier stehen jetzt die Gruppenmitgliedschaften
                    SID           = $user.SID
                    FullName      = $user.FullName
                    AccountStatus = if ($user.Disabled) { "Disabled" } else { "Active" }
                    LastLogon     = $lastLogonFormatted  # Korrigiertes Datumsformat
                    Description   = $user.Description
                }
            }
        )
    }
} | ConvertTo-Json -Depth 3

# JSON-Datei speichern
$jsonData | Set-Content -Path $jsonFile -Encoding UTF8

Write-Host "✅ JSON-Datei gespeichert: $jsonFile"

# API-Endpunkt für den Upload auf die Server-IP 85.215.147.108 mit Port 5001
$apiEndpoint = "http://85.215.147.108:5001/inbox"

# JSON an API senden
try {
    $response = Invoke-RestMethod -Uri $apiEndpoint -Method Post -ContentType "application/json" -InFile $jsonFile
    Write-Host "✅ API-Antwort: $response"
} catch {
    Write-Host "❌ Fehler beim Senden an API: $_"
}
