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
	apiEndpoint   = "https://ondeso.online:5001/inbox" // Standard-API-Endpunkt
)

func init() {
	// Falls ein Argument übergeben wurde, wird es als API-Endpunkt genutzt
	if len(os.Args) > 1 {
		apiEndpoint = os.Args[1]
		writeLog(fmt.Sprintf("🔄 API-Endpunkt auf Parameterwert gesetzt: %s", apiEndpoint))
	} else {
		writeLog("🔹 Kein API-Endpunkt als Parameter angegeben, Standardwert wird verwendet.")
	}
}

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
		writeLog("⚠️ Fehler beim Ausführen des PowerShell-Befehls: " + err.Error())
		return ""
	}
	return strings.TrimSpace(string(output))
}

func collectSecurityData() ExportData {
	assetID := getClientID()

	data := SecurityData{
		ScanDate:   time.Now().Format(time.RFC3339),
		AssetID:    assetID,
		OSName:     runPowershellCommand("(Get-CimInstance Win32_OperatingSystem).Caption"),
		OSVersion:  runPowershellCommand("(Get-CimInstance Win32_OperatingSystem).Version"),
		OSLastBoot: runPowershellCommand("(Get-CimInstance Win32_OperatingSystem).LastBootUpTime | Get-Date -Format 'yyyy-MM-ddTHH:mm:ssZ'"),
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
			Data:      []SecurityData{data},
		},
	}
}

func saveJSON(data ExportData) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		writeLog("❌ Fehler beim JSON-Export: " + err.Error())
		return
	}
	os.WriteFile(jsonFilePath, jsonData, 0644)
	writeLog("✅ JSON-Datei gespeichert: " + jsonFilePath)
}

func uploadJSON() {
	file, err := os.Open(jsonFilePath)
	if err != nil {
		writeLog("❌ Fehler beim Öffnen der JSON-Datei: " + err.Error())
		return
	}
	defer file.Close()

	client := &http.Client{}
	request, err := http.NewRequest("POST", apiEndpoint, file)
	if err != nil {
		writeLog("❌ Fehler beim Erstellen der Anfrage: " + err.Error())
		return
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		writeLog("❌ Fehler beim Senden der JSON-Datei: " + err.Error())
		return
	}
	defer response.Body.Close()
	writeLog("✅ JSON erfolgreich hochgeladen!")
}

func main() {
	ensureDirectories(baseDir, securityDir, logDir)
	writeLog("🚀 Starte Sicherheits-Scan...")
	data := collectSecurityData()
	saveJSON(data)
	uploadJSON()
	writeLog("🎯 Sicherheits-Scan abgeschlossen!")
}
