package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/ini.v1"
)

var (
	baseDir       = filepath.Join(os.Getenv("PROGRAMDATA"), "ondeso", "workplace")
	securityDir   = filepath.Join(baseDir, "security")
	logDir        = filepath.Join(baseDir, "logs")
	clientCfgPath = filepath.Join(baseDir, "client_config.ini")
	logFilePath   = filepath.Join(logDir, "security_scan.log")
	jsonFilePath  = filepath.Join(securityDir, "security_inventory.json")
	apiEndpoint   = "https://85.215.147.108:5001/inbox"
)

type MetaData struct {
	ContentType string `json:"ContentType"`
	Name        string `json:"Name"`
	Description string `json:"Description"`
	Version     string `json:"Version"`
	Creator     string `json:"Creator"`
	Vendor      string `json:"Vendor"`
	Preview     string `json:"Preview"`
	Schema      string `json:"Schema"`
}

type Const struct {
	Identifier string `json:"Identifier"`
	Value      string `json:"Value"`
}

type FieldMapping struct {
	TargetField  string `json:"TargetField"`
	Expression   string `json:"Expression"`
	IsIdentifier bool   `json:"IsIdentifier"`
	ImportField  bool   `json:"ImportField"`
}

type SecurityData struct {
	ScanDate        string `json:"scan_date"`
	AssetID         string `json:"asset_id"`
	OSName          string `json:"os_name"`
	OSVersion       string `json:"os_version"`
	OSLastBoot      string `json:"os_last_boot"`
	FirewallStatus  string `json:"firewall_status"`
	Antivirus       string `json:"antivirus_installed"`
	WindowsDefender string `json:"windows_defender"`
	BitLocker       string `json:"bitlocker_status"`
	UACStatus       string `json:"uac_status"`
	LocalAdmins     string `json:"local_admins"`
	RemoteDesktop   string `json:"remote_desktop"`
	SMBStatus       string `json:"smb_status"`
	GuestAccounts   string `json:"guest_accounts"`
	UserAccounts    string `json:"user_accounts"`
	OpenPorts       string `json:"open_ports"`
	LogonEvents     string `json:"logon_events"`
	FailedLogins    string `json:"failed_logins"`
	LastPatchDate   string `json:"last_patch_date"`
}

type Content struct {
	TableName     string         `json:"TableName"`
	Consts        []Const        `json:"Consts"`
	FieldMappings []FieldMapping `json:"FieldMappings"`
	Data          []SecurityData `json:"Data"`
}

type ExportData struct {
	MetaData MetaData `json:"MetaData"`
	Content  Content  `json:"Content"`
}

func ensureDirectories(paths ...string) {
	for _, path := range paths {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			os.MkdirAll(path, 0755)
		}
	}
}

func writeLog(message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logEntry := fmt.Sprintf("%s - %s\n", timestamp, message)
	f, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		f.WriteString(logEntry)
	}
	fmt.Println(logEntry)
}

// Holt oder generiert eine Client-ID und speichert sie in der INI-Datei
func getClientID() string {
	cfg, err := ini.Load(clientCfgPath)
	if err != nil {
		writeLog("⚠️ Keine INI-Datei gefunden, erstelle neue Datei...")
		cfg = ini.Empty()
	}

	section := cfg.Section("CLIENT")
	clientID := section.Key("client_id").String()

	if clientID == "" {
		clientID = generateGUID()
		section.Key("client_id").SetValue(clientID)
		err = cfg.SaveTo(clientCfgPath)
		if err != nil {
			writeLog("❌ Fehler beim Speichern der INI-Datei: " + err.Error())
		}
		writeLog("🆕 Neue Client-ID generiert: " + clientID)
	} else {
		writeLog("🔄 Verwende gespeicherte Client-ID: " + clientID)
	}

	return clientID
}

// Erstellt eine GUID
func generateGUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%12x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func runPowershellCommand(command string) string {
	cmd := exec.Command("powershell", "-Command", command)
	output, err := cmd.Output()
	if err != nil {
		writeLog("Failed to execute command: " + err.Error())
		return ""
	}
	return strings.TrimSpace(string(output))
}

func collectSecurityData() ExportData {
	assetID := getClientID()

	data := SecurityData{
		ScanDate:        time.Now().Format(time.RFC3339),
		AssetID:         assetID,
		OSName:          runPowershellCommand("(Get-CimInstance Win32_OperatingSystem).Caption"),
		OSVersion:       runPowershellCommand("(Get-CimInstance Win32_OperatingSystem).Version"),
		OSLastBoot:      runPowershellCommand("(Get-CimInstance Win32_OperatingSystem).LastBootUpTime | Get-Date -Format 'yyyy-MM-ddTHH:mm:ssZ'"),
		FirewallStatus:  runPowershellCommand("Get-NetFirewallProfile | Select-Object Name, Enabled | ConvertTo-Json -Compress"),
		Antivirus:       runPowershellCommand("Get-CimInstance -Namespace root\\SecurityCenter2 -ClassName AntiVirusProduct | Select-Object -ExpandProperty displayName"),
		WindowsDefender: runPowershellCommand("Get-MpComputerStatus | Select-Object -ExpandProperty AMRunningMode"),
		BitLocker:       runPowershellCommand("Get-BitLockerVolume | ForEach-Object { if ($_.ProtectionStatus -eq 1) { 'Enabled' } else { 'Disabled' } }"),
		UACStatus:       runPowershellCommand("(Get-ItemProperty -Path HKLM:\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Policies\\System -Name EnableLUA).EnableLUA"),
		LocalAdmins:     runPowershellCommand("Get-LocalGroupMember Administrators | Select-Object -ExpandProperty Name"),
		RemoteDesktop:   runPowershellCommand("(Get-ItemProperty -Path HKLM:\\System\\CurrentControlSet\\Control\\Terminal Server -Name fDenyTSConnections).fDenyTSConnections"),
		SMBStatus:       runPowershellCommand("(Get-ItemProperty -Path HKLM:\\SYSTEM\\CurrentControlSet\\Services\\LanmanServer\\Parameters -Name SMB1).SMB1"),
		GuestAccounts:   runPowershellCommand("Get-LocalGroupMember Guests | Select-Object -ExpandProperty Name"),
		UserAccounts:    runPowershellCommand("Get-LocalUser | Select-Object -ExpandProperty Name"),
		OpenPorts:       runPowershellCommand("Get-NetTCPConnection | Select-Object -ExpandProperty LocalPort | Sort-Object -Unique"),
		FailedLogins:    runPowershellCommand("Get-WinEvent -LogName Security -FilterHashtable @{Id=4625} -MaxEvents 10"),
		LastPatchDate:   runPowershellCommand("(Get-HotFix | Sort-Object InstalledOn -Descending | Select-Object -First 1).InstalledOn"),
	}

	return ExportData{
		MetaData: MetaData{
			ContentType: "db-import",
			Name:        "Windows Security Inventory",
			Description: "Sicherheitsbezogenes Inventar eines Windows-PCs",
			Version:     "1.0",
			Creator:     "FL",
			Vendor:      "ondeso GmbH",
		},
		Content: Content{
			TableName: "usr_security_inventory",
			Consts:    []Const{{Identifier: "ScanDate", Value: time.Now().Format(time.RFC3339)}},
			FieldMappings: []FieldMapping{
				{"scan_date", "{scan_date}", true, true},
				{"asset_id", "{asset_id}", false, true},
				{"os_name", "{os_name}", false, true},
				{"os_version", "{os_version}", false, true},
				{"os_last_boot", "{os_last_boot}", false, true},
				{"firewall_status", "{firewall_status}", false, true},
				{"antivirus_installed", "{antivirus_installed}", false, true},
				{"windows_defender", "{windows_defender}", false, true},
				{"bitlocker_status", "{bitlocker_status}", false, true},
				{"uac_status", "{uac_status}", false, true},
				{"local_admins", "{local_admins}", false, true},
				{"remote_desktop", "{remote_desktop}", false, true},
				{"smb_status", "{smb_status}", false, true},
				{"guest_account", "{guest_account}", false, true},
				{"user_accounts", "{user_accounts}", false, true},
				{"open_ports", "{open_ports}", false, true},
				{"failed_logins", "{failed_logins}", false, true},
				{"local_admins", "{local_admins}", false, true},
				{"last_patch_date", "{last_patch_date}", false, true},
			},
			Data: []SecurityData{data},
		},
	}
}

func saveJSON(data ExportData) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		writeLog("Failed to serialize JSON: " + err.Error())
		return
	}
	os.WriteFile(jsonFilePath, jsonData, 0644)
	writeLog("JSON file saved: " + jsonFilePath)
}

func uploadJSON() {
	file, err := os.Open(jsonFilePath)
	if err != nil {
		writeLog("Failed to open JSON file: " + err.Error())
		return
	}
	defer file.Close()

	client := &http.Client{}
	request, err := http.NewRequest("POST", apiEndpoint, file)
	if err != nil {
		writeLog("Failed to create request: " + err.Error())
		return
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		writeLog("Failed to send JSON: " + err.Error())
		return
	}
	defer response.Body.Close()
	writeLog("JSON uploaded successfully!")
}

func main() {
	ensureDirectories(baseDir, securityDir, logDir)
	writeLog("Starting security scan...")
	data := collectSecurityData()
	saveJSON(data)
	uploadJSON()
	writeLog("Security scan completed!")
}
