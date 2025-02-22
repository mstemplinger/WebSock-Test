import asyncio
import json
import logging
import websockets
from flask import Flask, render_template, request, jsonify
from flask_socketio import SocketIO
import socket
import threading
import os


SCRIPT_DIR = os.path.join(os.getcwd(), "scriptfile")

app = Flask(__name__)
socketio = SocketIO(app, cors_allowed_origins="*")  # WebSocket für HTML-Refresh

clients = {}
previous_clients = {}  # ⏳ Speichert vorherige Clients, um Änderungen zu erkennen

SCRIPT_DIR = os.path.join(os.path.dirname(__file__), "scriptfile")  # 📂 Skriptverzeichnis

# Logging konfigurieren
logging.basicConfig(
    filename="server.log",
    level=logging.DEBUG,
    format="%(asctime)s - %(levelname)s - %(message)s"
)

console_handler = logging.StreamHandler()
console_handler.setLevel(logging.DEBUG)
console_formatter = logging.Formatter("%(asctime)s - %(levelname)s - %(message)s")
console_handler.setFormatter(console_formatter)
logging.getLogger().addHandler(console_handler)

def is_port_in_use(port):
    """Prüft, ob der angegebene Port bereits in Benutzung ist."""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        return s.connect_ex(("0.0.0.0", port)) == 0

def check_for_refresh():
    """Überprüft, ob sich die Client-Liste geändert hat, und sendet nur dann ein Refresh"""
    global previous_clients
    current_clients = {client_id: {"hostname": data["hostname"], "ip": data["ip"]} for client_id, data in clients.items()}

    if previous_clients != current_clients:  # 🔍 Vergleiche mit vorheriger Liste
        previous_clients = current_clients  # 🔄 Update der vorherigen Clients
        socketio.emit("refresh", to=None)
        logging.info("🔄 Client-Liste geändert, Refresh gesendet.")
        print("🔄 Client-Liste geändert, Refresh gesendet.")

async def handle_client(websocket):
    """Verarbeitet eingehende WebSocket-Verbindungen von Clients"""
    try:
        client_ip = websocket.remote_address[0] if websocket.remote_address else "Unbekannt"
        logging.info(f"🔌 Neuer Client verbunden von {client_ip}")
        print(f"🔌 Neuer Client verbunden von {client_ip}")

        async for message in websocket:
            logging.info(f"📩 Nachricht erhalten von {client_ip}: {message}")
            print(f"📩 Nachricht erhalten von {client_ip}: {message}")

            try:
                data = json.loads(message)
                if data.get("action") == "register":
                    client_id = data["client_id"]
                    clients[client_id] = {
                        "websocket": websocket,
                        "hostname": data["hostname"],
                        "ip": data["ip"]
                    }
                    logging.info(f"✅ Client registriert: {data['hostname']} ({data['ip']})")
                    print(f"✅ Client registriert: {data['hostname']} ({data['ip']})")

                    # Sende Bestätigung an den Client
                    response = json.dumps({"status": "registered"})
                    await websocket.send(response)
                    logging.info(f"📤 Registrierungsbestätigung an {data['hostname']} gesendet")
                    print(f"📤 Registrierungsbestätigung an {data['hostname']} gesendet")

                    check_for_refresh()  # 🔄 Überprüfe Client-Änderungen für Refresh

                else:
                    logging.warning(f"⚠️ Unbekannte Aktion erhalten von {client_ip}: {data}")
                    print(f"⚠️ Unbekannte Aktion erhalten von {client_ip}: {data}")

            except json.JSONDecodeError:
                logging.error(f"🚨 Ungültiges JSON von {client_ip}: {message}")
                print(f"❌ JSON-Fehler: {message}")

    except websockets.exceptions.ConnectionClosed as e:
        logging.info(f"❌ WebSocket-Verbindung mit {client_ip} geschlossen: {e}")
        print(f"❌ WebSocket-Verbindung mit {client_ip} geschlossen: {e}")

    finally:
        # Client aus `clients` entfernen, wenn die Verbindung tatsächlich geschlossen wurde
        disconnected_clients = [key for key, value in clients.items() if value["websocket"] == websocket]
        for key in disconnected_clients:
            del clients[key]
            logging.info(f"🚪 Client {key} wurde entfernt.")
            print(f"🚪 Client {key} wurde entfernt.")

        check_for_refresh()  # 🔄 Überprüfe Client-Änderungen für Refresh

@app.route("/")
def index():
    """Zeigt die HTML-Webseite mit den verbundenen Clients an"""
    logging.debug("📄 HTML-Seite mit aktuellen Clients geöffnet.")
    print(f"Aktuelle Clients: {clients}")
    return render_template("index.html", clients=clients)

@app.route("/clients", methods=["GET"])
def get_clients():
    """Gibt die Liste der verbundenen Clients als JSON zurück"""
    logging.debug("📄 Clients API aufgerufen.")

    serializable_clients = {
        client_id: {"hostname": data["hostname"], "ip": data["ip"]}
        for client_id, data in clients.items()
    }

    return jsonify(serializable_clients)

@app.route("/send_message", methods=["POST"])
def send_message():
    """Sendet eine Nachricht an einen bestimmten Client"""
    client_id = request.form.get("client_id")
    message = request.form.get("message")

    if not client_id or not message:
        return json.dumps({"status": "error", "message": "Client-ID oder Nachricht fehlt!"}), 400

    if client_id in clients:
        ws = clients[client_id]["websocket"]

        if ws.close_code is None:
            try:
                # Nachricht als JSON-Format senden
                message_data = {
                    "action": "message",
                    "content": message
                }

                asyncio.run(ws.send(json.dumps(message_data, ensure_ascii=False)))

                return json.dumps({"status": "success", "message": "Nachricht gesendet"}), 200
            except Exception as e:
                return json.dumps({"status": "error", "message": f"Fehler beim Senden: {str(e)}"}), 500
        else:
            del clients[client_id]
            return json.dumps({"status": "error", "message": "Client nicht mehr verbunden"}), 410

    return json.dumps({"status": "error", "message": "Client nicht gefunden"}), 404


@app.route("/send_message_all", methods=["POST"])
def send_message_all():
    """Sendet eine Nachricht an alle verbundenen Clients"""
    message = request.form.get("message")

    if not message:
        return "Fehler: Nachricht fehlt!", 400

    logging.info(f"📤 Nachricht an alle Clients senden: {message}")

    for client_id, client in clients.items():
        ws = client["websocket"]
        if ws.close_code is None:  # Verbindung ist noch offen
            try:
                asyncio.run(ws.send(message))
            except Exception as e:
                logging.error(f"❌ Fehler beim Senden an {client_id}: {str(e)}")

    return "Gesendet", 200

@app.route("/send_script", methods=["POST"])
def send_script():
    """Sendet ein Skript an einen bestimmten Client"""
    client_id = request.form.get("client_id")
    script_name = request.form.get("script_name")

    if not client_id or not script_name:
        return "Fehler: Client-ID oder Skriptnamen fehlt!", 400

    script_path = os.path.join(SCRIPT_DIR, script_name)

    if not os.path.exists(script_path):
        return f"Fehler: Skript {script_name} nicht gefunden!", 404

    if client_id in clients:
        ws = clients[client_id]["websocket"]

        if ws.close_code is None:
            try:
                with open(script_path, "r") as script_file:
                    script_content = script_file.read()

                script_message = json.dumps({
                    "action": "execute_script",
                    "script_name": script_name,
                    "script_content": script_content
                })

                asyncio.run(ws.send(script_message))

                return "Skript gesendet", 200
            except Exception as e:
                return f"Fehler beim Senden: {str(e)}", 500
        else:
            del clients[client_id]
            return "Client nicht mehr verbunden", 410

    return "Client nicht gefunden", 404

@app.route("/send_script_all", methods=["POST"])
def send_script_all():
    """Sendet ein Skript an alle Clients"""
    script_name = request.form.get("script_name")

    if not script_name:
        return "Fehler: Skriptnamen fehlt!", 400

    script_path = os.path.join(SCRIPT_DIR, script_name)

    if not os.path.exists(script_path):
        return f"Fehler: Skript {script_name} nicht gefunden!", 404

    logging.info(f"📤 Sende Skript {script_name} an alle Clients...")

    for client_id, client in clients.items():
        ws = client["websocket"]
        if ws.close_code is None:
            try:
                with open(script_path, "r") as script_file:
                    script_content = script_file.read()

                script_message = json.dumps({
                    "action": "execute_script",
                    "script_name": script_name,
                    "script_content": script_content
                })

                asyncio.run(ws.send(script_message))
            except Exception as e:
                logging.error(f"❌ Fehler beim Senden an {client_id}: {str(e)}")

    return "Skript an alle gesendet", 200

async def start_websocket_server():
    """Startet den WebSocket-Server"""
    logging.info("websock check")
    print("websock check")

    if is_port_in_use(8765):
        logging.error("❌ FEHLER: Port 8765 ist bereits belegt! WebSocket-Server kann nicht gestartet werden.")
        print("❌ FEHLER: Port 8765 ist bereits belegt! WebSocket-Server kann nicht gestartet werden.")
        return

    logging.info("✅ WebSocket-Server gestartet auf Port 8765.")
    print("✅ WebSocket-Server läuft auf Port 8765...")

    async with websockets.serve(handle_client, "0.0.0.0", 8765):
        print("🟢 WebSocket-Server wartet auf Clients...")
        logging.info("🟢 WebSocket-Server wartet auf Clients...")
        while True:
            await asyncio.sleep(1)

def run_flask():
    socketio.run(app, host="0.0.0.0", port=5001, debug=False, allow_unsafe_werkzeug=True)

def main():
    threading.Thread(target=run_flask, daemon=True).start()
    asyncio.run(start_websocket_server())

if __name__ == "__main__":
    main()
