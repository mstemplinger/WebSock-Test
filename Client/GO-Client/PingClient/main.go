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

func getOSByTTL(ttl int) string {
	if osName, exists := knownTTLs[ttl]; exists {
		return osName
	}
	return "Unknown"
}

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

func getHostname(ip string) string {
	hosts, err := net.LookupAddr(ip)
	if err != nil || len(hosts) == 0 {
		return ""
	}
	return hosts[0]
}

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

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

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

func main() {
	listIfaces := flag.Bool("list-ifaces", false, "List available network interfaces")
	startIP := flag.String("start", "", "Start IP address")
	endIP := flag.String("end", "", "End IP address")
	output := flag.String("output", "scan_results.json", "Output JSON file")
	iface := flag.String("iface", "", "Network interface")
	flag.Parse()

	if *listIfaces {
		listInterfaces()
		return
	}

	ipList, err := generateIPRange(*startIP, *endIP)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("Using interface:", *iface)

	var wg sync.WaitGroup
	results := make(chan ScanResult, len(ipList))

	for _, ip := range ipList {
		wg.Add(1)
		go pingIP(ip, &wg, results)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var scanResults []ScanResult
	for res := range results {
		scanResults = append(scanResults, res)
	}

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
