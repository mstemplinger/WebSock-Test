package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mssql"
	"gopkg.in/natefinch/lumberjack.v2"
	"github.com/joho/godotenv"
)

// Constants
const (
	scriptDir = "scriptfile"
	chunkSize = 4000
)

// Client represents a connected WebSocket client.
type Client struct {
	ID       string
	Hostname string
	IP       string
	Conn     *websocket.Conn
}

// Global variables
var (
	clients         = make(map[string]Client)
	previousClients = make(map[string]map[string]interface{})
	clientsMutex    sync.RWMutex
	db              *gorm.DB
	upgrader        = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins.  For production, be more restrictive.
		},
	}
	appCtx    context.Context // For passing context to functions.
	templates *template.Template
)

// Models - GORM Models
type BaseModel struct {
	ID        uint      `gorm:"primary_key"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

type Inbox struct {
	BaseModel
	AcxInboxID               uint      `gorm:"primary_key;column:acx_inbox_id"`
	AcxInboxName             string    `gorm:"column:acx_inbox_name;size:255"`
	AcxInboxDescription      string    `gorm:"column:acx_inbox_description;size:1024"`
	AcxInboxCreator          string    `gorm:"column:acx_inbox_creator;size:255"`
	AcxInboxVendor           string    `gorm:"column:acx_inbox_vendor;size:255"`
	AcxInboxContentType      string    `gorm:"column:acx_inbox_content_type;size:255"`
	AcxInboxContent          string    `gorm:"column:acx_inbox_content;type:text"`
	AcxInboxProcessingState  string    `gorm:"column:acx_inbox_processing_state;size:50;default:'pending'"`
	AcxInboxProcessingStart  *time.Time `gorm:"column:acx_inbox_processing_start"`
	AcxInboxProcessingEnd    *time.Time `gorm:"column:acx_inbox_processing_end"`
	AcxInboxProcessingLog string     `gorm:"column:acx_inbox_processing_log;type:text"`
}

type Asset struct {
	BaseModel
	ClientID  string     `gorm:"column:client_id;size:255;unique_index"`
	Hostname  string     `gorm:"column:hostname;size:255"`
	IPAddress string     `gorm:"column:ip_address;size:50"`
	LastSeen  time.Time  `gorm:"column:last_seen"`
}

type ClientUser struct {
	BaseModel
	ClientID string `gorm:"column:client_id;size:255"`
	Username string `gorm:"column:username;size:255"`
}

// Helper function to get environment variable with a default value.
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// initDB initializes the database connection.
func initDB() *gorm.DB {
	connString := fmt.Sprintf("sqlserver://%s:%s@%s:%s?database=%s",
		getEnv("DB_USER", "sa"),
		getEnv("DB_PASSWORD", "MyStrongPassword123!"),
		getEnv("DB_HOST", "localhost"),
		getEnv("DB_PORT", "1433"),
		getEnv("DB_NAME", "mydatabase"),
	)

	var err error
	var db *gorm.DB

	for i := 0; i < 10; i++ {
		db, err = gorm.Open("mssql", connString)
		if err == nil {
			break
		}

		log.Printf("Attempt %d: Failed to connect to the database: %v", i+1, err)
		time.Sleep(5 * time.Second)
	}

	if err != nil {
		log.Fatalf("Failed to connect to database after multiple retries: %v", err)
		panic(err)
	}

	db.AutoMigrate(&Inbox{}, &Asset{}, &ClientUser{})
	db.LogMode(true)
	return db
}

// getTables returns a list of all tables in the database.
func getTables(db *gorm.DB) ([]string, error) {
	var tables []string
	rows, err := db.Raw("SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_TYPE = 'BASE TABLE'").Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}
		tables = append(tables, tableName)
	}
	return tables, nil
}

// getColumnLengths retrieves the maximum lengths of string columns in a given table.
func getColumnLengths(db *gorm.DB, tableName string) (map[string]int, error) {
	columnLengths := make(map[string]int)

	rows, err := db.Raw(`
        SELECT COLUMN_NAME, CHARACTER_MAXIMUM_LENGTH
        FROM INFORMATION_SCHEMA.COLUMNS
        WHERE TABLE_NAME = ? AND DATA_TYPE IN ('varchar', 'nvarchar', 'char', 'nchar')`, tableName).Rows()

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var columnName string
		var maxLength sql.NullInt64
		if err := rows.Scan(&columnName, &maxLength); err != nil {
			return nil, err
		}
		if maxLength.Valid {
			columnLengths[columnName] = int(maxLength.Int64)
		}
	}

	return columnLengths, nil
}

// truncateValues truncates string values based on column lengths.
func truncateValues(columnValues map[string]interface{}, columnLengths map[string]int) map[string]interface{} {
	for column, maxLength := range columnLengths {
		if val, ok := columnValues[column]; ok {
			if strVal, ok := val.(string); ok {
				if len(strVal) > maxLength {
					log.Printf("⚠️ Wert für `%s` wurde von %d auf %d Zeichen gekürzt!", column, len(strVal), maxLength)
					columnValues[column] = strVal[:maxLength]
				}
			}
		}
	}
	return columnValues
}

// processInbox processes entries in the Inbox table.
func processInbox(ctx context.Context) {
	log.Println("🟢 Starte `process_inbox`-Thread...")

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			processInboxEntries(ctx)
		case <-ctx.Done():
			log.Println("processInbox exiting due to context cancellation")
			return
		}
	}
}

func processInboxEntries(ctx context.Context) {
	log.Println("processInboxEntries triggered")
	var entries []Inbox
	if err := db.Where("acx_inbox_processing_state = ?", "pending").Find(&entries).Error; err != nil {
		log.Printf("❌ Fehler beim Abrufen von Inbox-Einträgen: %v", err)
		return
	}

	if len(entries) == 0 {
		log.Println("✅ Keine neuen Einträge zum Verarbeiten.")
		return
	}

	for _, entry := range entries {
		log.Printf("🔄 Verarbeite Inbox-ID: %d", entry.AcxInboxID)

		if err := db.Model(&entry).Updates(map[string]interface{}{
			"acx_inbox_processing_state": "running",
			"acx_inbox_processing_start": time.Now(),
		}).Error; err != nil {
			log.Printf("❌ Fehler beim Aktualisieren des Inbox-Eintrags %d: %v", entry.AcxInboxID, err)
			continue
		}

		err := processSingleInboxEntry(ctx, entry)

		if err != nil {
			log.Printf("❌ Fehler bei Inbox-ID %d: %v", entry.AcxInboxID, err)
			if err := db.Model(&entry).Updates(map[string]interface{}{
				"acx_inbox_processing_state": "error",
				"acx_inbox_processing_log":   err.Error(),
				"acx_inbox_processing_end":   time.Now(),
			}).Error; err != nil {
				log.Printf("❌ Fehler beim Aktualisieren des Fehlerstatus für Inbox-Eintrag %d: %v", entry.AcxInboxID, err)
			}
		} else {
			log.Printf("✅ Verarbeitung für Inbox-ID %d abgeschlossen!", entry.AcxInboxID)
			if err := db.Model(&entry).Updates(map[string]interface{}{
				"acx_inbox_processing_state": "success",
				"acx_inbox_processing_log":   "Verarbeitung erfolgreich",
				"acx_inbox_processing_end":   time.Now(),
			}).Error; err != nil {
				log.Printf("❌ Fehler beim Aktualisieren des Erfolgsstatus für Inbox-Eintrag %d: %v", entry.AcxInboxID, err)
			}
		}
	}
}

func processSingleInboxEntry(ctx context.Context, entry Inbox) error {
	var jsonContent map[string]interface{}
	if err := json.Unmarshal([]byte(entry.AcxInboxContent), &jsonContent); err != nil {
		return fmt.Errorf("❌ Fehler beim Dekodieren von JSON: %v", err)
	}

	contentSection, ok := jsonContent["Content"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("❌ `Content`-Bereich fehlt oder ist ungültig")
	}

	tableName, ok := contentSection["TableName"].(string)
	if !ok {
		return fmt.Errorf("❌ Tabellenname fehlt im Content-Bereich")
	}
	tableName = strings.TrimSpace(tableName)

	dataEntries, ok := contentSection["Data"].([]interface{})
	if !ok {
		return fmt.Errorf("❌ `Data`-Bereich fehlt oder ist ungültig")
	}

	mappings, ok := contentSection["FieldMappings"].([]interface{})
	if !ok {
		return fmt.Errorf("❌ `FieldMappings`-Bereich fehlt oder ist ungültig")
	}

	if len(tableName) == 0 || len(dataEntries) == 0 || len(mappings) == 0 {
		return fmt.Errorf("❌ Fehlende Daten oder Mappings im JSON")
	}

	columnLengths, err := getColumnLengths(db, tableName)
	if err != nil {
		return fmt.Errorf("❌ Fehler beim Abrufen der Spaltenlängen: %v", err)
	}

	for _, recordInterface := range dataEntries {
		record, ok := recordInterface.(map[string]interface{})
		if !ok {
			return fmt.Errorf("❌ Ungültiger Datensatz im JSON")
		}

		columnValues := make(map[string]interface{})
		log.Printf("📑 Verarbeite Datensatz: %v", record)

		for _, mappingInterface := range mappings {
			mapping, ok := mappingInterface.(map[string]interface{})
			if !ok {
				return fmt.Errorf("❌ Ungültiges Mapping im JSON")
			}

			dbFieldInterface, ok := mapping["TargetField"]
			if !ok {
				return fmt.Errorf("❌ `TargetField` fehlt in Mappings")
			}
			dbField, ok := dbFieldInterface.(string)
			if !ok {
				return fmt.Errorf("❌ `TargetField` ist kein String")
			}
			dbField = strings.TrimSpace(dbField)

			expressionInterface, ok := mapping["Expression"]
			if !ok {
				return fmt.Errorf("❌ `Expression` fehlt in Mappings für %s", dbField)
			}
			expression, ok := expressionInterface.(string)
			if !ok {
				return fmt.Errorf("❌ `Expression` ist kein String")
			}
			expression = strings.TrimSpace(expression)

			if len(dbField) == 0 {
				return fmt.Errorf("❌ `TargetField` fehlt in Mappings")
			}
			if len(expression) == 0 {
				return fmt.Errorf("❌ `Expression` fehlt für %s", dbField)
			}

			if expression == "NewGUID()" {
				columnValues[dbField] = uuid.New().String()
			} else if strings.HasPrefix(expression, "{") && strings.HasSuffix(expression, "}") {
				jsonField := strings.Trim(expression, "{}")
				columnValues[dbField] = record[jsonField]
			} else {
				columnValues[dbField] = expression
			}
			log.Printf("🔄 Mapping `%s`: `%s` -> `%v`", dbField, expression, columnValues[dbField])
		}

		columnValues = truncateValues(columnValues, columnLengths)

		if err := db.Table(tableName).Create(columnValues).Error; err != nil {
			return fmt.Errorf("❌ SQL-Fehler: %v", err)
		}
	}
	return nil
}

// isPortInUse checks if a port is in use.
func isPortInUse(port int) bool {
	address := fmt.Sprintf("0.0.0.0:%d", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return true
	}
	listener.Close()
	return false
}

// checkForRefresh checks if the client list has changed.
func checkForRefresh() {
	clientsMutex.RLock()
	currentClients := make(map[string]map[string]interface{})
	for id, client := range clients {
		currentClients[id] = map[string]interface{}{
			"hostname": client.Hostname,
			"ip":       client.IP,
		}
	}
	clientsMutex.RUnlock()

	prevJSON, _ := json.Marshal(previousClients)
	currJSON, _ := json.Marshal(currentClients)

	if !bytes.Equal(prevJSON, currJSON) {
		previousClients = currentClients

		clientsMutex.RLock()
		for _, client := range clients {
			if client.Conn != nil && !isClosed(client.Conn) {
				err := client.Conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"refresh"}`))
				if err != nil {
					log.Printf("Error sending refresh to %s: %v", client.ID, err)
				}
			}
		}
		clientsMutex.RUnlock()
	}
}

// isClosed checks if a WebSocket connection is closed.
func isClosed(conn *websocket.Conn) bool {
	conn.SetReadDeadline(time.Now())
	_, _, err := conn.ReadMessage()
	conn.SetReadDeadline(time.Time{})
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return false
		}
		return true
	}
	return false
}

// --- HTTP Handlers ---

func indexHandler(w http.ResponseWriter, r *http.Request) {
	clientsMutex.RLock()
	defer clientsMutex.RUnlock()

	err := templates.ExecuteTemplate(w, "index.html", map[string]interface{}{
		"clients": clients,
	})
	if err != nil {
		log.Printf("Error rendering index.html: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

func showClientsHandler(w http.ResponseWriter, r *http.Request) {
	// Assuming you have a ProjectDocu.html template. Adapt as needed.
    err := templates.ExecuteTemplate(w, "ProjectDocu.html", map[string]interface{}{
        "clients": clients, // You might or might not need clients here
    })
    if err != nil {
        log.Printf("Error rendering ProjectDocu.html: %v", err)
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }
}

func loadPageHandler(w http.ResponseWriter, r *http.Request) {
	filename := filepath.Clean(r.URL.Path[len("/page/"):]) // Extract filename, sanitize
	if filename == "" {
		http.NotFound(w, r)
		return
	}
	templateName := filename

	clientsMutex.RLock()
	tmplData := map[string]interface{}{
		"clients": clients,
	}
	clientsMutex.RUnlock()

	err := templates.ExecuteTemplate(w, templateName, tmplData)
	if err != nil {
		log.Printf("Error rendering %s: %v", templateName, err)
		// Distinguish between template not found and other errors
		if os.IsNotExist(err) {
			http.NotFound(w, r)
		} else {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}
}



func clientDetailsHandler(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id") // Get client_id from query parameter
	if clientID == "" {
		http.Error(w, "Client ID is required", http.StatusBadRequest)
		return
	}

	var client Asset
	if err := db.Where("client_id = ?", clientID).First(&client).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			http.Error(w, "Client not found", http.StatusNotFound)
		} else {
			log.Printf("Error retrieving client: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	tables, err := getTables(db)
	if err != nil {
		log.Printf("Error getting tables: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	err = templates.ExecuteTemplate(w, "client_details.html", map[string]interface{}{
		"client":    client,
		"tables":    tables,
		"client_id": clientID,
	})
	if err != nil {
		log.Printf("Error rendering client_details.html: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}



func getTablesAPIHandler(w http.ResponseWriter, r *http.Request) {
	tables, err := getTables(db)
	if err != nil {
		log.Printf("Error getting tables: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(tables)
}

func tableDataHandler(w http.ResponseWriter, r *http.Request) {
	tableName := r.URL.Query().Get("table_name")
    if tableName == "" {
        http.Error(w, "Table name is required", http.StatusBadRequest)
        return
    }

    tableExists := false
    allTables, err := getTables(db)
    if err != nil {
        log.Printf("Error getting tables: %v", err)
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }

    for _, table := range allTables {
        if table == tableName {
            tableExists = true
            break
        }
    }
    if !tableExists {
        log.Printf("Error: Table '%s' does not exist", tableName)
        http.Error(w, fmt.Sprintf("Table '%s' not found", tableName), http.StatusNotFound)
        return
    }


	rows, err := db.Table(tableName).Rows() // Use GORM's Rows() for raw query
	if err != nil {
		log.Printf("Error querying table %s: %v", tableName, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	columns, err := rows.Columns() // Get column names
	if err != nil {
		log.Printf("Error getting columns for table %s: %v", tableName, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Prepare data for template
	var data []map[string]interface{}
	for rows.Next() {
		rowValues := make([]interface{}, len(columns))
		rowValuePtrs := make([]interface{}, len(columns))
		for i := range rowValues {
			rowValuePtrs[i] = &rowValues[i]
		}

		if err := rows.Scan(rowValuePtrs...); err != nil {
			log.Printf("Error scanning row: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		rowData := make(map[string]interface{})
		for i, col := range columns {
			val := rowValues[i]
			b, ok := val.([]byte) // Convert []byte to string
			if ok {
				rowData[col] = string(b)
			} else {
				rowData[col] = val
			}
		}
		data = append(data, rowData)
	}


	err = templates.ExecuteTemplate(w, "table_data.html", map[string]interface{}{
		"table_name": tableName,
		"data":      data,
		"columns":   columns,
	})
	if err != nil {
		log.Printf("Error rendering table_data.html: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}



func inboxHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if len(data) == 0 {
		http.Error(w, "Empty request received", http.StatusBadRequest)
		return
	}

	jsonContent, err := json.Marshal(data)
	if err != nil {
		http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
		return
	}

	metaData, _ := data["MetaData"].(map[string]interface{}) // Safe type assertion
	name, _ := metaData["Name"].(string)                    // Safe type assertion
	description, _ := metaData["Description"].(string)      // Safe type assertion
	creator, _ := metaData["Creator"].(string)              // Safe type assertion
	vendor, _ := metaData["Vendor"].(string)                // Safe type assertion
	contentType, _ := metaData["ContentType"].(string)      // Safe type assertion

	newEntry := Inbox{
		AcxInboxName:        name,
		AcxInboxDescription: description,
		AcxInboxCreator:     creator,
		AcxInboxVendor:      vendor,
		AcxInboxContentType: contentType,
		AcxInboxContent:     string(jsonContent),
	}

	if err := db.Create(&newEntry).Error; err != nil {
		log.Printf("Error saving to Inbox: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Neuer JSON-Eintrag gespeichert in Inbox-ID: %d", newEntry.AcxInboxID)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "Daten erfolgreich gespeichert",
		"InboxID": newEntry.AcxInboxID,
	})
}



func getClientsHandler(w http.ResponseWriter, r *http.Request) {
	clientsMutex.RLock()
	defer clientsMutex.RUnlock()

	// Convert clients map to a serializable format
	serializableClients := make(map[string]map[string]interface{})
	for id, client := range clients {
		serializableClients[id] = map[string]interface{}{
			"hostname": client.Hostname,
			"ip":       client.IP,
		}
	}
	json.NewEncoder(w).Encode(serializableClients)
}


func sendMessageHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    clientID := r.FormValue("client_id")
    message := r.FormValue("message")

    if clientID == "" || message == "" {
        http.Error(w, "Client-ID or message missing", http.StatusBadRequest)
        return
    }

    clientsMutex.Lock() // Lock for writing
    client, ok := clients[clientID]
    if !ok {
        clientsMutex.Unlock()
        http.Error(w, "Client not found", http.StatusNotFound)
        return
    }

    if client.Conn == nil || isClosed(client.Conn) {
        delete(clients, clientID) // Remove disconnected client
        clientsMutex.Unlock()
        http.Error(w, "Client not connected", http.StatusGone) // 410 Gone
        return
    }

    messageData := map[string]interface{}{
        "action":  "message",
        "content": message,
    }
    msgJSON, _ := json.Marshal(messageData) // Ignoring marshal error for brevity

    err := client.Conn.WriteMessage(websocket.TextMessage, msgJSON)
    clientsMutex.Unlock() // Unlock after writing

    if err != nil {
        log.Printf("Error sending message to %s: %v", clientID, err)
        http.Error(w, "Error sending message", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
    fmt.Fprint(w, `{"status": "success", "message": "Nachricht gesendet"}`)

}




func sendMessageAllHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    message := r.FormValue("message")
    if message == "" {
        http.Error(w, "Message missing", http.StatusBadRequest)
        return
    }

    log.Printf("📤 Nachricht an alle Clients senden: %s", message)

    messageData := map[string]interface{}{
        "action":  "message",
        "content": message,
    }
    msgJSON, _ := json.Marshal(messageData) // Simplified error handling

    clientsMutex.Lock() // Lock for iterating and writing
    defer clientsMutex.Unlock()

    var errors int
    for clientID, client := range clients {
        if client.Conn != nil && !isClosed(client.Conn) {
            err := client.Conn.WriteMessage(websocket.TextMessage, msgJSON)
            if err != nil {
                log.Printf("❌ Fehler beim Senden an %s: %v", clientID, err)
                errors++
            } else {
                log.Printf("✅ Nachricht an %s gesendet.", clientID)
            }
        } else {
            log.Printf("⚠️ Client %s nicht mehr verbunden, entferne ihn.", clientID)
            delete(clients, clientID) // Remove disconnected clients
        }
    }

    if errors == 0 {
        w.WriteHeader(http.StatusOK)
        fmt.Fprint(w, `{"status": "success", "message": "Nachricht erfolgreich an alle gesendet"}`)
    } else {
        w.WriteHeader(http.StatusPartialContent) // 207 Partial Content
        fmt.Fprintf(w, `{"status": "partial_success", "message": "Nachricht an einige Clients fehlgeschlagen (%d Fehler)"}`, errors)
    }
}


func sendScriptHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    clientID := r.FormValue("client_id")
    scriptName := r.FormValue("script_name")
    scriptType := r.FormValue("script_type")

    if clientID == "" || scriptName == "" || scriptType == "" {
        http.Error(w, "Client-ID, script name, or script type missing", http.StatusBadRequest)
        return
    }

    scriptPath := filepath.Join(scriptDir, filepath.Clean(scriptName)) // Prevent path traversal
    if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
        http.Error(w, "Script not found", http.StatusNotFound)
        return
    }

    clientsMutex.Lock() // Lock for writing
    defer clientsMutex.Unlock()

    client, ok := clients[clientID]
    if !ok {
        http.Error(w, "Client not found", http.StatusNotFound)
        return
    }
    if client.Conn == nil || isClosed(client.Conn) {
        delete(clients, clientID)
        http.Error(w, "Client not connected", http.StatusGone)
        return
    }

    scriptContent, err := ioutil.ReadFile(scriptPath)
    if err != nil {
        log.Printf("Error reading script: %v", err)
        http.Error(w, "Error reading script", http.StatusInternalServerError)
        return
    }
	scriptContentBase64 := base64.StdEncoding.EncodeToString(scriptContent)
    totalChunks := (len(scriptContentBase64) + chunkSize - 1) / chunkSize

    for i := 0; i < totalChunks; i++ {
        start := i * chunkSize
        end := start + chunkSize
        if end > len(scriptContentBase64) {
            end = len(scriptContentBase64)
        }
        chunk := scriptContentBase64[start:end]

        chunkMessage := map[string]interface{}{
            "action":       "upload_script_chunk",
            "script_name":  scriptName,
            "chunk_index":  i,
            "total_chunks": totalChunks,
            "script_chunk": chunk,
            "script_type":  scriptType,
        }
        chunkJSON, _ := json.Marshal(chunkMessage) // Simplified error handling

        err := client.Conn.WriteMessage(websocket.TextMessage, chunkJSON)
        if err != nil {
            log.Printf("Error sending chunk to %s: %v", clientID, err)
             http.Error(w, "Error sending script chunk", http.StatusInternalServerError)
            return // Stop sending if there is error
        }
        log.Printf("📤 Gesendet: Chunk %d/%d (%d Bytes) an Client: %s", i+1, totalChunks, len(chunk), clientID)
    }
     w.WriteHeader(http.StatusOK)
     fmt.Fprint(w, "Skript in Chunks gesendet") // No need for JSON if no error
}


func sendScriptAllHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	scriptName := r.FormValue("script_name")
	scriptType := r.FormValue("script_type")

	if scriptName == "" || scriptType == "" {
		http.Error(w, "Script name or script type missing", http.StatusBadRequest)
		return
	}
	scriptPath := filepath.Join(scriptDir, filepath.Clean(scriptName))
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		http.Error(w, "Script not found", http.StatusNotFound)
		return
	}

	scriptContent, err := ioutil.ReadFile(scriptPath)
	if err != nil {
		log.Printf("Error reading script: %v", err)
		http.Error(w, "Error reading script", http.StatusInternalServerError)
		return
	}
	scriptContentBase64 := base64.StdEncoding.EncodeToString(scriptContent)

	scriptMessage := map[string]interface{}{
		"action":       "execute_script",
		"script_name":  scriptName,
		"script_content": scriptContentBase64,
		"script_type":  scriptType,
	}
	scriptJSON, _ := json.Marshal(scriptMessage)

	clientsMutex.Lock() // Lock for the entire client iteration
	defer clientsMutex.Unlock()

	for clientID, client := range clients {
		if client.Conn != nil && !isClosed(client.Conn) {
			err := client.Conn.WriteMessage(websocket.TextMessage, scriptJSON)
			if err != nil {
				log.Printf("❌ Fehler beim Senden an %s: %v", clientID, err)
			} else {
				log.Printf("✅ Skript an %s gesendet.", clientID)
			}
		} else { // Remove client if connection is closed.
			log.Printf("⚠️ Client %s nicht mehr verbunden, entferne ihn.", clientID)
			delete(clients,clientID)
		}
	}

	w.WriteHeader(http.StatusOK) // Indicate success
	fmt.Fprint(w, "Skript an alle gesendet")
}

func getScriptsHandler(w http.ResponseWriter, r *http.Request) {
	allowedExtensions := map[string]bool{
		".ps1": true, ".bat": true, ".py": true, ".sh": true, ".txt": true,
	}
	ignoredDirs := map[string]bool{
		"_obsolete_": true,
	}

	if _, err := os.Stat(scriptDir); os.IsNotExist(err) {
		http.Error(w, "Script directory not found", http.StatusInternalServerError)
		return
	}

	var scripts []map[string]string
	err := filepath.Walk(scriptDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err // Prevent walking into a directory we can't access
		}

		relPath, _ := filepath.Rel(scriptDir, path) // Relative path
		if relPath == "." { // Skip the root script directory itself
			return nil
		}

        dir, file := filepath.Split(relPath)
        dir = strings.TrimSuffix(dir, string(filepath.Separator)) // Remove trailing slash

        if ignoredDirs[dir] {
            if info.IsDir(){
                return filepath.SkipDir // Skip entire ignored directory
            }
            return nil
        }

        if dir != "" { // Check for subdirectories
            parts := strings.Split(dir, string(filepath.Separator))
            if len(parts) > 1 { // If in subdirectory > 1 deep
              return nil // Skip it.
            }
        }


		ext := filepath.Ext(file)
		if !info.IsDir() && allowedExtensions[ext] {
			scriptType := "text"
			switch ext {
			case ".ps1":
				scriptType = "powershell"
			case ".bat":
				scriptType = "bat"
			case ".py":
				scriptType = "python"
			case ".sh":
				scriptType = "linuxshell"
			}

            // Use forward slashes for consistency, even on Windows.
            normalizedPath := strings.ReplaceAll(relPath, "\\", "/")
            scripts = append(scripts, map[string]string{"name": normalizedPath, "type": scriptType})
		}
		return nil
	})

	if err != nil {
		log.Printf("Error walking the script directory: %v", err)
		http.Error(w, "Error retrieving scripts", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(scripts)
}



// --- WebSocket Handling ---
async def handleClient(websocket): # the Python-signature
func handleClient(conn *websocket.Conn) {
	clientIP := conn.RemoteAddr().String()
	log.Printf("🔌 Neuer Client verbunden von %s", clientIP)

	defer func() { // Ensure client is removed on disconnect
		conn.Close() // Close the connection

        clientsMutex.Lock()
        var disconnectedClientIDs []string
        for clientID, client := range clients {
            if client.Conn == conn {
                disconnectedClientIDs = append(disconnectedClientIDs, clientID)
                // Don't delete here. Delete outside the loop.
            }
        }

        for _, clientID := range disconnectedClientIDs {
            delete(clients, clientID)
            log.Printf("🚪 Entferne Client %s aus Clientspeicher...", clientID)

            // Find and remove the client from the 'acx_asset' table
            if err := db.Where("client_id = ?", clientID).Delete(&Asset{}).Error; err != nil {
				if !gorm.IsRecordNotFoundError(err){ // Log only real errors.
                	log.Printf("❌ Fehler beim Löschen von Client %s aus `acx_asset`: %v", clientID, err)
				}
            } else {
                log.Printf("✅ Client %s erfolgreich aus `acx_asset` entfernt.", clientID)
            }
        }
        clientsMutex.Unlock()
		checkForRefresh() // Check after removing client.
		log.Println("🛑 Client-Entfernung abgeschlossen.")
	}()


	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("❌ WebSocket-Verbindung mit %s geschlossen: %v", clientIP, err)
			}
			break // Exit loop on connection close/error
		}

		if messageType == websocket.TextMessage {
			log.Printf("📩 Eingehende Nachricht von %s: %s", clientIP, message)

			var data map[string]interface{}
			if err := json.Unmarshal(message, &data); err != nil {
				log.Printf("🚨 Ungültiges JSON von %s: %s", clientIP, message)
				conn.WriteMessage(websocket.TextMessage, []byte(`{"status": "error", "message": "Invalid JSON"}`))
				continue // Continue to the next message
			}

			action, ok := data["action"].(string)
			if !ok {
				log.Printf("⚠️ Unbekannte Aktion von %s: %v", clientIP, data)
				conn.WriteMessage(websocket.TextMessage, []byte(`{"status": "error", "message": "Unknown action"}`))
				continue
			}
			if action == "register" {
				clientID, _ := data["client_id"].(string)
				hostname, _ := data["hostname"].(string)
				ipAddress, _ := data["ip"].(string)

				if clientID == "" || hostname == "" || ipAddress == "" {
					log.Printf("⚠️ Ungültige Registrierungsdaten von %s: %v", clientIP, data)
					conn.WriteMessage(websocket.TextMessage, []byte(`{"status": "error", "message": "Invalid registration data"}`))
					continue
				}

				clientsMutex.Lock()
				clients[clientID] = Client{ID: clientID, Hostname: hostname, IP: ipAddress, Conn: conn}
				clientsMutex.Unlock()

                log.Printf("📥 Neuer Client zwischengespeichert: %s (%s, %s)", clientID, hostname, ipAddress)

				// Database operations within a function, using appCtx
				err = updateOrRegisterClient(appCtx, clientID, hostname, ipAddress)
                if err != nil {
                    log.Printf("❌ Fehler bei DB Operation für %s: %v", clientID, err)
                    // Send error to client, *but* continue (don't break the connection)
                    conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"status": "error", "message": "Database error: %v"}`, err)))
                   continue
                }

                // Send success response.
                response := map[string]string{"status": "registered"}
                responseJSON, _ := json.Marshal(response) // Ignore marshal error
                conn.WriteMessage(websocket.TextMessage, responseJSON)


				checkForRefresh()
				log.Printf("📤 Registrierungsbestätigung an %s (%s) gesendet", hostname, ipAddress)
            } else {
				log.Printf("⚠️ Unbekannte Aktion von %s: %v", clientIP, data)
				conn.WriteMessage(websocket.TextMessage, []byte(`{"status": "error", "message": "Unknown action"}`))
			}
		}
	}
}


func updateOrRegisterClient(ctx context.Context, clientID, hostname, ipAddress string) error {
    // Use a transaction to ensure atomicity
    tx := db.Begin()
    if tx.Error != nil {
        return tx.Error
    }
    defer func() { // Rollback in case of panic
        if r := recover(); r != nil {
            tx.Rollback()
        }
    }()


    var existingAsset Asset
    err := tx.Where("client_id = ?", clientID).First(&existingAsset).Error

    if err != nil && !gorm.IsRecordNotFoundError(err) {
        tx.Rollback() // Rollback on any error other than not found
        return err
    }

    if gorm.IsRecordNotFoundError(err) {
        log.Printf("🆕 Neues Asset wird erstellt für Client %s (%s, %s)", clientID, hostname, ipAddress)
        newAsset := Asset{ClientID: clientID, Hostname: hostname, IPAddress: ipAddress, LastSeen: time.Now()} // Set LastSeen on creation
        if err := tx.Create(&newAsset).Error; err != nil {
             tx.Rollback()
            return err
        }
    } else {
        log.Printf("🔄 Update bestehendes Asset: %s (Last Seen: %s)", existingAsset.ClientID, existingAsset.LastSeen)
        existingAsset.LastSeen = time.Now() // Update LastSeen
        existingAsset.Hostname = hostname     // Update Hostname
        existingAsset.IPAddress = ipAddress   // Update IPAddress
        if err := tx.Save(&existingAsset).Error; err != nil { //Use Save for updating
             tx.Rollback()
            return err
        }
    }

      if err := tx.Commit().Error; err != nil {
         return err // Commit error
     }
	log.Printf("✅ Client %s erfolgreich registriert oder aktualisiert in `acx_asset`", clientID)
    return nil
}

// --- Route Setup ---

func setupRoutes() {
	http.HandleFunc("/", indexHandler)
    http.HandleFunc("/showdocu", showClientsHandler) // Corrected handler name
	http.HandleFunc("/page/", loadPageHandler)
	http.HandleFunc("/client/", clientDetailsHandler)
	http.HandleFunc("/get_tables", getTablesAPIHandler)
	http.HandleFunc("/table/", tableDataHandler)
	http.HandleFunc("/inbox", inboxHandler)
	http.HandleFunc("/clients", getClientsHandler)
	http.HandleFunc("/send_message", sendMessageHandler)
	http.HandleFunc("/send_message_all", sendMessageAllHandler)
	http.HandleFunc("/send_script", sendScriptHandler)
	http.HandleFunc("/send_script_all", sendScriptAllHandler)
	http.HandleFunc("/get_scripts", getScriptsHandler)

	// Serve static files (Optional - if you need to serve CSS/JS locally)
	// http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
}

// --- Main Function ---

func main() {
	//env loading
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found") //  Don't fatal exit here.
	}

	// Initialize logging
	log.SetOutput(&lumberjack.Logger{
		Filename:   "server.log",
		MaxSize:    500, // megabytes
		MaxBackups: 3,
		MaxAge:     28,   //days
		Compress:   true, // disabled by default
	})
	log.Println("Server starting...")

	// Initialize database
	db = initDB()
	defer db.Close()

    // Initialize the context
    appCtx = context.Background()

	// Load templates
	var err error
	templates, err = template.ParseGlob("templates/*.html")
	if err != nil {
		log.Fatalf("Error parsing templates: %v", err) // Fatal error, exit
        panic(err) // Use panic for fatal errors in main
	}

	// Setup routes
	setupRoutes()

	// Start the inbox processing goroutine
	go processInbox(appCtx)

	// Start the WebSocket server in a goroutine
	go func() {
		http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Println("Upgrade error:", err)
				return
			}
			handleClient(conn)
		})

		if isPortInUse(8765) {
			log.Fatal("❌ FEHLER: Port 8765 ist bereits belegt! WebSocket-Server kann nicht gestartet werden.")
            panic("Port 8765 already in use") // Use panic for unrecoverable errors
		}
		log.Println("✅ WebSocket-Server läuft auf Port 8765...")
		log.Println("🟢 WebSocket-Server wartet auf Clients...")
		if err := http.ListenAndServe(":8765", nil); err != nil {
			log.Fatal("ListenAndServe (WebSocket): ", err)
            panic(err)
		}
	}()

	// Start the main HTTP server
	log.Println("✅ HTTP-Server läuft auf Port 5001...")
	if err := http.ListenAndServe(":5001", nil); err != nil {
		log.Fatal("ListenAndServe (HTTP): ", err)
        panic(err)
	}
}