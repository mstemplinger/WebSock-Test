# **Setze Hauptverzeichnis für alle Dateien**
$BaseDir = "$env:PROGRAMDATA\ondeso\workplace"
$ScanDir = "$BaseDir\scans"
$LogDir = "$BaseDir\logs"

# **Erstelle Verzeichnisse, falls sie nicht existieren**
If (!(Test-Path $ScanDir)) { New-Item -ItemType Directory -Path $ScanDir -Force | Out-Null }
If (!(Test-Path $LogDir)) { New-Item -ItemType Directory -Path $LogDir -Force | Out-Null }

# **Dateipfade setzen**
$WsusCabFile = "$ScanDir\wsusscn2.cab"
$JsonFileUpdates = "$ScanDir\wsus_scan_result.json"
$JsonFileDownloads = "$ScanDir\wsus_downloads.json"
$ClientConfigPath = "$BaseDir\client_config.ini"
$LogFilePath = "$LogDir\wsus_scan.log"

# **Microsoft-Download-URL für wsusscn2.cab**
$WsusCabUrl = "http://download.windowsupdate.com/microsoftupdate/v6/wsusscan/wsusscn2.cab"

# **API-Endpunkt**
$ApiEndpoint = "http://85.215.147.108:5001/inbox"

# **Eindeutige Scan-ID generieren**
$ScanID = [guid]::NewGuid().ToString()

# **Erfassungsdatum (UTC, ISO 8601 Format)**
$ScanDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

# ✅ Funktion zum Abrufen der `client_id` als `asset_id`
function Get-ClientId {
    if (Test-Path $ClientConfigPath) {
        try {
            $configContent = Get-Content -Path $ClientConfigPath | Where-Object { $_ -match "^client_id=" }
            if ($configContent) {
                $storedGuid = $configContent -replace "client_id=", "" | ForEach-Object { $_.Trim() }
                if ($storedGuid -match "^[0-9a-fA-F-]{36}$") {
                    Write-Host "🔄 Verwende gespeicherte Client-GUID als `asset_id`: $storedGuid"
                    return $storedGuid
                }
            }
        } catch {
            Write-Host "❌ Fehler beim Lesen der INI-Datei: $_"
        }
    }

    # Falls keine gültige GUID vorhanden ist, eine neue generieren
    $newGuid = [guid]::NewGuid().ToString()
    Write-Host "🆕 Generierte neue Client-GUID: $newGuid"

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
    Write-Host "📥 Lade neueste WSUS-Scan-Datei herunter..."
    try {
        Invoke-WebRequest -Uri $WsusCabUrl -OutFile $WsusCabFile
        Write-Host "✅ WSUS-Scan-Datei erfolgreich heruntergeladen: $WsusCabFile"
    } catch {
        Write-Host "❌ Fehler beim Herunterladen der `wsusscn2.cab`: $_"
        Exit
    }
} else {
    Write-Host "ℹ️ Bereits vorhandene `wsusscn2.cab` wird verwendet."
}

# ✅ Erstelle Update-Session
$UpdateSession = New-Object -ComObject Microsoft.Update.Session
$UpdateSearcher = $UpdateSession.CreateUpdateSearcher()

Write-Host "🔎 Suche nach installierten WSUS-Updates..."
$SearchResult = $UpdateSearcher.Search("IsInstalled=1")

# ✅ Falls keine Updates gefunden wurden
If ($SearchResult.Updates.Count -eq 0) {
    Write-Host "✅ Keine relevanten Updates gefunden."
    Exit
}

Write-Host "📋 Liste der relevanten Updates:"
$UpdatesArray = @()
$DownloadsArray = @()

# ✅ Updates erfassen und in JSON konvertieren
$ExistingUpdateIDs = @{}  # HashTable zur Duplikatsprüfung

ForEach ($update in $SearchResult.Updates) {
    $UpdateID = $update.Identity.UpdateID

    # ✅ Duplikate verhindern
    If ($ExistingUpdateIDs.ContainsKey($UpdateID)) {
        Write-Host "⚠️ Update bereits erfasst: $UpdateID"
        Continue
    }

    Write-Host "➕ Erfasse Update: $update.Title"
    $ExistingUpdateIDs[$UpdateID] = $true

    # ✅ Update-Metadaten für JSON speichern
    $UpdatesArray += [PSCustomObject]@{
        scan_id        = $ScanID
        scan_date      = $ScanDate
        asset_id       = $AssetId
        update_id      = $UpdateID
        title          = $update.Title
        description    = $update.Description
        kb_article_ids = ($update.KBArticleIDs -join ", ")
        support_url    = $update.SupportUrl
        is_downloaded  = [int]$update.IsDownloaded
        is_mandatory   = [int]$update.IsMandatory
    }

    # ✅ Download-Informationen abrufen
    ForEach ($downloadFile in $update.DownloadContents) {
        $fileUrl = $downloadFile.DownloadUrl
        $fileName = [System.IO.Path]::GetFileName($fileUrl)
        $fileSize = $downloadFile.Size

        # Prüfe, ob die Datei von einer sicheren Quelle (HTTPS) kommt
        $isSecure = If ($fileUrl -match "^https://") { 1 } Else { 0 }

        # ✅ JSON für Downloads speichern
        $DownloadsArray += [PSCustomObject]@{
            download_id  = [guid]::NewGuid().ToString()
            scan_id      = $ScanID
            asset_id     = $AssetId
            update_id    = $UpdateID
            file_url     = $fileUrl
            file_name    = $fileName
            file_size    = $fileSize
            is_secure    = $isSecure
        }
    }
}

# ✅ JSON-Datenstruktur für Updates
$JsonDataUpdates = [PSCustomObject]@{
    MetaData = [PSCustomObject]@{
        ContentType  = "db-import"
        Name         = "Windows WSUS Update Scan"
        Description  = "Scan-Ergebnisse für installierte Updates"
        Version      = "1.0"
        Creator      = "FL"
        Vendor       = "ondeso GmbH"
        Preview      = ""
        Schema       = @()
    }
    Content = [PSCustomObject]@{
        TableName = "usr_wsus_scan_results"
        Data = $UpdatesArray
    }
}

# ✅ JSON-Datenstruktur für Downloads
$JsonDataDownloads = [PSCustomObject]@{
    MetaData = [PSCustomObject]@{
        ContentType  = "db-import"
        Name         = "Windows WSUS Downloads"
        Description  = "Download-Details zu WSUS-Updates"
        Version      = "1.0"
        Creator      = "FL"
        Vendor       = "ondeso GmbH"
        Preview      = ""
        Schema       = @()
    }
    Content = [PSCustomObject]@{
        TableName = "usr_wsus_downloads"
        Data = $DownloadsArray
    }
}

# ✅ JSON-Dateien speichern
$JsonDataUpdates | ConvertTo-Json -Depth 10 | Set-Content -Path $JsonFileUpdates -Encoding UTF8
$JsonDataDownloads | ConvertTo-Json -Depth 10 | Set-Content -Path $JsonFileDownloads -Encoding UTF8

Write-Host "✅ JSON-Dateien gespeichert: $JsonFileUpdates, $JsonFileDownloads"

# ✅ API-Upload der JSON-Dateien
try {
    Invoke-RestMethod -Uri $ApiEndpoint -Method Post -ContentType "application/json" -InFile $JsonFileUpdates
    Invoke-RestMethod -Uri $ApiEndpoint -Method Post -ContentType "application/json" -InFile $JsonFileDownloads
    Write-Host "✅ API-Upload abgeschlossen!"
} catch {
    Write-Host "❌ Fehler beim Hochladen der JSON-Dateien: $_"
}
