# **Setze Hauptverzeichnis für alle Dateien**
$BaseDir = "$env:PROGRAMDATA\ondeso\workplace"
$SecurityDir = "$BaseDir\security"
$LogDir = "$BaseDir\logs"

# **Erstelle Verzeichnisse, falls sie nicht existieren**
If (!(Test-Path $SecurityDir)) { New-Item -ItemType Directory -Path $SecurityDir -Force | Out-Null }
If (!(Test-Path $LogDir)) { New-Item -ItemType Directory -Path $LogDir -Force | Out-Null }

# **Dateipfade setzen**
$JsonFileSecurity = "$SecurityDir\security_inventory.json"
$ClientConfigPath = "$BaseDir\client_config.ini"
$LogFilePath = "$LogDir\security_scan.log"

# **API-Endpunkt**
$ApiEndpoint = "http://85.215.147.108:5001/inbox"

# **Eindeutige Scan-ID generieren**
$ScanDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

# ✅ Funktion zum Abrufen der `client_id` als `asset_id`
function Get-ClientId {
    if (Test-Path $ClientConfigPath) {
        try {
            $configContent = Get-Content -Path $ClientConfigPath | Where-Object { $_ -match "^client_id=" }
            if ($configContent) {
                $storedGuid = $configContent -replace "client_id=", "" | ForEach-Object { $_.Trim() }
                if ($storedGuid -match "^[0-9a-fA-F-]{36}$") {
                    return $storedGuid
                }
            }
        } catch { }
    }

    # Falls keine gültige GUID vorhanden ist, eine neue generieren
    $newGuid = [guid]::NewGuid().ToString()
    @"
[CLIENT]
client_id=$newGuid
"@ | Set-Content -Path $ClientConfigPath
    return $newGuid
}

# ✅ Lese `client_id` als `asset_id`
$AssetId = Get-ClientId

# ✅ **Firewall-Status für alle Profile abrufen (Domain, Private, Public)**
$FirewallProfiles = Get-NetFirewallProfile -ErrorAction SilentlyContinue
function Get-FirewallStatus {
    $firewallProfiles = Get-NetFirewallProfile -ErrorAction SilentlyContinue
    $firewallStatus = @()

    if ($firewallProfiles) {
        foreach ($profile in $firewallProfiles) {
            $status = if ($profile.Enabled -eq $true) { "Enabled" } else { "Disabled" }
            $firewallStatus += "$($profile.Name)=$status"
        }
    } else {
        $firewallStatus = "N/A"
    }

    return $firewallStatus -join ', '
}
$FirewallStatus = Get-FirewallStatus

function Get-RDPStatus {
    $rdpStatus = "Disabled"

    # Prüfe Registry-Eintrag für Standard-RDP-Verbindungen auf Workstations
    $rdpEnabled = Get-ItemProperty -Path "HKLM:\System\CurrentControlSet\Control\Terminal Server" -Name "fDenyTSConnections" -ErrorAction SilentlyContinue | Select-Object -ExpandProperty fDenyTSConnections
    if ($rdpEnabled -eq 0) { $rdpStatus = "Enabled" }

    # Prüfe, ob die RDP-Dienste laufen (zusätzlich für Server-Umgebungen)
    $rdpService = Get-Service -Name "TermService" -ErrorAction SilentlyContinue
    if ($rdpService.Status -eq "Running") { $rdpStatus = "Enabled" }

    return $rdpStatus
}
$RemoteDesktopStatus = Get-RDPStatus

function Get-LocalGroupMembersBySID {
    param (
        [string]$GroupSID
    )

    try {
        # Auflösen der SID zur echten Gruppenbezeichnung
        $groupName = (Get-WmiObject Win32_Group -Filter "SID='$GroupSID'" | Select-Object -ExpandProperty Name)

        if (-not $groupName) {
            return "Error: Keine Gruppe für SID $GroupSID gefunden"
        }

        # Mitglieder der Gruppe abrufen
        $groupMembers = Get-LocalGroupMember -Group $groupName -ErrorAction SilentlyContinue |
            Select-Object -ExpandProperty Name

        if ($groupMembers) {
            return $groupMembers -join ", "
        } else {
            return "$groupName hat keine Mitglieder"
        }
    } catch {
        return "Error: Konnte Mitglieder der Gruppe nicht abrufen"
    }
}
# Beispiel: Aufrufen der Funktion mit einer SID
$Admins = Get-LocalGroupMembersBySID "S-1-5-32-544"
$Guests = Get-LocalGroupMembersBySID "S-1-5-32-546"
$Users  = Get-LocalGroupMembersBySID "S-1-5-32-545"

function Get-SMB1Status {
    try {
        $smb1State = Get-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Services\LanmanServer\Parameters" -Name SMB1 -ErrorAction SilentlyContinue
        if ($smb1State -and $smb1State.SMB1 -eq 1) {
            return "Enabled"
        } else {
            return "Disabled"
        }
    } catch {
        return "Error Retrieving SMB1 Status"
    }
}

$SMB1Status = Get-SMB1Status

function Get-FailedLogins {
    try {
        $events = Get-WinEvent -LogName Security -FilterHashtable @{Id=4625} -MaxEvents 10 -ErrorAction SilentlyContinue |
            Select-Object TimeCreated, @{Name="User"; Expression={$_.Properties[5].Value}}, @{Name="SourceIP"; Expression={$_.Properties[18].Value}}

        if ($events) {
            return $events | Format-Table -AutoSize | Out-String
        } else {
            return "No failed logins found"
        }
    } catch {
        return "Error retrieving failed logins"
    }
}
$FailedLogins = Get-FailedLogins

# ✅ **Sicherheitsbezogene Elemente abrufen** (mit Fehlerhandling)
$SecurityData = [PSCustomObject]@{
    scan_date             = "$ScanDate"
    asset_id              = "$AssetId"
    os_name               = (Get-CimInstance Win32_OperatingSystem).Caption
    os_version            = (Get-CimInstance Win32_OperatingSystem).Version
    os_last_boot          = ((Get-CimInstance Win32_OperatingSystem).LastBootUpTime).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    firewall_status       = "$FirewallStatus"
    antivirus_installed   = (Get-CimInstance -Namespace "root\SecurityCenter2" -ClassName AntiVirusProduct -ErrorAction SilentlyContinue | Select-Object -ExpandProperty displayName) -join ', '
    windows_defender      = (Get-MpComputerStatus -ErrorAction SilentlyContinue | Select-Object -ExpandProperty AMRunningMode) -as [string] -or "N/A"
    bitlocker_status      = (Get-BitLockerVolume -ErrorAction SilentlyContinue | ForEach-Object { if ($_.ProtectionStatus -eq 1) { "Enabled" } else { "Disabled" } }) -join ', '
    uac_status            = [int](Get-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System" -Name EnableLUA -ErrorAction SilentlyContinue | Select-Object -ExpandProperty EnableLUA) -or 0
    local_admins          = "Local Admin Users: $Admins"
    remote_desktop        = "RDP Status: $RemoteDesktopStatus"
	smb_status		      = "SMB1 Status: $SMB1Status"
	guest_accounts		  = "Guest: $Guests"
	user_accounts         = "Users: $Users"
	open_ports            = (Get-NetTCPConnection | Select-Object -ExpandProperty LocalPort | Sort-Object -Unique) -join ", "
	logon_events          = (Get-EventLog -LogName Security -InstanceId 4624 -Newest 10 | Select-Object -ExpandProperty TimeGenerated) -join ", "
	failed_logins         = $FailedLogins
	last_patch_date       = ((Get-HotFix | Sort-Object InstalledOn -Descending | Select-Object -First 1).InstalledOn).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
}

# ✅ JSON-Datenstruktur für Security-Scan mit FieldMappings
$JsonDataSecurity = [PSCustomObject]@{
    MetaData = [PSCustomObject]@{
        ContentType  = "db-import"
        Name         = "Windows Security Inventory"
        Description  = "Sicherheitsbezogenes Inventar eines Windows-PCs"
        Version      = "1.0"
        Creator      = "FL"
        Vendor       = "ondeso GmbH"
        Preview      = ""
        Schema       = @()
    }
    Content = [PSCustomObject]@{
        TableName = "usr_security_inventory"
        FieldMappings = @(
            @{ TargetField = "scan_date"; Expression = "{scan_date}"; IsIdentifier = $false; ImportField = $true }
            @{ TargetField = "asset_id"; Expression = "{asset_id}"; IsIdentifier = $true; ImportField = $true }
            @{ TargetField = "os_name"; Expression = "{os_name}"; IsIdentifier = $false; ImportField = $true }
            @{ TargetField = "os_version"; Expression = "{os_version}"; IsIdentifier = $false; ImportField = $true }
            @{ TargetField = "os_last_boot"; Expression = "{os_last_boot}"; IsIdentifier = $false; ImportField = $true }
            @{ TargetField = "firewall_status"; Expression = "{firewall_status}"; IsIdentifier = $false; ImportField = $true }
            @{ TargetField = "antivirus_installed"; Expression = "{antivirus_installed}"; IsIdentifier = $false; ImportField = $true }
            @{ TargetField = "windows_defender"; Expression = "{windows_defender}"; IsIdentifier = $false; ImportField = $true }
            @{ TargetField = "bitlocker_status"; Expression = "{bitlocker_status}"; IsIdentifier = $false; ImportField = $true }
            @{ TargetField = "uac_status"; Expression = "{uac_status}"; IsIdentifier = $false; ImportField = $true }
            @{ TargetField = "local_admins"; Expression = "{local_admins}"; IsIdentifier = $false; ImportField = $true }
            @{ TargetField = "remote_desktop"; Expression = "{remote_desktop}"; IsIdentifier = $false; ImportField = $true }
			@{ TargetField = "smb_status"; Expression = "{smb_status}"; IsIdentifier = $false; ImportField = $true }
			@{ TargetField = "guest_account"; Expression = "{guest_accounts}"; IsIdentifier = $false; ImportField = $true }
			@{ TargetField = "user_accounts"; Expression = "{user_accounts}"; IsIdentifier = $false; ImportField = $true }
			@{ TargetField = "open_ports"; Expression = "{open_ports}"; IsIdentifier = $false; ImportField = $true }
			@{ TargetField = "logon_events"; Expression = "{logon_events}"; IsIdentifier = $false; ImportField = $true }
			@{ TargetField = "failed_logins"; Expression = "{failed_logins}"; IsIdentifier = $false; ImportField = $true }
			@{ TargetField = "last_patch_date"; Expression = "{last_patch_date}"; IsIdentifier = $false; ImportField = $true }
        )
        Data = @($SecurityData)
    }
}

# ✅ JSON-Datei speichern
$JsonDataSecurity | ConvertTo-Json -Depth 10 | Set-Content -Path $JsonFileSecurity -Encoding UTF8

Write-Host "✅ JSON-Datei gespeichert: $JsonFileSecurity"

# ✅ API-Upload der JSON-Datei
try {
    Invoke-RestMethod -Uri $ApiEndpoint -Method Post -ContentType "application/json" -InFile $JsonFileSecurity
    Write-Host "✅ API-Upload abgeschlossen!"
} catch {
    Write-Host "❌ Fehler beim Hochladen der JSON-Datei: $_"
}
