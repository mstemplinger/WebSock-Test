package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/gorilla/websocket"
)

var (
	wsConn    *websocket.Conn
	exitChan  = make(chan bool)
	logField  *widget.MultiLineEntry
	serverURL = "ws://85.215.147.108:8765"
)

// Log-Funktion zur GUI-Anzeige
func writeLog(message string) {
	timestamp := time.Now().Format("15:04:05")
	logMsg := fmt.Sprintf("[%s] %s", timestamp, message)
	fmt.Println(logMsg)
	logField.SetText(logField.Text + "\n" + logMsg)
}

// WebSocket-Verbindung aufbauen
func connectWebSocket() {
	for {
		var err error
		wsConn, _, err = websocket.DefaultDialer.Dial(serverURL, nil)
		if err != nil {
			writeLog(fmt.Sprintf("❌ Verbindung fehlgeschlagen: %v", err))
			time.Sleep(5 * time.Second)
			continue
		}
		writeLog("✅ Erfolgreich mit WebSocket verbunden!")

		if registerClient() {
			listenWebSocket()
		} else {
			writeLog("❌ Registrierung fehlgeschlagen. Neuer Versuch in 5 Sekunden...")
			time.Sleep(5 * time.Second)
		}
	}
}

// Registriert den Client beim Server
func registerClient() bool {
	data := map[string]string{
		"action":   "register",
		"hostname": os.Getenv("COMPUTERNAME"),
		"ip":       getIPAddress(),
	}
	jsonData, _ := json.Marshal(data)

	err := wsConn.WriteMessage(websocket.TextMessage, jsonData)
	if err != nil {
		writeLog(fmt.Sprintf("❌ Fehler bei Registrierung: %v", err))
		return false
	}

	_, msg, err := wsConn.ReadMessage()
	if err != nil {
		writeLog(fmt.Sprintf("❌ Fehler bei Serverantwort: %v", err))
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
			writeLog("⚠️ Verbindung unterbrochen. Versuche erneut...")
			time.Sleep(5 * time.Second)
			connectWebSocket()
			return
		}
		writeLog(fmt.Sprintf("📩 Nachricht empfangen: %s", string(msg)))
	}
}

// Holt die IP-Adresse des Rechners
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

// Erstellt die GUI
func createUI() {
	app := app.New()
	window := app.NewWindow("WebSocket Client")
	window.Resize(fyne.NewSize(500, 400))

	logField = widget.NewMultiLineEntry()
	logField.SetMinRowsVisible(10)
	logField.Disable()

	btnStart := widget.NewButton("Start", func() {
		writeLog("🚀 Starte WebSocket-Client...")
		go connectWebSocket()
	})

	btnStop := widget.NewButton("Stop", func() {
		if wsConn != nil {
			wsConn.Close()
		}
		writeLog("🔴 WebSocket-Verbindung geschlossen")
	})

	container := container.NewVBox(
		btnStart,
		btnStop,
		logField,
	)
	window.SetContent(container)
	window.ShowAndRun()
}

// Startet die Anwendung
func main() {
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		if wsConn != nil {
			wsConn.Close()
		}
		os.Exit(0)
	}()

	createUI()
}
