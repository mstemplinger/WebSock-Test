package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var (
	baseDir = filepath.Join(os.Getenv("PROGRAMDATA"), "ondeso", "workplace")
	logDir  = filepath.Join(baseDir, "logs")
)

// writeLog schreibt Log-Nachrichten in eine Datei
func writeLog(message string) {
	logFilePath := filepath.Join(logDir, "inbox_upload.log")
	logMessage := fmt.Sprintf("%s - %s\n", time.Now().Format("2006-01-02 15:04:05"), message)
	fmt.Println(logMessage)
	file, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		file.WriteString(logMessage)
		file.Close()
	}
}

// sendJSON lädt die JSON-Datei hoch
func sendJSON(jsonFile string, apiURL string) {
	// Dateiinhalt lesen
	data, err := os.ReadFile(jsonFile)
	if err != nil {
		writeLog(fmt.Sprintf("❌ Fehler: Konnte JSON-Datei nicht lesen: %s", err))
		os.Exit(1)
	}

	// Prüfen, ob die Datei ein UTF-8 BOM enthält und entfernen
	data = removeUTF8BOM(data)

	// JSON-Validierung
	var jsonData map[string]interface{}
	if err := json.Unmarshal(data, &jsonData); err != nil {
		writeLog("❌ Fehler: Ungültige JSON-Struktur")
		os.Exit(1)
	}
	writeLog("🔍 JSON-Datei erfolgreich geladen und validiert.")

	// Request vorbereiten
	writeLog(fmt.Sprintf("📤 Sende JSON-Datei an API: %s", apiURL))
	resp, err := http.Post(apiURL, "application/json", bytes.NewReader(data))
	if err != nil {
		writeLog(fmt.Sprintf("❌ Fehler beim Hochladen: %s", err))
		os.Exit(1)
	}
	defer resp.Body.Close()

	// Antwort auslesen
	body, _ := io.ReadAll(resp.Body)
	writeLog(fmt.Sprintf("✅ API-Antwort erhalten: %s", string(body)))
}

// Entfernt ein UTF-8 BOM, falls vorhanden
func removeUTF8BOM(data []byte) []byte {
	bom := []byte{0xEF, 0xBB, 0xBF} // UTF-8 BOM
	if len(data) >= 3 && data[0] == bom[0] && data[1] == bom[1] && data[2] == bom[2] {
		return data[3:] // Entfernt das BOM
	}
	return data
}

func main() {
	// CLI-Parameter definieren
	jsonFile := flag.String("json", "", "Pfad zur JSON-Datei")
	apiURL := flag.String("url", "http://85.215.147.108:5001/inbox", "URL der Inbox-API")
	flag.Parse()

	// Verzeichnisse erstellen
	os.MkdirAll(logDir, 0755)

	// Parameter prüfen
	if *jsonFile == "" {
		writeLog("❌ Fehler: Kein JSON-Dateipfad angegeben.")
		flag.Usage()
		os.Exit(1)
	}

	// JSON-Datei hochladen
	sendJSON(*jsonFile, *apiURL)
}
