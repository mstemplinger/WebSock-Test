# PowerShell-Skript zur Erfassung von Systeminformationen und API-Upload

# **Verzeichnis für temporäre Speicherung**
$directoryPath = "$env:TEMP"
$jsonFile = "$directoryPath\system_info.json"

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
$ipAddress = (Get-NetIPAddress -AddressFamily IPv4 | Select-Object -ExpandProperty IPAddress) -join ", "
$macAddress = (Get-NetAdapter | Select-Object -ExpandProperty MacAddress) -join ", "

# **Erfassungszeitpunkt (ISO 8601 Format)**
$captureDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

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
            @{ TargetField = "transaction_id"; Expression = "{transaction_id}"; IsIdentifier = [bool]::Parse("true"); ImportField = [bool]::Parse("true") }
            @{ TargetField = "os_name"; Expression = "{os_name}"; IsIdentifier = [bool]::Parse("false"); ImportField = [bool]::Parse("true") }
            @{ TargetField = "os_version"; Expression = "{os_version}"; IsIdentifier = [bool]::Parse("false"); ImportField = [bool]::Parse("true") }
            @{ TargetField = "cpu_model"; Expression = "{cpu_model}"; IsIdentifier = [bool]::Parse("false"); ImportField = [bool]::Parse("true") }
            @{ TargetField = "cpu_cores"; Expression = "{cpu_cores}"; IsIdentifier = [bool]::Parse("false"); ImportField = [bool]::Parse("true") }
            @{ TargetField = "ram_total"; Expression = "{ram_total}"; IsIdentifier = [bool]::Parse("false"); ImportField = [bool]::Parse("true") }
            @{ TargetField = "disk_total"; Expression = "{disk_total}"; IsIdentifier = [bool]::Parse("false"); ImportField = [bool]::Parse("true") }
            @{ TargetField = "disk_free"; Expression = "{disk_free}"; IsIdentifier = [bool]::Parse("false"); ImportField = [bool]::Parse("true") }
            @{ TargetField = "ip_address"; Expression = "{ip_address}"; IsIdentifier = [bool]::Parse("true"); ImportField = [bool]::Parse("true") }
            @{ TargetField = "mac_address"; Expression = "{mac_address}"; IsIdentifier = [bool]::Parse("true"); ImportField = [bool]::Parse("true") }
        )
        Data = @(
            @{
                transaction_id = $transactionID
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
}
