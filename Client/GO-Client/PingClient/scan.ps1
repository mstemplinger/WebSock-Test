param(
    [switch]$listIfaces,
    [string]$start,
    [string]$end,
    [string]$output = "scan_results.json",
    [string]$iface
)

# Funktion zur Ermittlung des Betriebssystems anhand des TTL-Werts
function Get-OSByTTL {
    param(
        [int]$ttl
    )
    switch ($ttl) {
        64 { return "Linux" }
        128 { return "Windows" }
        255 { return "Cisco/Networking Devices" }
        default { return "Unknown" }
    }
}

# Funktion, um alle verfügbaren Netzwerkinterfaces aufzulisten
function List-Interfaces {
    Get-NetAdapter | ForEach-Object { Write-Output $_.Name }
}

# Funktion für den Reverse-DNS-Lookup
function Get-Hostname {
    param(
        [string]$ip
    )
    try {
        $hostEntry = [System.Net.Dns]::GetHostEntry($ip)
        return $hostEntry.HostName
    }
    catch {
        return ""
    }
}

# Funktion, um die MAC-Adresse via ARP-Tabelle zu ermitteln
function Get-MacAddress {
    param(
        [string]$ip
    )
    $arpOutput = arp -a
    foreach ($line in $arpOutput) {
        if ($line -match $ip) {
            $fields = $line -split "\s+"
            if ($fields.Length -ge 2 -and $fields[0] -eq $ip) {
                return $fields[1]
            }
        }
    }
    return ""
}

# Funktion, um ein IP-Ziel anzupingen und den TTL-Wert zu ermitteln.
function Ping-IP {
    param(
        [string]$ip
    )
    # Führe den Ping-Befehl aus und analysiere die Ausgabe
    $pingOutput = ping.exe -n 3 $ip
    $ttl = 0
    $received = $false
    foreach ($line in $pingOutput) {
        if ($line -match "TTL=(\d+)") {
            $ttl = [int]$Matches[1]
            $received = $true
            break
        }
    }
    if ($received) {
        $result = [PSCustomObject]@{
            IP       = $ip
            Hostname = Get-Hostname -ip $ip
            Mac      = Get-MacAddress -ip $ip
            OS       = Get-OSByTTL -ttl $ttl
            TTL      = $ttl
            Type     = "Ping"
        }
        return $result
    }
    else {
        return $null
    }
}

# Funktion zur Umwandlung einer IP in eine UInt32-Zahl
function Convert-IPToInt {
    param(
        [string]$ip
    )
    try {
        $bytes = [System.Net.IPAddress]::Parse($ip).GetAddressBytes()
        [Array]::Reverse($bytes)
        return [BitConverter]::ToUInt32($bytes, 0)
    }
    catch {
        return $null
    }
}

# Funktion zur Umwandlung einer UInt32-Zahl in eine IP-Adresse
function Convert-IntToIP {
    param(
        [UInt32]$int
    )
    $bytes = [BitConverter]::GetBytes($int)
    [Array]::Reverse($bytes)
    return [System.Net.IPAddress]::Parse(($bytes -join '.'))
}

# Funktion zur Erzeugung eines IP-Adressbereichs von startIP bis endIP
function Generate-IPRange {
    param(
        [string]$startIP,
        [string]$endIP
    )
    $ipList = @()
    $startInt = Convert-IPToInt -ip $startIP
    $endInt = Convert-IPToInt -ip $endIP
    if ($startInt -eq $null -or $endInt -eq $null) {
        throw "Ungültiges IP-Format"
    }
    for ($i = $startInt; $i -le $endInt; $i++) {
        $ipList += Convert-IntToIP -int $i
    }
    return $ipList
}

# Funktion zur Überprüfung, ob eine IP im Bereich zwischen startIP und endIP liegt
function Is-IPInRange {
    param(
        [string]$ip,
        [string]$startIP,
        [string]$endIP
    )
    $ipInt    = Convert-IPToInt -ip $ip
    $startInt = Convert-IPToInt -ip $startIP
    $endInt   = Convert-IPToInt -ip $endIP
    if ($ipInt -ge $startInt -and $ipInt -le $endInt) {
        return $true
    }
    else {
        return $false
    }
}

# Funktion zum Auslesen der ARP-Tabelle und Filtern der Einträge im Adressbereich
function Scan-ARPTable {
    param(
        [string]$startIP,
        [string]$endIP
    )
    $results = @()
    $arpOutput = arp -a
    foreach ($line in $arpOutput) {
        # Beispielzeile: "  192.168.1.1           00-11-22-33-44-55     dynamic"
        if ($line -match "(\d{1,3}(?:\.\d{1,3}){3})") {
            $ipFound = $Matches[1]
            $fields = $line -split "\s+"
            if ($fields.Length -ge 2) {
                $mac = $fields[1]
                if (Is-IPInRange -ip $ipFound -startIP $startIP -endIP $endIP) {
                    $obj = [PSCustomObject]@{
                        IP       = $ipFound
                        Hostname = Get-Hostname -ip $ipFound
                        Mac      = $mac
                        OS       = Get-OSByTTL -ttl 0  # TTL kann via ARP nicht ermittelt werden
                        TTL      = 0
                        Type     = "ARP"
                    }
                    $results += $obj
                }
            }
        }
    }
    return $results
}

# Hauptprogramm

if ($listIfaces) {
    List-Interfaces
    exit
}

if (-not $start -or -not $end) {
    Write-Output "Bitte sowohl --start als auch --end angeben"
    exit
}

Write-Output "Using interface: $iface"

try {
    $ipList = Generate-IPRange -startIP $start -endIP $end
}
catch {
    Write-Output $_.Exception.Message
    exit
}

$scanResults = @()
$existingIPs = @{}

# Jeden Host im Bereich pingen
foreach ($ip in $ipList) {
    $pingResult = Ping-IP -ip $ip
    if ($pingResult -ne $null) {
        $scanResults += $pingResult
        $existingIPs[$ip] = $true
    }
}

# ARP-Tabelle auslesen und ergänzen
$arpResults = Scan-ARPTable -startIP $start -endIP $end
foreach ($res in $arpResults) {
    if (-not $existingIPs.ContainsKey($res.IP)) {
        $scanResults += $res
    }
}

# Falls kein Ergebnis vorliegt, Dummy-Eintrag hinzufügen
if ($scanResults.Count -eq 0) {
    $dummy = [PSCustomObject]@{
        IP       = "N/A"
        Hostname = "Kein Host gefunden"
        Mac      = "N/A"
        OS       = "N/A"
        TTL      = 0
        Type     = "None"
    }
    $scanResults += $dummy
}

# Ergebnisse als JSON speichern
$scanResults | ConvertTo-Json -Depth 5 | Out-File -FilePath $output -Encoding utf8
Write-Output "Scan complete. Results saved to $output"
