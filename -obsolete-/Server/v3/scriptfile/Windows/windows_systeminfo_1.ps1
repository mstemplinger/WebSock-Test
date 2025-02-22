# PowerShell-Skript zur Erfassung von Systeminformationen und API-Upload

# **Verzeichnis für temporäre Speicherung**
# $directoryPath = "$env:TEMP"
$directoryPath = "c:\TEMP"
$jsonFile = "$directoryPath\system_info.json"
$clientConfigPath = "$directoryPath\client_config.ini"

# **Eindeutige Transaktions-ID generieren**
$transactionID = [guid]::NewGuid().ToString()

# **Systeminformationen abrufen**
$osInfo = Get-CimInstance -ClassName Win32_OperatingSystem
$cpuInfo = Get-CimInstance -ClassName Win32_Processor
$diskInfo = Get-CimInstance -ClassName Win32_LogicalDisk -Filter "DeviceID='C:'"
$ramTotal = [math]::Round($osInfo.TotalVisibleMemorySize / 1MB, 2)
$diskTotal = [math]::Round($diskInfo.Size / 1GB, 2)
$diskFree = [math]::Round($diskInfo.FreeSpace / 1GB, 2)

# **Netzwerkinformationen abrufen**
$ipAddress = (Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.InterfaceAlias -notlike "*Loopback*" } | Select-Object -ExpandProperty IPAddress) -join ", "
$macAddress = (Get-NetAdapter | Where-Object { $_.Status -eq "Up" } | Select-Object -ExpandProperty MacAddress) -join ", "

# **Erfassungszeitpunkt (ISO 8601 Format)**
$captureDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

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
Write-Host "📌 Asset-ID (Client GUID): $assetId"

# **JSON-Datenstruktur erstellen**
$jsonData = @{
    MetaData = @{
        Version     = "1.0"
        ContentType = "db-import"
        Name        = "Windows System Information"
        Creator     = "FL"
        Description = "Erfasst Systeminformationen"
        Vendor      = "ondeso GmbH"
        Schema      = ""
        Preview     = ""
    }
    Content = @{
        TableName   = "usr_system_info"
        Consts = @(
            @{
                Identifier = "CaptureDate"
                Value      = $captureDate
            }
        )
        FieldMappings = @(
            @{ TargetField = "transaction_id"; Expression = "{transaction_id}"; IsIdentifier = $true; ImportField = $true }
            @{ TargetField = "asset_id"; Expression = "{asset_id}"; IsIdentifier = $true; ImportField = $true }
            @{ TargetField = "os_name"; Expression = "{os_name}"; IsIdentifier = $false; ImportField = $true }
            @{ TargetField = "os_version"; Expression = "{os_version}"; IsIdentifier = $false; ImportField = $true }
            @{ TargetField = "cpu_model"; Expression = "{cpu_model}"; IsIdentifier = $false; ImportField = $true }
            @{ TargetField = "cpu_cores"; Expression = "{cpu_cores}"; IsIdentifier = $false; ImportField = $true }
            @{ TargetField = "ram_total"; Expression = "{ram_total}"; IsIdentifier = $false; ImportField = $true }
            @{ TargetField = "disk_total"; Expression = "{disk_total}"; IsIdentifier = $false; ImportField = $true }
            @{ TargetField = "disk_free"; Expression = "{disk_free}"; IsIdentifier = $false; ImportField = $true }
            @{ TargetField = "ip_address"; Expression = "{ip_address}"; IsIdentifier = $true; ImportField = $true }
            @{ TargetField = "mac_address"; Expression = "{mac_address}"; IsIdentifier = $true; ImportField = $true }
        )
        Data = @(
            @{
                transaction_id = $transactionID
                asset_id      = $assetId  # **Setze die client_id als asset_id**
                os_name       = $osInfo.Caption
                os_version    = $osInfo.Version
                cpu_model     = $cpuInfo.Name
                cpu_cores     = $cpuInfo.NumberOfCores
                ram_total     = "$ramTotal GB"
                disk_total    = "$diskTotal GB"
                disk_free     = "$diskFree GB"
                ip_address    = $ipAddress
                mac_address   = $macAddress
            }
        )
    }
} | ConvertTo-Json -Depth 3 -Compress

# **JSON-Datei speichern**
$jsonData | Set-Content -Path $jsonFile -Encoding UTF8
Write-Host "✅ JSON-Datei gespeichert: $jsonFile"

# **API-Endpunkt für den Upload**
$apiEndpoint = "http://85.215.147.108:5001/inbox"

# **JSON an API senden**
try {
    $response = Invoke-RestMethod -Uri $apiEndpoint -Method Post -ContentType "application/json" -InFile $jsonFile
    Write-Host "✅ API-Antwort: $response"
} catch {
    Write-Host "❌ Fehler beim Senden an API: $_"
    Write-Host "❌ JSON, das gesendet wurde:"
    Write-Host $jsonData
}
