package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	"github.com/gorilla/websocket"
	"gopkg.in/ini.v1"
)

//const serverURL = "ws://85.215.147.108:8765"

var (
	workplacePath    = flag.String("workplace", "", "Pfad zum Workplace-Verzeichnis setzen")
	baseDir          string
	logDir           string
	scriptDir        string
	clientCfg        string
	logFilePath      string
	clientID         string
	loggingLevel     = "normal" // Default: normal
	oldLogFiles      = 10       // Default: 10 Logfiles
	wsConn           *websocket.Conn
	scriptChunks     = make(map[string]map[int]string)
	scriptTotal      = make(map[string]int)
	exitChan         = make(chan bool)
	serverURL        = "ws://85.215.147.108:8765"
	HideScriptWindow bool
)

var defaultConfig = map[string]string{
	"logging":          "normal",
	"oldLogfiles":      "10",
	"websockserver":    "ws://85.215.147.108:8765",
	"HideScriptWindow": "1",
}

// **Initialisiert Pfade basierend auf CLI-Parameter**
func setupPaths() {
	// Falls `-workplace` angegeben wurde, wird dieser als `baseDir` genutzt
	if *workplacePath != "" {
		baseDir = *workplacePath
	} else {
		baseDir = filepath.Join(os.Getenv("PROGRAMDATA"), "ondeso", "workplace")
	}

	// Abhängige Pfade neu setzen
	logDir = filepath.Join(baseDir, "logs")
	scriptDir = filepath.Join(baseDir, "scriptfiles")
	clientCfg = filepath.Join(baseDir, "client_config.ini")
	logFilePath = filepath.Join(logDir, "client_stream.log")
}

// Initialisiert Logs, Verzeichnisse und Client-ID
func init() {
	flag.Parse()
	setupPaths()

	createDirs()
	createDefaultIniValues()
	readConfig()
	cleanupOldLogs()
	clientID = getClientID()
	setupLogging()
}

func createDefaultIniValues() {
	cfg, err := ini.Load(clientCfg)
	if err != nil {
		log.Printf("⚠️  Keine INI-Datei gefunden, erstelle neue Datei...")
		cfg = ini.Empty()
	}

	section := cfg.Section("CLIENT")

	for key, defaultValue := range defaultConfig {
		if !section.HasKey(key) {
			section.Key(key).SetValue(defaultValue)
		}
	}

	err = cfg.SaveTo(clientCfg)
	if err != nil {
		log.Fatalf("❌ Fehler beim Speichern der INI-Datei: %v", err)
	}

	fmt.Println("✅ INI-Datei erfolgreich aktualisiert!")
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
	writeLog("🔍 Lese Konfigurationsdatei: " + clientCfg)

	// INI-Datei laden
	cfg, err := ini.Load(clientCfg)
	if err != nil {
		writeLog("❌ Fehler beim Laden der INI-Datei: " + err.Error())
		return
	}

	// CLIENT-Abschnitt lesen
	section := cfg.Section("CLIENT")

	// Logging-Level setzen
	if value := section.Key("logging").String(); value != "" {
		loggingLevel = value
		writeLog(fmt.Sprintf("✅ Logging-Level gesetzt: %v", loggingLevel))
	} else {
		writeLog("⚠️ Kein Logging-Level gefunden, Standardwert wird verwendet: " + loggingLevel)
	}

	// WebSocket-Server
	if value := section.Key("websockserver").String(); value != "" {
		serverURL = value
		writeLog(fmt.Sprintf("✅ WebSocket-Server gesetzt: %s", serverURL))
	} else {
		writeLog("⚠️ Kein WebSocket-Server gefunden, Standardwert wird verwendet: " + serverURL)
	}

	// HideScriptWindow
	if value := section.Key("HideScriptWindow").String(); value != "" {
		HideScriptWindow = value == "1" || value == "true"
		writeLog(fmt.Sprintf("✅ HideScriptWindow gesetzt: %v", HideScriptWindow))
	} else {
		writeLog(fmt.Sprintf("⚠️ Kein HideScriptWindow-Wert gefunden, Standardwert wird verwendet: %v", HideScriptWindow))
	}

	// Anzahl der alten Logdateien
	if value := section.Key("oldLogfiles").String(); value != "" {
		if num, err := strconv.Atoi(value); err == nil && num > 0 {
			oldLogFiles = num
			writeLog(fmt.Sprintf("✅ Anzahl alter Logdateien gesetzt: %d", oldLogFiles))
		} else {
			writeLog(fmt.Sprintf("⚠️ Ungültiger Wert für oldLogfiles: %s. Standardwert (%d) wird verwendet.", value, oldLogFiles))
		}
	} else {
		writeLog(fmt.Sprintf("⚠️ Kein oldLogfiles-Wert gefunden, Standardwert wird verwendet: %d", oldLogFiles))
	}

	writeLog("✅ Konfigurationsdatei erfolgreich geladen.")
}

// Setzt Logging
func setupLogging() {
	if loggingLevel == "off" {
		log.SetOutput(io.Discard)
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
	files, err := os.ReadDir(logDir)
	if err != nil {
		return
	}

	var logFiles []os.DirEntry
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".log") {
			logFiles = append(logFiles, file)
		}
	}

	sort.Slice(logFiles, func(i, j int) bool {
		infoI, _ := logFiles[i].Info()
		infoJ, _ := logFiles[j].Info()
		return infoI.ModTime().After(infoJ.ModTime())
	})

	if len(logFiles) > oldLogFiles {
		for _, file := range logFiles[oldLogFiles:] {
			os.Remove(filepath.Join(logDir, file.Name()))
		}
	}
}

// Holt oder generiert eine Client-ID und speichert sie in der INI-Datei
func getClientID() string {
	cfg, err := ini.Load(clientCfg)
	if err != nil {
		writeLog("⚠️ Keine INI-Datei gefunden, erstelle neue Datei...")
		cfg = ini.Empty()
	}

	section := cfg.Section("CLIENT")
	clientID := section.Key("client_id").String()

	if clientID == "" {
		clientID = generateGUID()
		section.Key("client_id").SetValue(clientID)
		err = cfg.SaveTo(clientCfg)
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

	err := os.WriteFile(filePath, contentWithBOM, 0755)
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
	writeLog("🚀 Starte Programm mit Workplace: " + baseDir)
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
