#!/bin/bash

# API-Endpunkt
API_ENDPOINT="http://85.215.147.108:5001/inbox"

# JSON-Datei
JSON_FILE="/tmp/system_info.json"

# Generiere eine eindeutige Vorgangsnummer (UUID)
TRANSACTION_ID=$(uuidgen)

# Erfassungszeitpunkt (ISO 8601)
CAPTURE_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Systeminformationen sammeln
OS_NAME=$(lsb_release -ds 2>/dev/null || grep PRETTY_NAME /etc/os-release | cut -d '"' -f2)
KERNEL_VERSION=$(uname -r)
CPU_MODEL=$(lscpu | grep "Model name" | awk -F': ' '{print $2}')
CPU_CORES=$(nproc)
RAM_TOTAL=$(free -m | awk '/Mem:/ {printf "%.2f GB", $2/1024}')
DISK_TOTAL=$(df -h / | awk 'NR==2 {print $2}')
DISK_FREE=$(df -h / | awk 'NR==2 {print $4}')
IP_ADDRESS=$(hostname -I | tr '\n' ' ' | sed 's/ $//')
MAC_ADDRESS=$(ip link show | awk '/ether/ {print $2}' | head -n 1)

# JSON-Datenstruktur erstellen
cat <<EOF > $JSON_FILE
{
    "MetaData": {
        "ContentType": "db-import",
        "Name": "Linux System Information",
        "Description": "Erfasst Systeminformationen",
        "Version": "1.0",
        "Creator": "FL",
        "Vendor": "ondeso GmbH",
        "Preview": "",
        "Schema": ""
    },
    "Content": {
        "TableName": "usr_system_info",
        "Consts": [
            {
                "Identifier": "CaptureDate",
                "Value": "$CAPTURE_DATE"
            }
        ],
        "FieldMappings": [
            { "TargetField": "transaction_id", "Expression": "{transaction_id}", "IsIdentifier": true, "ImportField": true },
            { "TargetField": "os_name", "Expression": "{os_name}", "IsIdentifier": false, "ImportField": true },
            { "TargetField": "kernel_version", "Expression": "{kernel_version}", "IsIdentifier": false, "ImportField": true },
            { "TargetField": "cpu_model", "Expression": "{cpu_model}", "IsIdentifier": false, "ImportField": true },
            { "TargetField": "cpu_cores", "Expression": "{cpu_cores}", "IsIdentifier": false, "ImportField": true },
            { "TargetField": "ram_total", "Expression": "{ram_total}", "IsIdentifier": false, "ImportField": true },
            { "TargetField": "disk_total", "Expression": "{disk_total}", "IsIdentifier": false, "ImportField": true },
            { "TargetField": "disk_free", "Expression": "{disk_free}", "IsIdentifier": false, "ImportField": true },
            { "TargetField": "ip_address", "Expression": "{ip_address}", "IsIdentifier": true, "ImportField": true },
            { "TargetField": "mac_address", "Expression": "{mac_address}", "IsIdentifier": true, "ImportField": true }
        ],
        "Data": [
            {
                "transaction_id": "$TRANSACTION_ID",
                "os_name": "$OS_NAME",
                "kernel_version": "$KERNEL_VERSION",
                "cpu_model": "$CPU_MODEL",
                "cpu_cores": "$CPU_CORES",
                "ram_total": "$RAM_TOTAL",
                "disk_total": "$DISK_TOTAL",
                "disk_free": "$DISK_FREE",
                "ip_address": "$IP_ADDRESS",
                "mac_address": "$MAC_ADDRESS"
            }
        ]
    }
}
EOF

echo "✅ JSON-Datei gespeichert: $JSON_FILE"

# JSON an API senden
curl -X POST -H "Content-Type: application/json" --data "@$JSON_FILE" $API_ENDPOINT
if [ $? -eq 0 ]; then
    echo "✅ API-Antwort erfolgreich gesendet."
else
    echo "❌ Fehler beim Senden an API."
fi