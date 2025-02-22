#!/bin/bash

# 📌 API-Endpunkt
API_ENDPOINT="http://85.215.147.108:5001/inbox"

# 📂 Speicherort für Client-ID
CLIENT_ID_FILE="$HOME/.ondeso_client_id"

# 📂 Verzeichnis für temporäre Speicherung
TEMP_DIR="/tmp"
JSON_FILE="$TEMP_DIR/usr_user_info.json"

# 🖥️ Computernamen abrufen
CLIENT_NAME=$(hostname)

# 🔄 **Client-ID abrufen oder generieren**
if [[ -f "$CLIENT_ID_FILE" ]]; then
    ASSET_ID=$(cat "$CLIENT_ID_FILE" | tr -d '[:space:]')
else
    ASSET_ID=$(uuidgen)
    echo "$ASSET_ID" > "$CLIENT_ID_FILE"
    chmod 644 "$CLIENT_ID_FILE"
    echo "🆕 Neue Client-ID generiert und gespeichert: $ASSET_ID"
fi

# 🔄 Eindeutige Vorgangsnummer (TransactionID) generieren
TRANSACTION_ID=$(uuidgen)

# 📌 Alle Benutzer abrufen (ohne Systemkonten)
USER_LIST=$(awk -F':' '$3 >= 1000 && $1 != "nobody" {print $1}' /etc/passwd)
USER_COUNT=$(echo "$USER_LIST" | wc -l)

# ❌ Falls keine Benutzer vorhanden sind
if [[ -z "$USER_LIST" ]]; then
    echo "❌ Keine Benutzer gefunden, die importiert werden können."
    exit 1
fi

# 📌 **JSON-Kopf erstellen mit Asset-ID**
JSON_DATA=$(cat <<EOF
{
  "MetaData": {
    "ContentType": "db-import",
    "Name": "Linux User Import",
    "Description": "Collect User Information",
    "Version": "1.0",
    "Creator": "ondeso",
    "Vendor": "ondeso GmbH",
    "Preview": "",
    "Schema": ""
  },
  "Content": {
    "TableName": "usr_client_users",
    "FieldMappings": [
      {"TargetField": "asset_id", "Expression": "{AssetID}", "IsIdentifier": true, "ImportField": true},
      {"TargetField": "transaction_id", "Expression": "{TransactionID}", "IsIdentifier": true, "ImportField": true},
      {"TargetField": "username", "Expression": "{UserName}", "IsIdentifier": false, "ImportField": true},
      {"TargetField": "client", "Expression": "{ClientName}", "IsIdentifier": false, "ImportField": true},
      {"TargetField": "usercount", "Expression": "{UserCount}", "IsIdentifier": false, "ImportField": true},
      {"TargetField": "permissions", "Expression": "{Permissions}", "IsIdentifier": false, "ImportField": true},
      {"TargetField": "uid", "Expression": "{UID}", "IsIdentifier": false, "ImportField": true},
      {"TargetField": "gid", "Expression": "{GID}", "IsIdentifier": false, "ImportField": true},
      {"TargetField": "home_directory", "Expression": "{HomeDirectory}", "IsIdentifier": false, "ImportField": true},
      {"TargetField": "shell", "Expression": "{Shell}", "IsIdentifier": false, "ImportField": true}
    ],
    "Data": [
EOF
)

# 📌 Benutzer durchlaufen und JSON-Einträge hinzufügen
FIRST_ENTRY=true
for USER in $USER_LIST; do
    USER_UID=$(id -u "$USER")
    USER_GID=$(id -g "$USER")
    USER_HOME=$(eval echo ~$USER | jq -R '.' | sed 's/\"/\\\"/g')  # Home-Verzeichnis escapen
    USER_SHELL=$(getent passwd "$USER" | cut -d: -f7)

    # Gruppenmitgliedschaften abrufen (leere Werte durch "N/A" ersetzen)
    USER_GROUPS=$(groups "$USER" 2>/dev/null | cut -d: -f2 | sed 's/ /,/g')
    [[ -z "$USER_GROUPS" ]] && USER_GROUPS="N/A"
    
    [[ -z "$USER_SHELL" ]] && USER_SHELL="/bin/bash"  # Falls Shell leer ist, Default setzen

    # 🔄 JSON-Daten für den Benutzer hinzufügen
    if [ "$FIRST_ENTRY" = true ]; then
        FIRST_ENTRY=false
    else
        JSON_DATA+=","
    fi

    JSON_DATA+=$(cat <<EOF
      {
        "AssetID": "$ASSET_ID",
        "TransactionID": "$TRANSACTION_ID",
        "UserName": "$USER",
        "ClientName": "$CLIENT_NAME",
        "UserCount": "$USER_COUNT",
        "Permissions": "$USER_GROUPS",
        "UID": "$USER_UID",
        "GID": "$USER_GID",
        "HomeDirectory": "$USER_HOME",
        "Shell": "$USER_SHELL"
      }
EOF
)
done

# 📌 JSON-String abschließen
JSON_DATA+="] } }"

# 📌 JSON-Datei speichern (UTF-8 für Sonderzeichen)
echo "$JSON_DATA" | jq '.' > "$JSON_FILE" 2>/dev/null
if [ $? -ne 0 ]; then
    echo "❌ Fehler: Ungültiges JSON wurde generiert!"
    exit 1
fi

echo "✅ JSON-Datei gespeichert: $JSON_FILE"

# 📌 JSON an API senden
curl -X POST -H "Content-Type: application/json" -d @"$JSON_FILE" "$API_ENDPOINT"

if [ $? -eq 0 ]; then
    echo "✅ API-Upload erfolgreich!"
else
    echo "❌ Fehler beim API-Upload!"
fi
