# **Verzeichnis für temporäre Speicherung**
$TempDir = "$env:TEMP"
$WsusCabFile = "$TempDir\wsusscn2.cab"
$JsonFile = "$TempDir\wsus_scan_result.json"
$ClientConfigPath = "C:\TEMP\client_config.ini"

# **Microsoft-Download-URL für wsusscn2.cab**
$WsusCabUrl = "http://download.windowsupdate.com/microsoftupdate/v6/wsusscan/wsusscn2.cab"

# **API-Endpunkt**
$ApiEndpoint = "http://85.215.147.108:5001/inbox"

# **Eindeutige Scan-ID generieren**
$ScanID = [guid]::NewGuid().ToString()

# **Erfassungsdatum (UTC, ISO 8601 Format)**
$ScanDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

# **Funktion zum Lesen der `client_id` als `asset_id`**
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

# **Lese `client_id` als `asset_id` aus der INI**
$AssetId = Get-ClientId

# **1️⃣ Prüfen, ob wsusscn2.cab bereits existiert, sonst herunterladen**
If (!(Test-Path $WsusCabFile)) {
    Write-Host "📥 Lade neueste wsusscn2.cab herunter..."
    try {
        Invoke-WebRequest -Uri $WsusCabUrl -OutFile $WsusCabFile
        Write-Host "✅ WSUS-Scan-Datei erfolgreich heruntergeladen: $WsusCabFile"
    } catch {
        Write-Host "❌ Fehler beim Herunterladen der wsusscn2.cab: $_"
        Exit
    }
} else {
    Write-Host "ℹ️ Bereits vorhandene wsusscn2.cab wird verwendet."
}

# **2️⃣ Erstelle Update-Session**
$UpdateSession = New-Object -ComObject Microsoft.Update.Session
$UpdateServiceManager = New-Object -ComObject Microsoft.Update.ServiceManager
$UpdateService = $UpdateServiceManager.AddScanPackageService("Offline Sync Service", $WsusCabFile)
$UpdateSearcher = $UpdateSession.CreateUpdateSearcher()

Write-Host "🔎 Suche nach WSUS-Updates..."
$UpdateSearcher.ServerSelection = 3  # ssOthers
$UpdateSearcher.ServiceID = [string] $UpdateService.ServiceID
$SearchResult = $UpdateSearcher.Search("IsInstalled=1")
$Updates = $SearchResult.Updates

# **3️⃣ Falls keine Updates gefunden wurden**
If ($SearchResult.Updates.Count -eq 0) {
    Write-Host "✅ Keine relevanten Updates gefunden."
    Exit
}

Write-Host "📋 Liste der relevanten Updates:"
$UpdatesArray = @()

# **4️⃣ Updates erfassen und in JSON konvertieren**
$ExistingUpdateIDs = @{}  # HashTable zur Duplikatsprüfung

ForEach ($update in $SearchResult.Updates) {
    $UpdateID = $update.Identity.UpdateID

    # **Duplikate verhindern**
    If ($ExistingUpdateIDs.ContainsKey($UpdateID)) {
        Write-Host "⚠️ Update bereits erfasst: $UpdateID"
        Continue
    }

    Write-Host "➕ Hinzufügen: $update.Title"
    $ExistingUpdateIDs[$UpdateID] = $true

    $UpdatesArray += [PSCustomObject]@{
        scan_id        = $ScanID
        scan_date      = $ScanDate
        asset_id       = $AssetId  # **Asset-ID hinzufügen**
        update_id      = $UpdateID
        title          = $update.Title
        description    = $update.Description
        kb_article_ids = ($update.KBArticleIDs -join ", ")
        support_url    = $update.SupportUrl
        is_downloaded  = [int]$update.IsDownloaded  # Boolean → Integer (0/1)
        is_mandatory   = [int]$update.IsMandatory   # Boolean → Integer (0/1)
    }
}

# **5️⃣ JSON-Datenstruktur gemäß Vorgabe erstellen**
$JsonData = [PSCustomObject]@{
    MetaData = [PSCustomObject]@{
        ContentType  = "db-import"
        Name         = "Windows WSUS Update Scan"
        Description  = "Scan-Ergebnisse für ausstehende Updates"
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
                Value      = $ScanDate
            }
        )
        FieldMappings = @(
            [PSCustomObject]@{ TargetField = "scan_id"; Expression = "{scan_id}"; IsIdentifier = $true; ImportField = $true }
            [PSCustomObject]@{ TargetField = "scan_date"; Expression = "{scan_date}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "asset_id"; Expression = "{asset_id}"; IsIdentifier = $true; ImportField = $true }
            [PSCustomObject]@{ TargetField = "update_id"; Expression = "{update_id}"; IsIdentifier = $true; ImportField = $true }
            [PSCustomObject]@{ TargetField = "title"; Expression = "{title}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "description"; Expression = "{description}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "kb_article_ids"; Expression = "{kb_article_ids}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "support_url"; Expression = "{support_url}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "is_downloaded"; Expression = "{is_downloaded}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "is_mandatory"; Expression = "{is_mandatory}"; IsIdentifier = $false; ImportField = $true }
        )
        Data = $UpdatesArray
    }
}

# **6️⃣ JSON-Datei speichern**
$JsonData | ConvertTo-Json -Depth 10 | Set-Content -Path $JsonFile -Encoding UTF8
Write-Host "✅ JSON-Datei gespeichert: $JsonFile"

# **7️⃣ JSON an API senden**
try {
    $Response = Invoke-RestMethod -Uri $ApiEndpoint -Method Post -ContentType "application/json" -InFile $JsonFile
    Write-Host "✅ API-Antwort: $Response"
} catch {
    Write-Host "❌ Fehler beim Senden an API: $_"
}
