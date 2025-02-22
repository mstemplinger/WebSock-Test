# **Setze Hauptverzeichnis für alle Dateien**
$BaseDir = "$env:PROGRAMDATA\ondeso\workplace"
$ScanDir = "$BaseDir\scans"
$LogDir = "$BaseDir\logs"
$WsusCabFile = "$ScanDir\wsusscn2.cab"
$JsonFile = "$ScanDir\wsus_scan_result.json"
$ClientConfigPath = "$BaseDir\client_config.ini"
$LogFilePath = "$LogDir\wsus_scan.log"

# **Microsoft-Download-URL für wsusscn2.cab**
$WsusCabUrl = "http://download.windowsupdate.com/microsoftupdate/v6/wsusscan/wsusscn2.cab"

# **API-Endpunkt**
$ApiEndpoint = "http://85.215.147.108:5001/inbox"

# ✅ Erstelle Verzeichnisse, falls sie nicht existieren
$Folders = @($BaseDir, $ScanDir, $LogDir)
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

Write-Log "🚀 Starte WSUS-Scan mit Basisverzeichnis: $BaseDir"

# ✅ Funktion zum Lesen der `client_id` als `asset_id`
function Get-ClientId {
    if (Test-Path $ClientConfigPath) {
        try {
            $configContent = Get-Content -Path $ClientConfigPath | Where-Object { $_ -match "^client_id=" }
            if ($configContent) {
                $storedGuid = $configContent -replace "client_id=", "" | ForEach-Object { $_.Trim() }
                if ($storedGuid -match "^[0-9a-fA-F-]{36}$") {
                    Write-Log "🔄 Verwende gespeicherte Client-GUID als `asset_id`: $storedGuid"
                    return $storedGuid
                }
            }
        } catch {
            Write-Log "❌ Fehler beim Lesen der INI-Datei: $_"
        }
    }

    # ✅ Falls keine gültige GUID vorhanden ist, eine neue generieren
    $newGuid = [guid]::NewGuid().ToString()
    Write-Log "🆕 Generierte neue Client-GUID: $newGuid"

    # ✅ Speichere die GUID in der INI-Datei
    @"
[CLIENT]
client_id=$newGuid
"@ | Set-Content -Path $ClientConfigPath

    return $newGuid
}

# ✅ Lese `client_id` als `asset_id` aus der INI
$AssetId = Get-ClientId

# ✅ Prüfen, ob wsusscn2.cab bereits existiert, sonst herunterladen
If (!(Test-Path $WsusCabFile)) {
    Write-Log "📥 Lade neueste wsusscn2.cab herunter..."
    try {
        Invoke-WebRequest -Uri $WsusCabUrl -OutFile $WsusCabFile
        Write-Log "✅ WSUS-Scan-Datei erfolgreich heruntergeladen: $WsusCabFile"
    } catch {
        Write-Log "❌ Fehler beim Herunterladen der wsusscn2.cab: $_"
        Exit
    }
} else {
    Write-Log "ℹ️ Bereits vorhandene wsusscn2.cab wird verwendet."
}

# ✅ Eindeutige Scan-ID generieren
$ScanID = [guid]::NewGuid().ToString()
$ScanDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

# ✅ Erstelle Update-Session
$UpdateSession = New-Object -ComObject Microsoft.Update.Session
$UpdateServiceManager = New-Object -ComObject Microsoft.Update.ServiceManager
$UpdateService = $UpdateServiceManager.AddScanPackageService("Offline Sync Service", $WsusCabFile)
$UpdateSearcher = $UpdateSession.CreateUpdateSearcher()

Write-Log "🔎 Suche nach WSUS-Updates..."
$UpdateSearcher.ServerSelection = 3
$UpdateSearcher.ServiceID = [string] $UpdateService.ServiceID
$SearchResult = $UpdateSearcher.Search("IsInstalled=1")

# ✅ Falls keine Updates gefunden wurden
If ($SearchResult.Updates.Count -eq 0) {
    Write-Log "✅ Keine relevanten Updates gefunden."
    Exit
}

Write-Log "📋 Liste der relevanten Updates:"
$UpdatesArray = @()
$ExistingUpdateIDs = @{}

# ✅ Updates erfassen & JSON erstellen
ForEach ($Update in $SearchResult.Updates) {
    $UpdateID = $Update.Identity.UpdateID

    # ✅ Duplikate verhindern
    If ($ExistingUpdateIDs.ContainsKey($UpdateID)) {
        Write-Log "⚠️ Update bereits erfasst: $UpdateID"
        Continue
    }

    Write-Log "➕ Hinzufügen: $Update.Title"
    $ExistingUpdateIDs[$UpdateID] = $true

    $UpdatesArray += [PSCustomObject]@{
        scan_id        = $ScanID
        scan_date      = $ScanDate
        asset_id       = $AssetId
        update_id      = $UpdateID
        title          = $Update.Title
        description    = $Update.Description
        kb_article_ids = ($Update.KBArticleIDs -join ", ")
        support_url    = $Update.SupportUrl
        is_downloaded  = [int]$Update.IsDownloaded
        is_mandatory   = [int]$Update.IsMandatory
    }
}

# ✅ JSON-Datenstruktur gemäß Vorgabe erstellen
$JsonData = [PSCustomObject]@{
    MetaData = [PSCustomObject]@{
        ContentType  = "db-import"
        Name         = "Windows WSUS Update Scan"
        Description  = "Scan-Ergebnisse für ausstehende Updates"
        Version      = "1.0"
        Creator      = "ondeso"
        Vendor       = "ondeso GmbH"
        Preview      = ""
        Schema       = ""
    }
    Content = [PSCustomObject]@{
        TableName = "usr_wsus_scan_results"
        Consts = @(
            [PSCustomObject]@{
                Identifier = "ScanDate"
                Value      = $ScanDate
            }
        )
        FieldMappings = @(
            [PSCustomObject]@{ TargetField = "scan_id"; Expression = "{scan_id}"; ImportField = $true }
            [PSCustomObject]@{ TargetField = "scan_date"; Expression = "{scan_date}"; ImportField = $true }
            [PSCustomObject]@{ TargetField = "asset_id"; Expression = "{asset_id}"; ImportField = $true }
            [PSCustomObject]@{ TargetField = "update_id"; Expression = "{update_id}"; ImportField = $true }
            [PSCustomObject]@{ TargetField = "title"; Expression = "{title}"; ImportField = $true }
            [PSCustomObject]@{ TargetField = "description"; Expression = "{description}"; ImportField = $true }
            [PSCustomObject]@{ TargetField = "kb_article_ids"; Expression = "{kb_article_ids}"; ImportField = $true }
            [PSCustomObject]@{ TargetField = "support_url"; Expression = "{support_url}"; ImportField = $true }
            [PSCustomObject]@{ TargetField = "is_downloaded"; Expression = "{is_downloaded}"; ImportField = $true }
            [PSCustomObject]@{ TargetField = "is_mandatory"; Expression = "{is_mandatory}"; ImportField = $true }
        )
        Data = $UpdatesArray
    }
}

# ✅ JSON speichern
$JsonData | ConvertTo-Json -Depth 10 | Set-Content -Path $JsonFile -Encoding UTF8
Write-Log "✅ JSON-Datei gespeichert: $JsonFile"

# ✅ JSON an API senden
try {
    Invoke-RestMethod -Uri $ApiEndpoint -Method Post -ContentType "application/json" -InFile $JsonFile
    Write-Log "✅ API-Upload erfolgreich!"
} catch {
    Write-Log "❌ Fehler beim Hochladen: $_"
}
