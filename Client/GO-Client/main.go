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
	"regexp"
	"strings"
	"strconv"
	"sort"
	"syscall"
	"time"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	"github.com/gorilla/websocket"
)

//const serverURL = "ws://85.215.147.108:8765"

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
	serverURL    = "ws://85.215.147.108:8765"
	HideScriptWindow bool
)

// Initialisiert Logs, Verzeichnisse und Client-ID
func init() {
	createDirs()
	readConfig()
	cleanupOldLogs()
	clientID = getClientID()
	setupLogging()
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


// Liest Konfiguration aus client_config.ini
func readConfig() {
	if data, err := ioutil.ReadFile(clientCfg); err == nil {
		lines := bytes.Split(data, []byte("\n"))
		for _, line := range lines {
			lineStr := strings.TrimSpace(string(line))
			if strings.HasPrefix(lineStr, "logging=") {
				loggingLevel = strings.TrimSpace(strings.Split(lineStr, "=")[1])
			}
			if strings.HasPrefix(lineStr, "websockserver=") {
				serverURL = strings.TrimSpace(strings.Split(lineStr, "=")[1])
			}
			if strings.HasPrefix(lineStr, "HideScriptWindow=") {
				val := strings.TrimSpace(strings.Split(lineStr, "=")[1])
				HideScriptWindow = val == "1" || strings.ToLower(val) == "true"
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


// Liest oder generiert eine Client-ID
func getClientID() string {
	if data, err := ioutil.ReadFile(clientCfg); err == nil {
		lines := bytes.Split(data, []byte("\n"))
		for _, line := range lines {
			if bytes.HasPrefix(line, []byte("client_id=")) {
				clientID := string(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("client_id="))))
				writeLog("🔄 Verwende gespeicherte Client-ID: " + clientID)
				return clientID
			}
		}
	}

	newID := generateGUID()
	ioutil.WriteFile(clientCfg, []byte("[CLIENT]\nclient_id="+newID), 0644)
	writeLog("🆕 Neue Client-ID generiert: " + newID)
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
	logEntry := fmt.Sprintf("%s - %s", time.Now().Format("2006-01-02 15:04:05"), message)
	fmt.Println(logEntry)
	log.Println(logEntry)
}

// WebSocket-Verbindung aufbauen
func connectWebSocket() {
	for {
		var err error
		wsConn, _, err = websocket.DefaultDialer.Dial(serverURL, nil)
		if err != nil {
			writeLog(fmt.Sprintf("❌ Verbindung fehlgeschlagen: %v. Neuer Versuch in 5 Sekunden...", err))
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

// Registriert den Client
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
			writeLog(fmt.Sprintf("⚠️ Verbindung verloren: %v", err))
			time.Sleep(5 * time.Second)
			connectWebSocket()
			return
		}
		processMessage(msg)
	}
}

// Prüft Dateinamen und entfernt ungültige Zeichen
func sanitizeFilename(name string) string {
	re := regexp.MustCompile(`[^\w\.-]`)
	return re.ReplaceAllString(name, "_")
}

// Verarbeitet Nachrichten
func processMessage(msg []byte) {
	var data map[string]interface{}
	json.Unmarshal(msg, &data)

	switch data["action"] {
	case "message":
		content := data["content"].(string)
		writeLog(fmt.Sprintf("📩 Nachricht: %s", content))

		if content == "STOP" {
			writeLog("🛑 STOP-Befehl erhalten. Beende Programm...")
			exitChan <- true
		}

	case "upload_script_chunk":
		processIncomingChunk(data)
	}
}

// Verarbeitet Skript-Chunks
func processIncomingChunk(data map[string]interface{}) {
	scriptName := sanitizeFilename(data["script_name"].(string))
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

// Speichert und führt Skripte aus (UTF-8 BOM + Logging + automatische Fensterschließung)
func executeScript(scriptName string, chunks map[int]string, scriptType string) {
	// Setzt den vollständigen Skript-Inhalt zusammen
	fullScriptBase64 := ""
	for i := 0; i < len(chunks); i++ {
		fullScriptBase64 += chunks[i]
	}
	scriptContent, _ := base64.StdEncoding.DecodeString(fullScriptBase64)

	// Erzeugt Dateinamen mit Zeitstempel
	timestamp := time.Now().Format("20060102_150405")
	filePath := filepath.Join(scriptDir, timestamp+"_"+scriptName)

	// **PowerShell Base64 Direktverarbeitung**
	if scriptType == "powershell-base64" {
		writeLog("▶️ Direktes Ausführen des Base64-codierten PowerShell-Skripts im Speicher")

		// **Konvertiere UTF-8 nach UTF-16LE**
		utf16Encoder := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewEncoder()
		utf16Script, _, err := transform.String(utf16Encoder, string(scriptContent))
		if err != nil {
			writeLog(fmt.Sprintf("❌ Fehler bei UTF-16LE-Konvertierung: %v", err))
			return
		}

		// **Base64-Encodierung (UTF-16LE)**
		encodedScript := base64.StdEncoding.EncodeToString([]byte(utf16Script))

		// **PowerShell-Befehl erstellen**
		cmd := exec.Command("powershell.exe", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodedScript)

		// **Fenstersteuerung**
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: HideScriptWindow}

		// **Skript starten**
		err = cmd.Run()
		if err != nil {
			writeLog(fmt.Sprintf("❌ Fehler beim Starten des PowerShell-Skripts: %v", err))
		} else {
			writeLog("✅ PowerShell-Base64-Skript erfolgreich ausgeführt")
		}
		return
	}

	// **UTF-8 mit BOM speichern**
	utf8BOM := []byte{0xEF, 0xBB, 0xBF}
	contentWithBOM := append(utf8BOM, scriptContent...)

	err := ioutil.WriteFile(filePath, contentWithBOM, 0755)
	if err != nil {
		writeLog(fmt.Sprintf("❌ Fehler beim Speichern des Skripts: %v", err))
		return
	}
	writeLog(fmt.Sprintf("📄 Skript gespeichert (UTF-8 BOM): %s", filePath))

	// Log-Datei für Skriptausgabe
	outputLog := filepath.Join(logDir, timestamp+"_"+scriptName+".log")

	// Ermittelt den auszuführenden Befehl basierend auf dem Skripttyp
	var cmd *exec.Cmd

	switch scriptType {
	case "powershell":
		if HideScriptWindow {
			cmd = exec.Command("cmd.exe", "/c", "start", "/b", "powershell.exe", "-ExecutionPolicy", "Bypass", "-File", filePath, ">", outputLog, "2>&1", "&", "exit")
		} else {
			cmd = exec.Command("cmd.exe", "/c", "start", "powershell.exe", "-ExecutionPolicy", "Bypass", "-File", filePath, ">", outputLog, "2>&1", "&", "exit")

		}
	case "bat":
		if HideScriptWindow {
			cmd = exec.Command("cmd.exe", "/c", "start", "/b", "/wait", filePath, ">", outputLog, "2>&1", "&", "exit")
		} else {
			cmd = exec.Command("cmd.exe", "/c", "start", "/wait", filePath, ">", outputLog, "2>&1", "&", "exit")
		}
	case "python":
		if HideScriptWindow {
			cmd = exec.Command("cmd.exe", "/c", "start", "/b", "/wait", "python", filePath, ">", outputLog, "2>&1", "&", "exit")
		} else {
			cmd = exec.Command("cmd.exe", "/c", "start", "/wait", "python", filePath, ">", outputLog, "2>&1", "&", "exit")
		}
	case "linuxshell":
		if HideScriptWindow {
			cmd = exec.Command("cmd.exe", "/c", "start", "/b", "/wait", "bash", filePath, ">", outputLog, "2>&1", "&", "exit")
		} else {
			cmd = exec.Command("cmd.exe", "/c", "start", "/wait", "bash", filePath, ">", outputLog, "2>&1", "&", "exit")
		}
	default:
		writeLog(fmt.Sprintf("⚠️ Unbekannter Skripttyp: %s", scriptType))
		return
	}

	// **Fenster bleibt sichtbar, schließt sich aber nach Skript-Ende**
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: HideScriptWindow}

	// **Startet das Skript in einem neuen Fenster, ohne die Haupt-Go-Konsole zu blockieren**
	err = cmd.Start()
	if err != nil {
		writeLog(fmt.Sprintf("❌ Fehler beim Starten des Skripts: %v", err))
	} else {
		writeLog(fmt.Sprintf("✅ Skript gestartet (Fenster schließt automatisch): %s (Log: %s)", filePath, outputLog))
	}

	// **Kurz warten, damit sich die Anzeige in der Konsole normalisiert**
	time.Sleep(500 * time.Millisecond)
}

// Startet das Programm
func main() {
	writeLog("🚀 Starte WebSocket-Client...")
	go connectWebSocket()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigChan:
		writeLog("🔴 Beende Programm durch Benutzer-Interrupt...")
	case <-exitChan:
		writeLog("🔴 Beende Programm durch STOP-Nachricht...")
	}

	if wsConn != nil {
		wsConn.Close()
	}
}
