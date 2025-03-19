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

var debugEnabled bool

// debugLog gibt Debug-Nachrichten aus, wenn Debugging aktiviert ist.
func debugLog(format string, a ...interface{}) {
	if debugEnabled {
		fmt.Printf("[DEBUG] "+format+"\n", a...)
	}
}

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
	debugLog("getOSByTTL: received ttl = %d", ttl)
	if osName, exists := knownTTLs[ttl]; exists {
		debugLog("getOSByTTL: found osName = %s for ttl = %d", osName, ttl)
		return osName
	}
	debugLog("getOSByTTL: ttl = %d not found, returning Unknown", ttl)
	return "Unknown"
}

// listInterfaces zeigt alle verfügbaren Netzwerkinterfaces an.
func listInterfaces() {
	debugLog("listInterfaces: fetching network interfaces")
	interfaces, err := net.Interfaces()
	if err != nil {
		fmt.Println("Error getting network interfaces:", err)
		return
	}
	fmt.Println("Available network interfaces:")
	for _, iface := range interfaces {
		fmt.Printf("- %s\n", iface.Name)
		debugLog("listInterfaces: found interface %s", iface.Name)
	}
}

// getHostname führt einen Reverse-DNS-Lookup durch.
func getHostname(ip string) string {
	debugLog("getHostname: performing reverse DNS lookup for IP %s", ip)
	hosts, err := net.LookupAddr(ip)
	if err != nil || len(hosts) == 0 {
		debugLog("getHostname: lookup failed for IP %s: %v", ip, err)
		return ""
	}
	debugLog("getHostname: found hostname %s for IP %s", hosts[0], ip)
	return hosts[0]
}

// getMacAddress versucht, über das OS-ARP-Tool die MAC-Adresse abzurufen.
func getMacAddress(ip string) string {
	debugLog("getMacAddress: executing arp -a for IP %s", ip)
	cmd := exec.Command("arp", "-a", ip)
	output, err := cmd.Output()
	if err != nil {
		debugLog("getMacAddress: error executing arp command for IP %s: %v", ip, err)
		return ""
	}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		debugLog("getMacAddress: processing line: %s", line)
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == ip {
			debugLog("getMacAddress: found MAC %s for IP %s", fields[1], ip)
			return fields[1]
		}
	}
	debugLog("getMacAddress: MAC not found for IP %s", ip)
	return ""
}

// pingIP nutzt github.com/go-ping/ping, um ein IP-Ziel anzupingen.
// Wird eine Antwort empfangen, so wird ein ScanResult mit aktuellem Zeitstempel (LastSeen) zurückgegeben.
func pingIP(ip string, wg *sync.WaitGroup, results chan<- ScanResult) {
	defer wg.Done()
	debugLog("pingIP: starting ping for IP %s", ip)

	pinger, err := ping.NewPinger(ip)
	if err != nil {
		debugLog("pingIP: error creating pinger for IP %s: %v", ip, err)
		return
	}
	pinger.Count = 3
	pinger.Timeout = 2 * time.Second
	pinger.SetPrivileged(true)

	var ttl int
	pinger.OnRecv = func(pkt *ping.Packet) {
		debugLog("pingIP: received packet from %s with ttl %d", ip, pkt.Ttl)
		ttl = pkt.Ttl
	}

	err = pinger.Run()
	if err != nil {
		debugLog("pingIP: error running ping for IP %s: %v", ip, err)
		return
	}
	stats := pinger.Statistics()
	debugLog("pingIP: ping statistics for IP %s: sent=%d, received=%d", ip, stats.PacketsSent, stats.PacketsRecv)
	if stats.PacketsRecv > 0 {
		result := ScanResult{
			IP:       ip,
			Hostname: getHostname(ip),
			Mac:      getMacAddress(ip),
			OS:       getOSByTTL(ttl),
			TTL:      ttl,
			Type:     "Ping",
			LastSeen: time.Now().UTC().Format(time.RFC3339),
		}
		debugLog("pingIP: sending result for IP %s: %+v", ip, result)
		results <- result
	} else {
		debugLog("pingIP: no packets received for IP %s", ip)
	}
}

// inc inkrementiert eine IP-Adresse byteweise.
func inc(ip net.IP) {
	before := ip.String()
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
	after := ip.String()
	debugLog("inc: incremented IP from %s to %s", before, after)
}

// generateIPRange erzeugt alle IP-Adressen innerhalb eines Bereichs von Start- zu End-IP.
func generateIPRange(startIP, endIP string) ([]string, error) {
	debugLog("generateIPRange: generating range from %s to %s", startIP, endIP)
	ips := []string{}
	start := net.ParseIP(startIP)
	end := net.ParseIP(endIP)
	if start == nil || end == nil {
		debugLog("generateIPRange: invalid IP format for start=%s or end=%s", startIP, endIP)
		return nil, fmt.Errorf("invalid IP format")
	}
	for ip := start; !ip.Equal(end); inc(ip) {
		ips = append(ips, ip.String())
	}
	ips = append(ips, end.String())
	debugLog("generateIPRange: generated %d IP addresses", len(ips))
	return ips, nil
}

// ipToUint32 konvertiert eine IPv4-Adresse in eine uint32-Zahl.
func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	result := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
	debugLog("ipToUint32: converted IP %s to %d", ip.String(), result)
	return result
}

// isIPInRange prüft, ob die IP zwischen startIP und endIP liegt.
func isIPInRange(ipStr, startStr, endStr string) bool {
	debugLog("isIPInRange: checking if IP %s is in range %s - %s", ipStr, startStr, endStr)
	ip := net.ParseIP(ipStr).To4()
	start := net.ParseIP(startStr).To4()
	end := net.ParseIP(endStr).To4()
	if ip == nil || start == nil || end == nil {
		debugLog("isIPInRange: one of the IPs is invalid (ip: %v, start: %v, end: %v)", ip, start, end)
		return false
	}
	ipVal := ipToUint32(ip)
	startVal := ipToUint32(start)
	endVal := ipToUint32(end)
	inRange := ipVal >= startVal && ipVal <= endVal
	debugLog("isIPInRange: IP %s (value %d) in range %d-%d: %v", ipStr, ipVal, startVal, endVal, inRange)
	return inRange
}

// scanARPTable liest die ARP-Tabelle aus und liefert alle Einträge,
// deren IP im Bereich von startIP bis endIP liegt.
func scanARPTable(startIP, endIP string) ([]ScanResult, error) {
	debugLog("scanARPTable: scanning ARP table for range %s - %s", startIP, endIP)
	var results []ScanResult
	cmd := exec.Command("arp", "-a")
	output, err := cmd.Output()
	if err != nil {
		debugLog("scanARPTable: error executing arp command: %v", err)
		return nil, err
	}
	lines := strings.Split(string(output), "\n")
	debugLog("scanARPTable: processing %d lines from arp output", len(lines))
	for _, line := range lines {
		debugLog("scanARPTable: processing line: %s", line)
		startIdx := strings.Index(line, "(")
		endIdx := strings.Index(line, ")")
		if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
			debugLog("scanARPTable: skipping line due to missing IP info")
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
			debugLog("scanARPTable: no MAC found in line, skipping")
			continue
		}
		if isIPInRange(ip, startIP, endIP) {
			debugLog("scanARPTable: IP %s is in range, adding result", ip)
			results = append(results, ScanResult{
				IP:       ip,
				Hostname: getHostname(ip),
				Mac:      mac,
				OS:       getOSByTTL(0), // TTL ist aus ARP nicht ermittelbar
				TTL:      0,
				Type:     "ARP",
			})
		} else {
			debugLog("scanARPTable: IP %s is not in range", ip)
		}
	}
	debugLog("scanARPTable: found %d results", len(results))
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
	debugLog("createNewOutputStruct: creating new output schema with %d data entries", len(data))
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
	debug := flag.Bool("debug", false, "Enable debug output")
	flag.Parse()

	debugEnabled = *debug
	debugLog("main: debug enabled")

	if *listIfaces {
		debugLog("main: listing interfaces")
		listInterfaces()
		return
	}

	if *startIP == "" || *endIP == "" {
		fmt.Println("Bitte sowohl --start als auch --end angeben")
		debugLog("main: missing startIP or endIP")
		return
	}

	debugLog("main: generating IP range from %s to %s", *startIP, *endIP)
	ipList, err := generateIPRange(*startIP, *endIP)
	if err != nil {
		fmt.Println(err)
		debugLog("main: error generating IP range: %v", err)
		return
	}

	fmt.Println("Using interface:", *iface)
	debugLog("main: using interface %s", *iface)

	var wg sync.WaitGroup
	resultsChan := make(chan ScanResult, len(ipList))

	// Jeden Host im Bereich pingen
	for _, ip := range ipList {
		debugLog("main: scheduling ping for IP %s", ip)
		wg.Add(1)
		go pingIP(ip, &wg, resultsChan)
	}

	// Kanal schließen, sobald alle Goroutinen fertig sind
	go func() {
		wg.Wait()
		close(resultsChan)
		debugLog("main: all pings completed, channel closed")
	}()

	var scanResults []ScanResult
	existingIPs := make(map[string]bool)
	for res := range resultsChan {
		debugLog("main: received scan result: %+v", res)
		scanResults = append(scanResults, res)
		existingIPs[res.IP] = true
	}

	// ARP-Tabelle auslesen und ergänzen
	debugLog("main: scanning ARP table")
	arpResults, err := scanARPTable(*startIP, *endIP)
	if err != nil {
		fmt.Println("Error scanning ARP table:", err)
		debugLog("main: error scanning ARP table: %v", err)
	} else {
		for _, res := range arpResults {
			if !existingIPs[res.IP] {
				debugLog("main: adding ARP result for IP %s", res.IP)
				scanResults = append(scanResults, res)
			} else {
				debugLog("main: ARP result for IP %s already exists", res.IP)
			}
		}
	}

	// Falls kein Ergebnis vorliegt, Dummy-Eintrag hinzufügen
	if len(scanResults) == 0 {
		debugLog("main: no scan results found, adding dummy entry")
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
		debugLog("main: updatelist flag set, attempting to update existing file %s", *output)
		if _, err := os.Stat(*output); err == nil {
			debugLog("main: output file %s exists, reading file", *output)
			fileData, err := os.ReadFile(*output)
			if err == nil {
				var previous OutputSchema
				if json.Unmarshal(fileData, &previous) == nil {
					debugLog("main: successfully parsed previous output file")
					// Mergen der alten Ergebnisse (key: IP)
					merged := make(map[string]ScanResult)
					for _, dev := range previous.Content.Data {
						merged[dev.IP] = dev
						debugLog("main: previous device added: %s", dev.IP)
					}
					// Aktualisiere oder füge neue Geräte hinzu.
					for _, dev := range scanResults {
						if dev.Type == "Ping" {
							dev.LastSeen = time.Now().UTC().Format(time.RFC3339)
							debugLog("main: updating LastSeen for device %s", dev.IP)
						}
						merged[dev.IP] = dev
						debugLog("main: merged device: %s", dev.IP)
					}
					var mergedSlice []ScanResult
					for _, v := range merged {
						mergedSlice = append(mergedSlice, v)
					}
					previous.Content.Data = mergedSlice
					if len(previous.Content.Consts) > 0 {
						previous.Content.Consts[0].Value = time.Now().UTC().Format(time.RFC3339)
						debugLog("main: updated CaptureDate in previous output")
					}
					outputStruct = previous
				} else {
					debugLog("main: error parsing previous output file, creating new output")
					outputStruct = createNewOutputStruct(scanResults)
				}
			} else {
				debugLog("main: error reading output file, creating new output: %v", err)
				outputStruct = createNewOutputStruct(scanResults)
			}
		} else {
			debugLog("main: output file %s does not exist, creating new output", *output)
			outputStruct = createNewOutputStruct(scanResults)
		}
	} else {
		debugLog("main: updatelist flag not set, creating new output")
		outputStruct = createNewOutputStruct(scanResults)
	}

	jsonData, err := json.MarshalIndent(outputStruct, "", "  ")
	if err != nil {
		fmt.Println("Error generating JSON:", err)
		debugLog("main: error generating JSON: %v", err)
		return
	}

	err = os.WriteFile(*output, jsonData, 0644)
	if err != nil {
		fmt.Println("Error writing JSON file:", err)
		debugLog("main: error writing JSON file: %v", err)
		return
	}

	fmt.Printf("Scan complete. Results saved to %s\n", *output)
	debugLog("main: scan complete, results saved to %s", *output)
}
