package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"gopkg.in/ini.v1"
)

var (
	baseDir       = filepath.Join(os.Getenv("PROGRAMDATA"), "ondeso", "workplace")
	logDir        = filepath.Join(baseDir, "logs")
	scriptDir     = filepath.Join(baseDir, "scriptfiles")
	clientCfgPath = filepath.Join(baseDir, "client_config.ini")
	logFilePath   = filepath.Join(logDir, "client_stream.log")
	apiEndpoint   = "https://85.215.147.108:5001/inbox"
	clientID      string
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

type UserData struct {
	ID            string `json:"id"`
	TransactionID string `json:"transaction_id"`
	AssetID       string `json:"asset_id"`
	Username      string `json:"username"`
	Client        string `json:"client"`
	UserCount     string `json:"usercount"`
	Permissions   string `json:"permissions"`
	SID           string `json:"sid"`
	FullName      string `json:"full_name"`
	AccountStatus string `json:"account_status"`
	LastLogon     string `json:"last_logon"`
	Description   string `json:"description"`
}

type Content struct {
	TableName     string         `json:"TableName"`
	Consts        []Const        `json:"Consts"`
	FieldMappings []FieldMapping `json:"FieldMappings"`
	Data          []UserData     `json:"Data"`
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
	writeLog("⚠️ Meine ClientID =" + clientID)
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

func collectUserData() ExportData {

	users := getWindowsUsers()

	return ExportData{
		MetaData: MetaData{
			ContentType: "db-import",
			Name:        "Windows User Import",
			Description: "Collect User Information",
			Version:     "1.0",
			Creator:     "ondeso",
			Vendor:      "ondeso GmbH",
			Preview:     "",
			Schema:      "",
		},
		Content: Content{
			TableName: "usr_client_users",
			Consts:    []Const{{Identifier: "CaptureDate", Value: time.Now().Format(time.RFC3339)}},
			FieldMappings: []FieldMapping{
				{"transaction_id", "{transaction_id}", true, true},
				{"asset_id", "{asset_id}", false, true},
				{"username", "{username}", false, true},
				{"client", "{client}", false, true},
				{"usercount", "{usercount}", false, true},
				{"permissions", "{permissions}", false, true},
				{"sid", "{sid}", false, true},
				{"full_name", "{full_name}", false, true},
				{"account_status", "{account_status}", false, true},
				{"last_logon", "{last_logon}", false, true},
				{"description", "{description}", false, true},
			},
			Data: users,
		},
	}
}

func saveJSON(data ExportData) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		writeLog("Failed to serialize JSON: " + err.Error())
		return
	}
	os.WriteFile(filepath.Join(scriptDir, "system_info.json"), jsonData, 0644)
	writeLog("JSON file saved: " + filepath.Join(scriptDir, "system_info.json"))
}

func getWindowsUsers() []UserData {
	var users []UserData

	cmd := exec.Command("powershell", "-Command", "Get-WmiObject Win32_UserAccount | Select-Object Name, SID, FullName, Status, Disabled | ConvertTo-Json -Compress")
	output, err := cmd.Output()
	if err != nil {
		writeLog("Failed to fetch Windows users: " + err.Error())
		return users
	}

	var rawUsers []map[string]interface{}
	err = json.Unmarshal(output, &rawUsers)
	if err != nil {
		writeLog("JSON parsing error: " + err.Error())
		return users
	}

	for _, u := range rawUsers {
		transactionID := uuid.New().String()
		//assetID := getClientID()
		users = append(users, UserData{
			ID:            uuid.New().String(),
			TransactionID: transactionID, //uuid.New().String(),
			AssetID:       getClientID(),
			Username:      fmt.Sprintf("%v", u["Name"]),
			SID:           fmt.Sprintf("%v", u["SID"]),
			FullName:      fmt.Sprintf("%v", u["FullName"]),
			AccountStatus: fmt.Sprintf("%v", u["Status"]),
			Description:   "Imported Windows User",
		})
	}
	return users
}

func sendToAPI(data ExportData) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		writeLog("Failed to serialize JSON for API: " + err.Error())
		return
	}

	req, err := http.NewRequest("POST", apiEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		writeLog("Failed to create API request: " + err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		writeLog("Failed to send JSON to API: " + err.Error())
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	writeLog("API Response: " + string(body))
}

func main() {
	ensureDirectories(baseDir, logDir, scriptDir)
	writeLog("Starting user information collection...")
	data := collectUserData()
	saveJSON(data)
	sendToAPI(data)
	writeLog("Process completed!")
}
