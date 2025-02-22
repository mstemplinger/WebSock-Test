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

Write-Log "🚀 Starte WSUS-Scan-Skript mit Basisverzeichnis: $BaseDir"

# ✅ Funktion zum Abrufen der `client_id` als `asset_id`
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

    # Falls keine gültige GUID vorhanden ist, eine neue generieren
    $newGuid = [guid]::NewGuid().ToString()
    Write-Log "🆕 Generierte neue Client-GUID: $newGuid"

    @"
[CLIENT]
client_id=$newGuid
"@ | Set-Content -Path $ClientConfigPath

    return $newGuid
}

# ✅ Lese `client_id` als `asset_id`
$AssetId = Get-ClientId

# ✅ Prüfen, ob `wsusscn2.cab` existiert, sonst herunterladen
If (!(Test-Path $WsusCabFile)) {
    Write-Log "📥 Lade neueste WSUS-Scan-Datei herunter..."
    try {
        Invoke-WebRequest -Uri $WsusCabUrl -OutFile $WsusCabFile
        Write-Log "✅ WSUS-Scan-Datei erfolgreich heruntergeladen: $WsusCabFile"
    } catch {
        Write-Log "❌ Fehler beim Herunterladen der `wsusscn2.cab`: $_"
        Exit
    }
} else {
    Write-Log "ℹ️ Bereits vorhandene `wsusscn2.cab` wird verwendet."
}

# ✅ Erstelle Update-Session
$UpdateSession = New-Object -ComObject Microsoft.Update.Session
$UpdateSearcher = $UpdateSession.CreateUpdateSearcher()

Write-Log "🔎 Suche nach installierten WSUS-Updates..."
$SearchResult = $UpdateSearcher.Search("IsInstalled=1")

# ✅ Falls keine Updates gefunden wurden
If ($SearchResult.Updates.Count -eq 0) {
    Write-Log "✅ Keine relevanten Updates gefunden."
    Exit
}

Write-Log "📋 Liste der relevanten Updates:"
$UpdatesArray = @()

# ✅ Updates erfassen und in JSON konvertieren
$ExistingUpdateIDs = @{}  # HashTable zur Duplikatsprüfung

ForEach ($update in $SearchResult.Updates) {
    $UpdateID = $update.Identity.UpdateID

    # ✅ Duplikate verhindern
    If ($ExistingUpdateIDs.ContainsKey($UpdateID)) {
        Write-Log "⚠️ Update bereits erfasst: $UpdateID"
        Continue
    }

    Write-Log "➕ Erfasse Update: $update.Title"
    $ExistingUpdateIDs[$UpdateID] = $true

    # ✅ Update-Metadaten für JSON speichern
    $UpdatesArray += [PSCustomObject]@{
        scan_id        = [guid]::NewGuid().ToString()
        scan_date      = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
        asset_id       = $AssetId
        update_id      = $UpdateID
        title          = $update.Title
        description    = $update.Description
        kb_article_ids = ($update.KBArticleIDs -join ", ")
        support_url    = $update.SupportUrl
        is_downloaded  = [int]$update.IsDownloaded
        is_mandatory   = [int]$update.IsMandatory
    }
}

# ✅ JSON-Datenstruktur für Updates
$JsonData = [PSCustomObject]@{
    MetaData = [PSCustomObject]@{
        ContentType  = "db-import"
        Name         = "Windows WSUS Update Scan"
        Description  = "Scan-Ergebnisse für installierte Updates"
        Version      = "1.0"
        Creator      = "FL"
        Vendor       = "ondeso GmbH"
        Preview      = ""
        Schema       = ""
    }
    Content = [PSCustomObject]@{
        TableName = "usr_wsus_scan_results"
        Consts = @(
            [PSCustomObject]@{
                Identifier = "ScanDate"
                Value      = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
            }
        )
        FieldMappings = @(
            [PSCustomObject]@{ TargetField = "scan_id"; Expression = "{scan_id}"; IsIdentifier = $true; ImportField = $true }
            [PSCustomObject]@{ TargetField = "scan_date"; Expression = "{scan_date}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "asset_id"; Expression = "{asset_id}"; IsIdentifier = $true; ImportField = $true }
            [PSCustomObject]@{ TargetField = "update_id"; Expression = "{update_id}"; IsIdentifier = $true; ImportField = $true }
            [PSCustomObject]@{ TargetField = "title"; Expression = "{title}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "kb_article_ids"; Expression = "{kb_article_ids}"; IsIdentifier = $false; ImportField = $true }
        )
        Data = $UpdatesArray
    }
}

# ✅ JSON-Datei speichern
$JsonData | ConvertTo-Json -Depth 10 | Set-Content -Path $JsonFile -Encoding UTF8
Write-Log "✅ JSON-Datei gespeichert: $JsonFile"

# ✅ JSON an API senden
try {
    Invoke-RestMethod -Uri $ApiEndpoint -Method Post -ContentType "application/json" -InFile $JsonFile
    Write-Log "✅ API-Upload abgeschlossen!"
} catch {
    Write-Log "❌ Fehler beim Hochladen der JSON-Datei: $_"
}
