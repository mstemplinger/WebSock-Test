package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
	wsConn       *websocket.Conn
	scriptChunks = make(map[string]map[int]string)
	scriptTotal  = make(map[string]int)
)

// Initialisiert Logs, Verzeichnisse und Client-ID
func init() {
	createDirs()
	clientID = getClientID()
}

// Erstellt Verzeichnisse für Logs und Skripte
func createDirs() {
	dirs := []string{baseDir, logDir, scriptDir}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			os.MkdirAll(dir, 0755)
		}
	}
}

// Generiert oder liest eine client_id
func getClientID() string {
	if data, err := ioutil.ReadFile(clientCfg); err == nil {
		lines := bytes.Split(data, []byte("\n"))
		for _, line := range lines {
			if bytes.HasPrefix(line, []byte("client_id=")) {
				return string(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("client_id="))))
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

// Ruft die IP-Adresse des Rechners ab
func getIPAddress() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "Unknown"
	}
	for _, addr := range addrs {
		if ip, ok := addr.(*net.IPNet); ok && !ip.IP.IsLoopback() {
			if ip.IP.To4() != nil {
				return ip.IP.String()
			}
		}
	}
	return "Unknown"
}

// Schreibt Logs
func writeLog(message string) {
	logEntry := fmt.Sprintf("%s - %s\n", time.Now().Format("2006-01-02 15:04:05"), message)
	fmt.Print(logEntry)
	ioutil.WriteFile(logFilePath, []byte(logEntry), os.ModeAppend)
}

// WebSocket-Verbindung aufbauen
func connectWebSocket() {
	for {
		var err error
		wsConn, _, err = websocket.DefaultDialer.Dial(serverURL, nil)
		if err != nil {
			writeLog(fmt.Sprintf("Verbindung fehlgeschlagen: %v. Neuer Versuch in 5 Sekunden...", err))
			time.Sleep(5 * time.Second)
			continue
		}
		writeLog("✅ Erfolgreich mit WebSocket verbunden!")

		if registerClient() {
			listenWebSocket()
		} else {
			writeLog("❌ Registrierung fehlgeschlagen, neuer Versuch in 5 Sekunden...")
			time.Sleep(5 * time.Second)
		}
	}
}

// Registriert den Client am Server
func registerClient() bool {
	data := map[string]string{
		"action":    "register",
		"client_id": clientID,
		"hostname":  os.Getenv("COMPUTERNAME"),
		"ip":        getIPAddress(),
	}
	jsonData, _ := json.Marshal(data)

	err := wsConn.WriteMessage(websocket.TextMessage, jsonData)
	if err != nil {
		writeLog(fmt.Sprintf("❌ Fehler bei Registrierung: %v", err))
		return false
	}

	_, msg, err := wsConn.ReadMessage()
	if err != nil {
		writeLog(fmt.Sprintf("❌ Fehler bei Antwort des Servers: %v", err))
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
			writeLog(fmt.Sprintf("Verbindung verloren: %v", err))
			time.Sleep(5 * time.Second)
			connectWebSocket()
			return
		}
		processMessage(msg)
	}
}

// Verarbeitet eingehende Nachrichten
func processMessage(msg []byte) {
	var data map[string]interface{}
	json.Unmarshal(msg, &data)

	switch data["action"] {
	case "message":
		content := data["content"].(string)
		if content == "STOP" {
			writeLog("🛑 STOP-Befehl erhalten. Beende Service...")
			os.Exit(0)
		} else {
			writeLog(fmt.Sprintf("📩 Nachricht: %s", content))
		}

	case "upload_script_chunk":
		processIncomingChunk(data)
	}
}

// Verarbeitet Skript-Chunks
func processIncomingChunk(data map[string]interface{}) {
	scriptName := data["script_name"].(string)
	chunkIndex := int(data["chunk_index"].(float64))
	totalChunks := int(data["total_chunks"].(float64))
	scriptChunk := data["script_chunk"].(string)
	scriptType := data["script_type"].(string)

	if _, exists := scriptChunks[scriptName]; !exists {
		scriptChunks[scriptName] = make(map[int]string)
		scriptTotal[scriptName] = totalChunks
	}
	scriptChunks[scriptName][chunkIndex] = scriptChunk

	if len(scriptChunks[scriptName]) == totalChunks {
		executeScript(scriptName, scriptChunks[scriptName], scriptType)
		delete(scriptChunks, scriptName)
		delete(scriptTotal, scriptName)
	}
}

// Setzt das Skript zusammen und führt es aus
func executeScript(scriptName string, chunks map[int]string, scriptType string) {
	fullScriptBase64 := ""
	for i := 0; i < len(chunks); i++ {
		fullScriptBase64 += chunks[i]
	}
	scriptContent, _ := base64.StdEncoding.DecodeString(fullScriptBase64)

	filePath := filepath.Join(scriptDir, scriptName)
	ioutil.WriteFile(filePath, scriptContent, 0755)
	writeLog(fmt.Sprintf("📄 Skript gespeichert: %s", filePath))

	switch scriptType {
	case "powershell":
		exec.Command("powershell.exe", "-ExecutionPolicy", "Bypass", "-File", filePath).Start()
	case "bat":
		exec.Command("cmd.exe", "/c", filePath).Start()
	case "python":
		exec.Command("python", filePath).Start()
	case "linuxshell":
		exec.Command("bash", filePath).Start()
	}
	writeLog(fmt.Sprintf("✅ Skript ausgeführt: %s", scriptName))
}

// Startet den WebSocket-Client
func main() {
	writeLog("🚀 Starte WebSocket-Dienst...")
	go connectWebSocket()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	writeLog("Beende Service...")
	if wsConn != nil {
		wsConn.Close()
	}
}
