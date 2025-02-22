# **Setze Hauptverzeichnis für alle Dateien**
$BaseDir = "$env:PROGRAMDATA\ondeso\workplace"
$SystemDir = "$BaseDir\system_info"
$LogDir = "$BaseDir\logs"
$JsonFile = "$SystemDir\system_info.json"
$ClientConfigPath = "$BaseDir\client_config.ini"
$LogFilePath = "$LogDir\system_info.log"

# ✅ Erstelle Verzeichnisse, falls sie nicht existieren
$Folders = @($BaseDir, $SystemDir, $LogDir)
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

Write-Log "🚀 Starte System-Info-Erfassung mit Basisverzeichnis: $BaseDir"

# ✅ Eindeutige Transaktions-ID generieren
$TransactionID = [guid]::NewGuid().ToString()

# ✅ Systeminformationen abrufen
$OsInfo = Get-CimInstance -ClassName Win32_OperatingSystem
$CpuInfo = Get-CimInstance -ClassName Win32_Processor
$DiskInfo = Get-CimInstance -ClassName Win32_LogicalDisk -Filter "DeviceID='C:'"
$RamTotal = [math]::Round($OsInfo.TotalVisibleMemorySize / 1MB, 2)
$DiskTotal = [math]::Round($DiskInfo.Size / 1GB, 2)
$DiskFree = [math]::Round($DiskInfo.FreeSpace / 1GB, 2)

# ✅ Netzwerkinformationen abrufen
$IpAddress = (Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.InterfaceAlias -notlike "*Loopback*" } | Select-Object -ExpandProperty IPAddress) -join ", "
$MacAddress = (Get-NetAdapter | Where-Object { $_.Status -eq "Up" } | Select-Object -ExpandProperty MacAddress) -join ", "

# ✅ Erfassungszeitpunkt (ISO 8601 Format)
$CaptureDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

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

# ✅ JSON-Datenstruktur erstellen
$JsonData = [PSCustomObject]@{
    MetaData = [PSCustomObject]@{
        Version     = "1.0"
        ContentType = "db-import"
        Name        = "Windows System Information"
        Creator     = "ondeso"
        Description = "Erfasst Systeminformationen"
        Vendor      = "ondeso GmbH"
        Schema      = ""
        Preview     = ""
    }
    Content = [PSCustomObject]@{
        TableName   = "usr_system_info"
        Consts = @(
            [PSCustomObject]@{
                Identifier = "CaptureDate"
                Value      = $CaptureDate
            }
        )
        FieldMappings = @(
            [PSCustomObject]@{ TargetField = "transaction_id"; Expression = "{transaction_id}"; IsIdentifier = $true; ImportField = $true }
            [PSCustomObject]@{ TargetField = "asset_id"; Expression = "{asset_id}"; IsIdentifier = $true; ImportField = $true }
            [PSCustomObject]@{ TargetField = "os_name"; Expression = "{os_name}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "os_version"; Expression = "{os_version}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "cpu_model"; Expression = "{cpu_model}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "cpu_cores"; Expression = "{cpu_cores}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "ram_total"; Expression = "{ram_total}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "disk_total"; Expression = "{disk_total}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "disk_free"; Expression = "{disk_free}"; IsIdentifier = $false; ImportField = $true }
            [PSCustomObject]@{ TargetField = "ip_address"; Expression = "{ip_address}"; IsIdentifier = $true; ImportField = $true }
            [PSCustomObject]@{ TargetField = "mac_address"; Expression = "{mac_address}"; IsIdentifier = $true; ImportField = $true }
        )
        Data = @(
            [PSCustomObject]@{
                transaction_id = $TransactionID
                asset_id      = $AssetId  # **Setze die client_id als asset_id**
                os_name       = $OsInfo.Caption
                os_version    = $OsInfo.Version
                cpu_model     = $CpuInfo.Name
                cpu_cores     = $CpuInfo.NumberOfCores
                ram_total     = "$RamTotal GB"
                disk_total    = "$DiskTotal GB"
                disk_free     = "$DiskFree GB"
                ip_address    = $IpAddress
                mac_address   = $MacAddress
            }
        )
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
    Write-Log "❌ JSON, das gesendet wurde:"
    Write-Host $JsonData
}
