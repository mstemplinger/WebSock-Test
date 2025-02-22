# **Verzeichnis für temporäre Speicherung**
$directoryPath = "$env:TEMP"
$jsonFile = "$directoryPath\usr_user_info.json"
$clientConfigPath = "C:\TEMP\client_config.ini"

# **Funktion zum Lesen der Client-GUID aus der INI-Datei**
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
    
    # Falls keine gültige GUID gefunden wird, neue GUID generieren
    $newGuid = [guid]::NewGuid().ToString()
    Write-Host "🆕 Generierte neue Client-GUID: $newGuid"

    # Speichere die GUID in der INI-Datei mit [CLIENT]-Sektion
    @"
[CLIENT]
client_id=$newGuid
"@ | Set-Content -Path $clientConfigPath

    return $newGuid
}

# **Lese die `client_id` als `asset_id` aus der INI**
$assetId = Get-ClientId

# **Lokalen Rechnernamen abrufen**
$clientName = $env:COMPUTERNAME

# **Eindeutige Vorgangsnummer (GUID)**
$transactionID = [guid]::NewGuid().ToString()

# **Lokale Benutzer abrufen**
$userList = Get-LocalUser

# **Anzahl der Benutzer**
$userCount = $userList.Count

# **Alle lokalen Gruppen abrufen**
$localGroups = Get-LocalGroup | Select-Object -ExpandProperty Name

# **Benutzerdaten sammeln**
$userData = @()
foreach ($user in $userList) {
    # **Eindeutige Benutzer-ID generieren**
    $userID = [guid]::NewGuid().ToString()

    # Gruppenmitgliedschaften abrufen
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
            # Fehler ignorieren
        }
    }

    # Letzter Logon-Zeitstempel abrufen mit Get-WinEvent
    $lastLogonEvent = Get-WinEvent -LogName Security -FilterXPath "*[System[EventID=4624]]" -MaxEvents 1 -ErrorAction SilentlyContinue | Select-Object -ExpandProperty TimeCreated
    $lastLogonFormatted = if ($lastLogonEvent) { $lastLogonEvent.ToString("yyyy-MM-dd HH:mm:ss") } else { $null }

    # **Daten in ein korrektes JSON-Format bringen**
    $userData += [PSCustomObject]@{
        id            = $userID  # **Neue eindeutige Benutzer-ID**
        transaction_id = $transactionID  # **Vorgangsnummer**
        asset_id      = $assetId  # **Setze die client_id als asset_id**
        UserName      = $user.Name
        ClientName    = $clientName
        UserCount     = "$userCount"
        Permissions   = $userGroups -join ","
        SID           = $user.SID.Value  # **Fix für SID**
        FullName      = $user.Description
        AccountStatus = if ($user.Enabled) { "Active" } else { "Disabled" }
        LastLogon     = $lastLogonFormatted
        Description   = $user.Description
    }
}

# **JSON-Datenstruktur erstellen**
$jsonData = [PSCustomObject]@{
    MetaData = [PSCustomObject]@{
        ContentType  = "db-import"
        Name         = "Windows User Import"
        Description  = "Collect User Information"
        Version      = "1.0"
        Creator      = "FL"
        Vendor       = "ondeso GmbH"
        Preview      = ""
        Schema       = ""
    }
    Content = [PSCustomObject]@{
        TableName = "usr_client_users"
        Consts = @(
            [PSCustomObject]@{
                Identifier = "CaptureDate"
                Value      = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
            }
        )
        FieldMappings = @(
            [PSCustomObject]@{ TargetField = "id"; Expression = "{id}"; IsIdentifier = $true; ImportField = $true }
            [PSCustomObject]@{ TargetField = "transaction_id"; Expression = "{transaction_id}"; IsIdentifier = $true; ImportField = $true }
            [PSCustomObject]@{ TargetField = "asset_id"; Expression = "{asset_id}"; IsIdentifier = $false; ImportField = $true }  # **Neues Feld für asset_id**
            [PSCustomObject]@{ TargetField = "username"; Expression = "{UserName}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "client"; Expression = "{ClientName}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "usercount"; Expression = "{UserCount}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "permissions"; Expression = "{Permissions}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "sid"; Expression = "{SID}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "full_name"; Expression = "{FullName}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "account_status"; Expression = "{AccountStatus}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "last_logon"; Expression = "{LastLogon}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "description"; Expression = "{Description}"; IsIdentifier = $false; ImportField = $true }
        )
        Data = $userData
    }
}

# **JSON in Datei speichern**
$jsonData | ConvertTo-Json -Depth 10 | Set-Content -Path $jsonFile -Encoding UTF8

Write-Host "✅ JSON-Datei gespeichert: $jsonFile"

# **API-Endpunkt für den Upload**
$apiEndpoint = "http://85.215.147.108:5001/inbox"

# **JSON an API senden**
try {
    $response = Invoke-RestMethod -Uri $apiEndpoint -Method Post -ContentType "application/json" -InFile $jsonFile
    Write-Host "✅ API-Antwort: $response"
} catch {
    Write-Host "❌ Fehler beim Senden an API: $_"
}
