package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const serverURL = "ws://85.215.147.108:8765"

var (
	baseDir      = filepath.Join(os.Getenv("PROGRAMDATA"), "ondeso", "workplace")
	logDir       = filepath.Join(baseDir, "logs")
	scriptDir    = filepath.Join(baseDir, "scriptfiles")
	clientCfg    = filepath.Join(baseDir, "client_config.ini")
	logFilePath  = filepath.Join(logDir, "client_stream.log")
	clientID     string
	loggingLevel = "normal" // Default: normal
	oldLogFiles  = 10        // Default: 10 Logfiles
	wsConn       *websocket.Conn
	scriptChunks = make(map[string]map[int]string)
	scriptTotal  = make(map[string]int)
	exitChan     = make(chan bool)
)

// Initialisierung
func init() {
	createDirs()
	readConfig()
	setupLogging()
	cleanupOldLogs()
	clientID = getClientID()
}

// Erstellt Verzeichnisse
func createDirs() {
	dirs := []string{baseDir, logDir, scriptDir}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			os.MkdirAll(dir, 0755)
		}
	}
}

// Liest Konfiguration aus client_config.ini
func readConfig() {
	if data, err := ioutil.ReadFile(clientCfg); err == nil {
		lines := bytes.Split(data, []byte("\n"))
		for _, line := range lines {
			lineStr := strings.TrimSpace(string(line))
			if strings.HasPrefix(lineStr, "logging=") {
				loggingLevel = strings.TrimSpace(strings.Split(lineStr, "=")[1])
			}
			if strings.HasPrefix(lineStr, "oldLogfiles=") {
				val := strings.TrimSpace(strings.Split(lineStr, "=")[1])
				if num, err := strconv.Atoi(val); err == nil && num > 0 {
					oldLogFiles = num
				}
			}
		}
	}
}

// Setzt Logging
func setupLogging() {
	if loggingLevel == "off" {
		log.SetOutput(ioutil.Discard)
		return
	}

	file, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("❌ Konnte Log-Datei nicht öffnen: %v", err)
	}
	log.SetOutput(file)
}

// Löscht alte Log-Dateien
func cleanupOldLogs() {
	files, err := ioutil.ReadDir(logDir)
	if err != nil {
		return
	}

	var logFiles []os.FileInfo
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".log") {
			logFiles = append(logFiles, file)
		}
	}

	sort.Slice(logFiles, func(i, j int) bool {
		return logFiles[i].ModTime().After(logFiles[j].ModTime())
	})

	if len(logFiles) > oldLogFiles {
		for _, file := range logFiles[oldLogFiles:] {
			os.Remove(filepath.Join(logDir, file.Name()))
		}
	}
}

// Schreibt Logs je nach Logging-Level
func writeLog(message string) {
	if loggingLevel == "off" {
		return
	}

	logEntry := fmt.Sprintf("%s - %s", time.Now().Format("2006-01-02 15:04:05"), message)
	if loggingLevel == "verbose" {
		fmt.Println(logEntry) // Mehr Debug-Infos
	}
	log.Println(logEntry)
}

// Generiert oder liest die Client-ID
func getClientID() string {
	if data, err := ioutil.ReadFile(clientCfg); err == nil {
		lines := bytes.Split(data, []byte("\n"))
		for _, line := range lines {
			if strings.HasPrefix(string(line), "client_id=") {
				return strings.TrimSpace(strings.TrimPrefix(string(line), "client_id="))
			}
		}
	}

	newID := generateGUID()
	ioutil.WriteFile(clientCfg, []byte("[CLIENT]\nclient_id="+newID), 0644)
	return newID
}

// Erstellt eine GUID
func generateGUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%12x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// WebSocket-Verbindung herstellen
func connectWebSocket() {
	for {
		var err error
		wsConn, _, err = websocket.DefaultDialer.Dial(serverURL, nil)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		if registerClient() {
			listenWebSocket()
		} else {
			time.Sleep(5 * time.Second)
		}
	}
}

// Registriert den Client beim Server
func registerClient() bool {
	data := map[string]string{
		"action":    "register",
		"client_id": clientID,
		"hostname":  os.Getenv("COMPUTERNAME"),
	}

	jsonData, _ := json.Marshal(data)
	err := wsConn.WriteMessage(websocket.TextMessage, jsonData)
	if err != nil {
		return false
	}

	_, msg, err := wsConn.ReadMessage()
	if err != nil {
		return false
	}

	var response map[string]string
	json.Unmarshal(msg, &response)

	return response["status"] == "registered"
}

// Lauscht auf WebSocket-Nachrichten
func listenWebSocket() {
	for {
		_, msg, err := wsConn.ReadMessage()
		if err != nil {
			time.Sleep(5 * time.Second)
			connectWebSocket()
			return
		}
		processMessage(msg)
	}
}

// Verarbeitet Nachrichten
func processMessage(msg []byte) {
	var data map[string]interface{}
	json.Unmarshal(msg, &data)

	switch data["action"] {
	case "message":
		content := data["content"].(string)
		if content == "STOP" {
			exitChan <- true
		}
	case "upload_script_chunk":
		processIncomingChunk(data)
	}
}

// Speichert und führt Skripte aus
func processIncomingChunk(data map[string]interface{}) {
	scriptName := fmt.Sprintf("%s_%s.ps1", time.Now().Format("20060102_150405"), data["script_name"].(string))
	scriptPath := filepath.Join(scriptDir, scriptName)

	scriptContent, _ := base64.StdEncoding.DecodeString(data["script_chunk"].(string))

	// UTF-8 BOM hinzufügen
	utf8BOM := []byte{0xEF, 0xBB, 0xBF}
	fullScriptContent := append(utf8BOM, scriptContent...)

	err := ioutil.WriteFile(scriptPath, fullScriptContent, 0755)
	if err != nil {
		writeLog(fmt.Sprintf("❌ Fehler beim Speichern des Skripts: %v", err))
		return
	}

	cmd := exec.Command("powershell.exe", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
	err = cmd.Start()
	if err != nil {
		writeLog(fmt.Sprintf("❌ Fehler beim Starten des Skripts: %v", err))
	}
}

func main() {
	go connectWebSocket()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigChan:
	case <-exitChan:
	}

	if wsConn != nil {
		wsConn.Close()
	}
}
