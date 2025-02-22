#!/bin/bash

# API-Endpunkt
API_ENDPOINT="http://85.215.147.108:5001/inbox"

# Verzeichnis für temporäre Speicherung
TEMP_DIR="/tmp"
JSON_FILE="$TEMP_DIR/usr_user_info.json"

# Computernamen abrufen
CLIENT_NAME=$(hostname)

# Eindeutige Vorgangsnummer (TransactionID) generieren
TRANSACTION_ID=$(uuidgen)

# Alle Benutzer abrufen (ohne Systemkonten)
USER_LIST=$(awk -F':' '$3 >= 1000 && $1 != "nobody" {print $1}' /etc/passwd)
USER_COUNT=$(echo "$USER_LIST" | wc -l)

# JSON-Daten erstellen
JSON_DATA='{
  "MetaData": {
    "ContentType": "db-import",
    "Name": "Linux User Import",
    "Description": "Collect User Information",
    "Version": "1.0",
    "Creator": "FL",
    "Vendor": "ondeso GmbH",
    "Preview": "",
    "Schema": ""
  },
  "Content": {
    "TableName": "usr_client_users",
    "Mappings": [
      {"TargetField": "id", "Expression": "NewGUID()"},
      {"TargetField": "transaction_id", "Expression": "{TransactionID}"},
      {"TargetField": "username", "Expression": "{UserName}"},
      {"TargetField": "client", "Expression": "{ClientName}"},
      {"TargetField": "usercount", "Expression": "{UserCount}"},
      {"TargetField": "permissions", "Expression": "{Permissions}"},
      {"TargetField": "uid", "Expression": "{UID}"},
      {"TargetField": "gid", "Expression": "{GID}"},
      {"TargetField": "home_directory", "Expression": "{HomeDirectory}"},
      {"TargetField": "shell", "Expression": "{Shell}"}
    ],
    "Fields": [
      {"Field": "TransactionID", "Type": "string"},
      {"Field": "UserName", "Type": "string"},
      {"Field": "ClientName", "Type": "string"},
      {"Field": "UserCount", "Type": "string"},
      {"Field": "Permissions", "Type": "string"},
      {"Field": "UID", "Type": "integer"},
      {"Field": "GID", "Type": "integer"},
      {"Field": "HomeDirectory", "Type": "string"},
      {"Field": "Shell", "Type": "string"}
    ],
    "Data": [
'

# Benutzer durchlaufen und JSON-Einträge hinzufügen
FIRST_ENTRY=true
for USER in $USER_LIST; do
    # Benutzerinformationen abrufen
    USER_UID=$(id -u "$USER")
    USER_GID=$(id -g "$USER")
    USER_HOME=$(eval echo ~$USER)
    USER_SHELL=$(getent passwd "$USER" | cut -d: -f7)
    
    # Gruppenmitgliedschaften abrufen
    USER_GROUPS=$(groups "$USER" | cut -d: -f2 | sed 's/ /,/g')

    # Komma bei weiteren Einträgen setzen
    if [ "$FIRST_ENTRY" = true ]; then
        FIRST_ENTRY=false
    else
        JSON_DATA+=','
    fi

    # Benutzerobjekt zum JSON hinzufügen
    JSON_DATA+='{
      "TransactionID": "'"$TRANSACTION_ID"'",
      "UserName": "'"$USER"'",
      "ClientName": "'"$CLIENT_NAME"'",
      "UserCount": "'"$USER_COUNT"'",
      "Permissions": "'"$USER_GROUPS"'",
      "UID": "'"$USER_UID"'",
      "GID": "'"$USER_GID"'",
      "HomeDirectory": "'"$USER_HOME"'",
      "Shell": "'"$USER_SHELL"'"
    }'
done

# JSON-String abschließen
JSON_DATA+=']} }'

# JSON-Datei speichern
echo "$JSON_DATA" > "$JSON_FILE"
echo "✅ JSON-Datei gespeichert: $JSON_FILE"

# JSON an API senden
curl -X POST -H "Content-Type: application/json" -d @"$JSON_FILE" "$API_ENDPOINT"

if [ $? -eq 0 ]; then
    echo "✅ API-Upload erfolgreich!"
else
    echo "❌ Fehler beim API-Upload!"
fi
