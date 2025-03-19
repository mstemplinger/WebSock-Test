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

// ScanResult repräsentiert ein einzelnes Scan-Ergebnis.
// Das Feld LastSeen wird nur gesetzt, wenn das Endgerät per Ping erreichbar war.
type ScanResult struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
	Mac      string `json:"mac"`
	OS       string `json:"os"`
	TTL      int    `json:"ttl"`
	Type     string `json:"type"`
	LastSeen string `json:"last_seen,omitempty"`
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

// listInterfaces zeigt alle verfügbaren Netzwerkinterfaces an.
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

// getHostname führt einen Reverse-DNS-Lookup durch.
func getHostname(ip string) string {
	hosts, err := net.LookupAddr(ip)
	if err != nil || len(hosts) == 0 {
		return ""
	}
	return hosts[0]
}

// getMacAddress versucht, über das OS-ARP-Tool die MAC-Adresse abzurufen.
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
// Wird eine Antwort empfangen, so wird ein ScanResult mit aktuellem Zeitstempel (LastSeen) zurückgegeben.
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
			LastSeen: time.Now().UTC().Format(time.RFC3339),
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
		startIdx := strings.Index(line, "(")
		endIdx := strings.Index(line, ")")
		if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
			continue
		}
		ip := line[startIdx+1 : endIdx]

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

// --- Strukturen für das Output-Schema ---

type MetaData struct {
	Version     string `json:"Version"`
	ContentType string `json:"ContentType"`
	Name        string `json:"Name"`
	Creator     string `json:"Creator"`
	Description string `json:"Description"`
	Vendor      string `json:"Vendor"`
	Schema      string `json:"Schema"`
	Preview     string `json:"Preview"`
}

type ConstItem struct {
	Identifier string `json:"Identifier"`
	Value      string `json:"Value"`
}

type FieldMapping struct {
	TargetField  string `json:"TargetField"`
	Expression   string `json:"Expression"`
	IsIdentifier bool   `json:"IsIdentifier"`
	ImportField  bool   `json:"ImportField"`
}

type Content struct {
	TableName     string         `json:"TableName"`
	Consts        []ConstItem    `json:"Consts"`
	FieldMappings []FieldMapping `json:"FieldMappings"`
	Data          []ScanResult   `json:"Data"`
}

type OutputSchema struct {
	MetaData MetaData `json:"MetaData"`
	Content  Content  `json:"Content"`
}

// createNewOutputStruct erstellt ein neues OutputSchema mit den aktuellen Scan-Ergebnissen.
func createNewOutputStruct(data []ScanResult) OutputSchema {
	return OutputSchema{
		MetaData: MetaData{
			Version:     "1.0",
			ContentType: "db-import",
			Name:        "NetworkScan",
			Creator:     "ondeso",
			Description: "NetworkScan Ergebnisse",
			Vendor:      "ondeso GmbH",
			Schema:      "",
			Preview:     "",
		},
		Content: Content{
			TableName: "asm_asset",
			Consts: []ConstItem{
				{
					Identifier: "CaptureDate",
					Value:      time.Now().UTC().Format(time.RFC3339),
				},
			},
			FieldMappings: []FieldMapping{
				{TargetField: "ip", Expression: "{ip}", IsIdentifier: false, ImportField: true},
				{TargetField: "hostname", Expression: "{hostname}", IsIdentifier: false, ImportField: true},
				{TargetField: "mac", Expression: "{mac}", IsIdentifier: false, ImportField: true},
				{TargetField: "os", Expression: "{os}", IsIdentifier: false, ImportField: true},
				{TargetField: "ttl", Expression: "{ttl}", IsIdentifier: false, ImportField: true},
				{TargetField: "type", Expression: "{type}", IsIdentifier: false, ImportField: true},
				{TargetField: "last_seen", Expression: "{last_seen}", IsIdentifier: false, ImportField: true},
			},
			Data: data,
		},
	}
}

// --- main ---
func main() {
	listIfaces := flag.Bool("list-ifaces", false, "List available network interfaces")
	startIP := flag.String("start", "", "Start IP address")
	endIP := flag.String("end", "", "End IP address")
	output := flag.String("output", "scan_results.json", "Output JSON file")
	iface := flag.String("iface", "", "Network interface") // nur angezeigt, nicht genutzt
	updatelist := flag.Bool("updatelist", false, "Aktualisiere die bestehende Ergebnisliste")
	flag.Parse()

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

	fmt.Println("Using interface:", *iface)

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

	var scanResults []ScanResult
	existingIPs := make(map[string]bool)
	for res := range resultsChan {
		scanResults = append(scanResults, res)
		existingIPs[res.IP] = true
	}

	// ARP-Tabelle auslesen und ergänzen
	arpResults, err := scanARPTable(*startIP, *endIP)
	if err != nil {
		fmt.Println("Error scanning ARP table:", err)
	} else {
		for _, res := range arpResults {
			if !existingIPs[res.IP] {
				scanResults = append(scanResults, res)
			}
		}
	}

	// Falls kein Ergebnis vorliegt, Dummy-Eintrag hinzufügen
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

	var outputStruct OutputSchema

	// Falls --updatelist angegeben wurde und die Output-Datei existiert, mergen wir die alten Ergebnisse.
	if *updatelist {
		if _, err := os.Stat(*output); err == nil {
			fileData, err := os.ReadFile(*output)
			if err == nil {
				var previous OutputSchema
				if json.Unmarshal(fileData, &previous) == nil {
					// Mergen der alten Ergebnisse (key: IP)
					merged := make(map[string]ScanResult)
					for _, dev := range previous.Content.Data {
						merged[dev.IP] = dev
					}
					// Aktualisiere oder füge neue Geräte hinzu.
					for _, dev := range scanResults {
						// Bei PING-Ergebnissen wird der Zeitstempel aktualisiert.
						if dev.Type == "Ping" {
							dev.LastSeen = time.Now().UTC().Format(time.RFC3339)
						}
						merged[dev.IP] = dev
					}
					var mergedSlice []ScanResult
					for _, v := range merged {
						mergedSlice = append(mergedSlice, v)
					}
					previous.Content.Data = mergedSlice
					// Aktualisiere den CaptureDate in den Consts.
					if len(previous.Content.Consts) > 0 {
						previous.Content.Consts[0].Value = time.Now().UTC().Format(time.RFC3339)
					}
					outputStruct = previous
				} else {
					outputStruct = createNewOutputStruct(scanResults)
				}
			} else {
				outputStruct = createNewOutputStruct(scanResults)
			}
		} else {
			outputStruct = createNewOutputStruct(scanResults)
		}
	} else {
		outputStruct = createNewOutputStruct(scanResults)
	}

	jsonData, err := json.MarshalIndent(outputStruct, "", "  ")
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
