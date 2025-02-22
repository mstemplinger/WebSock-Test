# 📌 Variablen für JSON- und API-Pfad
$TempDir = "$env:TEMP"
$JsonFileUpdates = "$TempDir\wsus_scan_result.json"
$JsonFileDownloads = "$TempDir\wsus_downloads.json"
$ApiEndpoint = "http://85.215.147.108:5001/inbox"

# 📌 WSUS Update Catalog URL
$WsusCatalogUrl = "https://www.microsoft.com/download/confirmation.aspx?id="

# 📌 WSUS Scan Datei herunterladen
$WsusCabFile = "$TempDir\wsusscn2.cab"
$WsusDownloadUrl = "http://download.windowsupdate.com/microsoftupdate/v6/wsusscan/wsusscn2.cab"

if (!(Test-Path $WsusCabFile)) {
    Write-Host "⬇️  Lade WSUS Scan Datei herunter..."
    Invoke-WebRequest -Uri $WsusDownloadUrl -OutFile $WsusCabFile
}

# 📌 Erstelle Update-Session
$UpdateSession = New-Object -ComObject Microsoft.Update.Session
$UpdateServiceManager = New-Object -ComObject Microsoft.Update.ServiceManager
$UpdateService = $UpdateServiceManager.AddScanPackageService("Offline Sync Service", $WsusCabFile)
$UpdateSearcher = $UpdateSession.CreateUpdateSearcher()

Write-Host "🔎 Suche nach Updates..."
$UpdateSearcher.ServerSelection = 3  # ssOthers
$UpdateSearcher.ServiceID = [string] $UpdateService.ServiceID
$SearchResult = $UpdateSearcher.Search("IsInstalled=0")
$Updates = $SearchResult.Updates

# 📌 Falls keine Updates gefunden wurden
If ($SearchResult.Updates.Count -eq 0) {
    Write-Host "✅ Keine Updates erforderlich."
    Exit
}

Write-Host "📋 Liste der relevanten Updates:"
$UpdatesArray = @()
$DownloadsArray = @()

# 📌 Updates erfassen und in JSON konvertieren
ForEach ($update in $Updates) {
    $updateId = $update.Identity.UpdateID
    $kbNumbers = ($update.KBArticleIDs -join ", ")
    
    Write-Host "📌 Update gefunden: $($update.Title)"
    
    # 📌 JSON für Update-Scan-Informationen erstellen
    $UpdatesArray += @{
        "scan_id"       = [guid]::NewGuid().ToString()
        "scan_date"     = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
        "update_id"     = $updateId
        "title"         = $update.Title
        "description"   = $update.Description
        "kb_article_ids"= $kbNumbers
        "support_url"   = $update.SupportUrl
        "is_downloaded" = [int]$update.IsDownloaded
        "is_mandatory"  = [int]$update.IsMandatory
    }

    # 📌 Download-Informationen abrufen
    ForEach ($downloadFile in $update.DownloadContents) {
        $fileUrl = $downloadFile.DownloadUrl
        $fileName = [System.IO.Path]::GetFileName($fileUrl)
        $fileSize = $downloadFile.Size

        # Prüfe, ob die Datei von einer sicheren Quelle (HTTPS) kommt
        $isSecure = If ($fileUrl -match "^https://") { 1 } Else { 0 }

        # 📌 JSON für Update-Download-Informationen erstellen
        $DownloadsArray += @{
            "download_id"  = [guid]::NewGuid().ToString()
            "scan_id"      = $updateId
            "update_id"    = $updateId
            "file_url"     = $fileUrl
            "file_name"    = $fileName
            "file_size"    = $fileSize
            "is_secure"    = $isSecure
        }
    }
}

# 📌 JSON-Datenstruktur für Updates erstellen
$JsonDataUpdates = @{
    "MetaData" = @{
        "ContentType"  = "db-import"
        "Name"         = "Windows WSUS Update Scan"
        "Description"  = "Scan-Ergebnisse für ausstehende Updates"
        "Version"      = "1.0"
        "Creator"      = "FL"
        "Vendor"       = "ondeso GmbH"
        "Preview"      = ""
        "Schema"       = ""
    }
    "Content" = @{
        "TableName"  = "usr_wsus_scan_results"
        "Consts"     = @(@{ "Identifier" = "ScanDate"; "Value" = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ") })
        "FieldMappings" = @(
            @{ "TargetField" = "scan_id"; "Expression" = "{scan_id}"; "IsIdentifier" = $true; "ImportField" = $true }
            @{ "TargetField" = "scan_date"; "Expression" = "{scan_date}"; "IsIdentifier" = $false; "ImportField" = $true }
            @{ "TargetField" = "update_id"; "Expression" = "{update_id}"; "IsIdentifier" = $true; "ImportField" = $true }
            @{ "TargetField" = "title"; "Expression" = "{title}"; "IsIdentifier" = $false; "ImportField" = $true }
            @{ "TargetField" = "description"; "Expression" = "{description}"; "IsIdentifier" = $false; "ImportField" = $true }
        )
        "Data" = $UpdatesArray
    }
}

# 📌 JSON-Dateien speichern
$JsonDataUpdates | ConvertTo-Json -Depth 10 | Set-Content -Path $JsonFileUpdates -Encoding UTF8
$DownloadsArray | ConvertTo-Json -Depth 10 | Set-Content -Path $JsonFileDownloads -Encoding UTF8

# 📌 API-Upload der JSON-Dateien
Invoke-RestMethod -Uri $ApiEndpoint -Method Post -ContentType "application/json" -InFile $JsonFileUpdates
Invoke-RestMethod -Uri $ApiEndpoint -Method Post -ContentType "application/json" -InFile $JsonFileDownloads

Write-Host "✅ API-Upload abgeschlossen!"
