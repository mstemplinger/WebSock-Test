package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/go-ping/ping"
)

type ScanResult struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
	Mac      string `json:"mac"`
	OS       string `json:"os"`
	TTL      int    `json:"ttl"`
	Type     string `json:"type"`
}

var knownTTLs = map[int]string{
	64:  "Linux",
	128: "Windows",
	255: "Cisco/Networking Devices",
}

// getOSByTTL liefert anhand der Map ein mögliches OS zurück.
// Schlägt die Map fehl, wird "Unknown" ausgegeben.
func getOSByTTL(ttl int) string {
	if osName, exists := knownTTLs[ttl]; exists {
		return osName
	}
	return "Unknown"
}

// listInterfaces zeigt alle verfügbaren Netzwerkinterfaces an
func listInterfaces() {
	interfaces, err := net.Interfaces()
	if err != nil {
		fmt.Println("Error getting network interfaces:", err)
		return
	}
	fmt.Println("Available network interfaces:")
	for _, iface := range interfaces {
		fmt.Printf("- %s\n", iface.Name)
	}
}

// getHostname führt einen Reverse-DNS-Lookup durch
func getHostname(ip string) string {
	hosts, err := net.LookupAddr(ip)
	if err != nil || len(hosts) == 0 {
		return ""
	}
	return hosts[0]
}

// getMacAddress versucht, über das OS-ARP-Tool die MAC-Adresse abzurufen
func getMacAddress(ip string) string {
	cmd := exec.Command("arp", "-a", ip)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == ip {
			return fields[1]
		}
	}
	return ""
}

// pingIP nutzt github.com/go-ping/ping, um ein IP-Ziel anzupingen.
// Der TTL-Wert wird im Callback OnRecv erfasst.
func pingIP(ip string, wg *sync.WaitGroup, results chan<- ScanResult) {
	defer wg.Done()

	pinger, err := ping.NewPinger(ip)
	if err != nil {
		return
	}
	pinger.Count = 3
	pinger.Timeout = 2 * time.Second
	pinger.SetPrivileged(true)

	var ttl int
	pinger.OnRecv = func(pkt *ping.Packet) {
		ttl = pkt.Ttl
	}

	err = pinger.Run()
	if err != nil {
		return
	}
	stats := pinger.Statistics()
	if stats.PacketsRecv > 0 {
		results <- ScanResult{
			IP:       ip,
			Hostname: getHostname(ip),
			Mac:      getMacAddress(ip),
			OS:       getOSByTTL(ttl),
			TTL:      ttl,
			Type:     "Ping",
		}
	}
}

// inc inkrementiert eine IP-Adresse byteweise.
func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// generateIPRange erzeugt alle IP-Adressen innerhalb eines Bereichs von Start- zu End-IP.
func generateIPRange(startIP, endIP string) ([]string, error) {
	ips := []string{}
	start := net.ParseIP(startIP)
	end := net.ParseIP(endIP)
	if start == nil || end == nil {
		return nil, fmt.Errorf("invalid IP format")
	}

	for ip := start; !ip.Equal(end); inc(ip) {
		ips = append(ips, ip.String())
	}
	ips = append(ips, end.String())
	return ips, nil
}

// ipToUint32 konvertiert eine IPv4-Adresse in eine uint32-Zahl.
func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

// isIPInRange prüft, ob die IP zwischen startIP und endIP liegt.
func isIPInRange(ipStr, startStr, endStr string) bool {
	ip := net.ParseIP(ipStr).To4()
	start := net.ParseIP(startStr).To4()
	end := net.ParseIP(endStr).To4()
	if ip == nil || start == nil || end == nil {
		return false
	}
	ipVal := ipToUint32(ip)
	startVal := ipToUint32(start)
	endVal := ipToUint32(end)
	return ipVal >= startVal && ipVal <= endVal
}

// scanARPTable liest die ARP-Tabelle aus und liefert alle Einträge,
// deren IP im Bereich von startIP bis endIP liegt.
func scanARPTable(startIP, endIP string) ([]ScanResult, error) {
	var results []ScanResult
	cmd := exec.Command("arp", "-a")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		// Beispielzeile (Linux): "hostname (192.168.1.10) at 00:11:22:33:44:55 [ether] on eth0"
		startIdx := strings.Index(line, "(")
		endIdx := strings.Index(line, ")")
		if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
			continue
		}
		ip := line[startIdx+1 : endIdx]

		// MAC-Adresse ermitteln: Suche nach dem Wort "at"
		parts := strings.Fields(line)
		var mac string
		for i, part := range parts {
			if part == "at" && i+1 < len(parts) {
				mac = parts[i+1]
				break
			}
		}
		if mac == "" {
			continue
		}
		if isIPInRange(ip, startIP, endIP) {
			results = append(results, ScanResult{
				IP:       ip,
				Hostname: getHostname(ip),
				Mac:      mac,
				OS:       getOSByTTL(0), // TTL ist aus ARP nicht ermittelbar
				TTL:      0,
				Type:     "ARP",
			})
		}
	}
	return results, nil
}

func main() {
	listIfaces := flag.Bool("list-ifaces", false, "List available network interfaces")
	startIP := flag.String("start", "", "Start IP address")
	endIP := flag.String("end", "", "End IP address")
	output := flag.String("output", "scan_results.json", "Output JSON file")
	iface := flag.String("iface", "", "Network interface") // nur angezeigt, nicht genutzt
	flag.Parse()

	// Falls der Nutzer --list-ifaces übergibt, zeigen wir nur die Interfaces an.
	if *listIfaces {
		listInterfaces()
		return
	}

	if *startIP == "" || *endIP == "" {
		fmt.Println("Bitte sowohl --start als auch --end angeben")
		return
	}

	ipList, err := generateIPRange(*startIP, *endIP)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("Using interface:", *iface) // der Wert wird nur ausgegeben, nicht genutzt

	var wg sync.WaitGroup
	resultsChan := make(chan ScanResult, len(ipList))

	// Jeden Host im Bereich pingen
	for _, ip := range ipList {
		wg.Add(1)
		go pingIP(ip, &wg, resultsChan)
	}

	// Kanal schließen, sobald alle Goroutinen fertig sind
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Ergebnisse einsammeln
	var scanResults []ScanResult
	existingIPs := make(map[string]bool)
	for res := range resultsChan {
		scanResults = append(scanResults, res)
		existingIPs[res.IP] = true
	}

	// ARP-Tabelle auslesen und Einträge im Bereich ergänzen
	arpResults, err := scanARPTable(*startIP, *endIP)
	if err != nil {
		fmt.Println("Error scanning ARP table:", err)
	} else {
		for _, res := range arpResults {
			// Falls die IP noch nicht in den Ping-Ergebnissen auftaucht, hinzufügen
			if !existingIPs[res.IP] {
				scanResults = append(scanResults, res)
			}
		}
	}

	// Falls kein Ergebnis vorliegt, füge einen Dummy-Eintrag ein
	if len(scanResults) == 0 {
		scanResults = append(scanResults, ScanResult{
			IP:       "N/A",
			Hostname: "Kein Host gefunden",
			Mac:      "N/A",
			OS:       "N/A",
			TTL:      0,
			Type:     "None",
		})
	}

	// JSON-Datei erzeugen
	jsonData, err := json.MarshalIndent(scanResults, "", "  ")
	if err != nil {
		fmt.Println("Error generating JSON:", err)
		return
	}

	err = os.WriteFile(*output, jsonData, 0644)
	if err != nil {
		fmt.Println("Error writing JSON file:", err)
		return
	}

	fmt.Printf("Scan complete. Results saved to %s\n", *output)
}
