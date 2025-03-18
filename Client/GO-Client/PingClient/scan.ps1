param(
    [switch]$listIfaces,
    [string]$start,
    [string]$end,
    [string]$output = "scan_results.json",
    [string]$iface
)

# Mapping von TTL zu OS
$global:knownTTLs = @{
    64  = "Linux"
    128 = "Windows"
    255 = "Cisco/Networking Devices"
}

function Get-OSByTTL {
    param([int]$ttl)
    if ($global:knownTTLs.ContainsKey($ttl)) {
        return $global:knownTTLs[$ttl]
    }
    return "Unknown"
}

# Listet verfügbare Netzwerkadapter
function List-Interfaces {
    Get-NetAdapter | ForEach-Object {
        Write-Output "- $($_.Name)"
    }
}

# Ermittelt den Hostnamen via DNS
function Get-Hostname {
    param([string]$ip)
    try {
        $entry = [System.Net.Dns]::GetHostEntry($ip)
        return $entry.HostName
    }
    catch {
        return ""
    }
}

# Ermittelt die MAC-Adresse über den arp-Befehl
function Get-MacAddress {
    param([string]$ip)
    $arpOutput = arp -a | Select-String $ip
    foreach ($line in $arpOutput) {
        $parts = $line -split "\s+"
        # Bei Windows steht die IP in der ersten Spalte
        if ($parts[0] -eq $ip -and $parts.Length -ge 2) {
            return $parts[1]
        }
    }
    return ""
}

# Konvertiert eine IP-Adresse (String) in einen Integer
function Convert-IPToInt {
    param([string]$ip)
    $parts = $ip.Split('.') | ForEach-Object { [int]$_ }
    return ($parts[0] -shl 24) -bor ($parts[1] -shl 16) -bor ($parts[2] -shl 8) -bor $parts[3]
}

# Konvertiert einen Integer in eine IP-Adresse (String)
function Convert-IntToIP {
    param([uint32]$int)
    $o1 = ($int -shr 24) -band 0xFF
    $o2 = ($int -shr 16) -band 0xFF
    $o3 = ($int -shr 8) -band 0xFF
    $o4 = $int -band 0xFF
    return "$o1.$o2.$o3.$o4"
}

# Erzeugt einen Bereich von IP-Adressen von $start bis $end (inklusive)
function Generate-IPRange {
    param(
        [string]$start,
        [string]$end
    )
    $ipList = @()
    $startInt = Convert-IPToInt $start
    $endInt   = Convert-IPToInt $end
    for ($i = $startInt; $i -le $endInt; $i++) {
        $ipList += Convert-IntToIP ([uint32]$i)
    }
    return $ipList
}

# Prüft, ob eine IP im Bereich zwischen $start und $end liegt
function Is-IPInRange {
    param(
        [string]$ip,
        [string]$start,
        [string]$end
    )
    $ipInt    = Convert-IPToInt $ip
    $startInt = Convert-IPToInt $start
    $endInt   = Convert-IPToInt $end
    return ($ipInt -ge $startInt -and $ipInt -le $endInt)
}

# Führt einen Ping-Versuch (3 Versuche, Timeout 2s) aus und liefert ein Ergebnisobjekt zurück, falls erfolgreich.
function Ping-IP {
    param([string]$ip)
    $ping = New-Object System.Net.NetworkInformation.Ping
    $success = $false
    $ttl = 0
    for ($i=0; $i -lt 3; $i++) {
        try {
            $reply = $ping.Send($ip, 2000)
            if ($reply.Status -eq "Success") {
                $ttl = $reply.Options.Ttl
                $success = $true
                break
            }
        }
        catch {
            continue
        }
    }
    if ($success) {
        return [PSCustomObject]@{
            ip       = $ip
            hostname = Get-Hostname $ip
            mac      = Get-MacAddress $ip
            os       = Get-OSByTTL $ttl
            ttl      = $ttl
            type     = "Ping"
        }
    }
    return $null
}

# Liest die ARP-Tabelle aus und liefert alle Einträge, deren IP im Bereich liegt.
function Scan-ARPTable {
    param(
        [string]$start,
        [string]$end
    )
    $results = @()
    $arpOutput = arp -a
    foreach ($line in $arpOutput) {
        # Für Windows: Zeilen enthalten z. B. "192.168.1.1           00-11-22-33-44-55     dynamic"
        if ($line -match "(\d{1,3}(?:\.\d{1,3}){3})\s+([0-9a-fA-F\-]{17})") {
            $ipFound  = $matches[1]
            $macFound = $matches[2]
            if (Is-IPInRange -ip $ipFound -start $start -end $end) {
                $results += [PSCustomObject]@{
                    ip       = $ipFound
                    hostname = Get-Hostname $ipFound
                    mac      = $macFound
                    os       = Get-OSByTTL 0  # TTL kann aus ARP nicht ermittelt werden
                    ttl      = 0
                    type     = "ARP"
                }
            }
        }
    }
    return $results
}

# --- Hauptprogramm ---

if ($listIfaces) {
    Write-Output "Verfügbare Netzwerkinterfaces:"
    List-Interfaces
    exit
}

if ([string]::IsNullOrEmpty($start) -or [string]::IsNullOrEmpty($end)) {
    Write-Output "Bitte sowohl --start als auch --end angeben"
    exit
}

$ipList = Generate-IPRange -start $start -end $end

if ($iface) {
    Write-Output "Using interface: $iface"
}

$scanResults = @()
$existingIPs = @{}

foreach ($ip in $ipList) {
    $result = Ping-IP -ip $ip
    if ($result -ne $null) {
        $scanResults += $result
        $existingIPs[$result.ip] = $true
    }
}

# ARP-Tabelle auslesen und Einträge ergänzen, falls noch nicht vorhanden
$arpResults = Scan-ARPTable -start $start -end $end
foreach ($res in $arpResults) {
    if (-not $existingIPs.ContainsKey($res.ip)) {
        $scanResults += $res
    }
}

# Falls kein Ergebnis vorliegt, füge einen Dummy-Eintrag ein
if ($scanResults.Count -eq 0) {
    $scanResults += [PSCustomObject]@{
        ip       = "N/A"
        hostname = "Kein Host gefunden"
        mac      = "N/A"
        os       = "N/A"
        ttl      = 0
        type     = "None"
    }
}

# Ergebnisse als JSON speichern
$scanResults | ConvertTo-Json -Depth 4 | Out-File $output -Encoding utf8
Write-Output "Scan complete. Results saved to $output"
