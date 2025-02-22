# **Setze Hauptverzeichnis für alle Dateien**
$BaseDir = "$env:PROGRAMDATA\ondeso\workplace"
$UserDir = "$BaseDir\users"
$LogDir = "$BaseDir\logs"
$JsonFile = "$UserDir\usr_user_info.json"
$ClientConfigPath = "$BaseDir\client_config.ini"
$LogFilePath = "$LogDir\user_info.log"

# ✅ Erstelle Verzeichnisse, falls sie nicht existieren
$Folders = @($BaseDir, $UserDir, $LogDir)
ForEach ($Folder in $Folders) {
    If (!(Test-Path $Folder)) {
        New-Item -ItemType Directory -Path $Folder -Force | Out-Null
    }
}

# ✅ Logging-Funktion
function Write-Log {
    param([string]$message)
    $timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    $logMessage = "$timestamp - $message"
    Add-Content -Path $LogFilePath -Value $logMessage
    Write-Host $logMessage
}

Write-Log "🚀 Starte Benutzer-Info-Erfassung mit Basisverzeichnis: $BaseDir"

# ✅ Funktion zum Lesen der Client-GUID aus der INI-Datei
function Get-ClientId {
    if (Test-Path $ClientConfigPath) {
        try {
            $configContent = Get-Content -Path $ClientConfigPath | Where-Object { $_ -match "^client_id=" }
            if ($configContent) {
                $storedGuid = $configContent -replace "client_id=", "" | ForEach-Object { $_.Trim() }
                if ($storedGuid -match "^[0-9a-fA-F-]{36}$") {
                    Write-Log "🔄 Verwende gespeicherte Client-GUID: $storedGuid"
                    return $storedGuid
                }
            }
        } catch {
            Write-Log "❌ Fehler beim Lesen der INI-Datei: $_"
        }
    }

    # ✅ Falls keine gültige GUID gefunden wird, neue GUID generieren
    $newGuid = [guid]::NewGuid().ToString()
    Write-Log "🆕 Generierte neue Client-GUID: $newGuid"

    # ✅ Speichere die GUID in der INI-Datei mit [CLIENT]-Sektion
    @"
[CLIENT]
client_id=$newGuid
"@ | Set-Content -Path $ClientConfigPath

    return $newGuid
}

# ✅ Lese die `client_id` als `asset_id` aus der INI
$AssetId = Get-ClientId
Write-Log "📌 Asset-ID (Client GUID): $AssetId"

# ✅ Lokalen Rechnernamen abrufen
$ClientName = $env:COMPUTERNAME

# ✅ Eindeutige Vorgangsnummer (GUID)
$TransactionID = [guid]::NewGuid().ToString()

# ✅ Lokale Benutzer abrufen
$UserList = Get-LocalUser
$UserCount = $UserList.Count

# ✅ Alle lokalen Gruppen abrufen
$LocalGroups = Get-LocalGroup | Select-Object -ExpandProperty Name

# ✅ Benutzerdaten sammeln
$UserData = @()

foreach ($User in $UserList) {
    # ✅ Eindeutige Benutzer-ID generieren
    $UserID = [guid]::NewGuid().ToString()

    # ✅ Gruppenmitgliedschaften effizient abrufen
    $UserGroups = @()
    try {
        $UserGroups = Get-LocalGroupMember -Group (Get-LocalGroup) | Where-Object { $_.Name -match $User.Name } | Select-Object -ExpandProperty Name -ErrorAction SilentlyContinue
    } catch {
        Write-Log "⚠️ Fehler beim Abrufen der Gruppen für Benutzer $($User.Name): $_"
    }

    # ✅ Letzter Logon-Zeitstempel abrufen
    $LastLogonEvent = Get-WinEvent -LogName Security -FilterXPath "*[System[EventID=4624]]" -MaxEvents 1 -ErrorAction SilentlyContinue | Select-Object -ExpandProperty TimeCreated
    $LastLogonFormatted = if ($LastLogonEvent) { $LastLogonEvent.ToString("yyyy-MM-dd HH:mm:ss") } else { $null }

    # ✅ Benutzerinformationen speichern
    $UserData += [PSCustomObject]@{
        id            = $UserID
        transaction_id = $TransactionID
        asset_id      = $AssetId
        username      = $User.Name
        client        = $ClientName
        usercount     = "$UserCount"
        permissions   = $UserGroups -join ","
        sid           = $User.SID.Value
        full_name     = $User.Description
        account_status = if ($User.Enabled) { "Active" } else { "Disabled" }
        last_logon     = $LastLogonFormatted
        description   = $User.Description
    }
}

# ✅ JSON-Datenstruktur erstellen
$JsonData = [PSCustomObject]@{
    MetaData = [PSCustomObject]@{
        ContentType  = "db-import"
        Name         = "Windows User Import"
        Description  = "Collect User Information"
        Version      = "1.0"
        Creator      = "ondeso"
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
            [PSCustomObject]@{ TargetField = "transaction_id"; Expression = "{transaction_id}"; IsIdentifier = $true; ImportField = $true }
            [PSCustomObject]@{ TargetField = "asset_id"; Expression = "{asset_id}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "username"; Expression = "{username}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "client"; Expression = "{client}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "usercount"; Expression = "{usercount}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "permissions"; Expression = "{permissions}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "sid"; Expression = "{sid}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "full_name"; Expression = "{full_name}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "account_status"; Expression = "{account_status}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "last_logon"; Expression = "{last_logon}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "description"; Expression = "{description}"; IsIdentifier = $false; ImportField = $true }
        )
        Data = $UserData
    }
}

# ✅ JSON-Datei speichern
$JsonData | ConvertTo-Json -Depth 10 | Set-Content -Path $JsonFile -Encoding UTF8
Write-Log "✅ JSON-Datei gespeichert: $JsonFile"

# ✅ API-Endpunkt für den Upload
$ApiEndpoint = "http://85.215.147.108:5001/inbox"

# ✅ JSON an API senden
try {
    Invoke-RestMethod -Uri $ApiEndpoint -Method Post -ContentType "application/json" -InFile $JsonFile
    Write-Log "✅ API-Upload abgeschlossen!"
} catch {
    Write-Log "❌ Fehler beim Hochladen der JSON-Datei: $_"
}
