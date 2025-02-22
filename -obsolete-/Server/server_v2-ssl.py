import ssl
import asyncio
import json
import logging
import websockets
import time
import uuid
import os
import base64
import socket
import threading
from flask import Flask, render_template, request, jsonify
from flask_socketio import SocketIO
from flask_sqlalchemy import SQLAlchemy
from config import Config
from models import db, Inbox, Asset, Inbox, ClientUser, SystemInfo, WSUSScanResult, WSUSDownloadInfo
from sqlalchemy.exc import SQLAlchemyError
from datetime import datetime, timezone
from sqlalchemy import text, inspect, literal
from sqlalchemy import String, NVARCHAR
from sqlalchemy.exc import SQLAlchemyError
from websockets.exceptions import ConnectionClosed



SCRIPT_DIR = os.path.join(os.getcwd(), "scriptfile")

app = Flask(__name__)
app.config.from_object(Config)  # Lade Einstellungen aus der config.py
db.init_app(app) 

socketio = SocketIO(app, cors_allowed_origins="*")  # WebSocket für HTML-Refresh

# 🔹 **Hier den Fingerabdruck deines Zertifikats einfügen!**
CERT_PEM = r"C:\ondesoDC-PY\_WebSock-Test\Server\certs\cert.pem"
CERT_KEY_PEM = r"C:\ondesoDC-PY\_WebSock-Test\Server\certs\key.pem"

clients = {}
previous_clients = {}  # ⏳ Speichert vorherige Clients, um Änderungen zu erkennen

SCRIPT_DIR = os.path.join(os.path.dirname(__file__), "scriptfile")  # 📂 Skriptverzeichnis

CHUNK_SIZE = 4000  # Maximale Größe pro Chunk

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

def get_ssl_certificate(cert_file: str, key_file: str = None):
    """
    Lädt das SSL-Zertifikat aus einer Datei und erstellt einen SSL-Context.
    :param cert_file: Pfad zur Zertifikatsdatei (PEM-Format).
    :param key_file: (Optional) Pfad zur Schlüsseldatei (PEM-Format), falls benötigt.
    :return: SSLContext-Objekt oder None bei Fehler.
    """
    try:
        if not os.path.exists(cert_file):
            print(f"❌ Zertifikatsdatei nicht gefunden: {cert_file}")
            return None
        
        if key_file and not os.path.exists(key_file):
            print(f"❌ Schlüsseldatei nicht gefunden: {key_file}")
            return None
        
        # SSL-Context erstellen
        ssl_context = ssl.create_default_context(ssl.Purpose.CLIENT_AUTH)
        ssl_context.load_cert_chain(certfile=cert_file, keyfile=key_file)
        
        print(f"✅ Zertifikat erfolgreich geladen: {cert_file}")
        return ssl_context
    
    except Exception as e:
        print(f"❌ Fehler beim Laden des Zertifikats: {e}")
        return None



ssl_context = get_ssl_certificate(CERT_PEM, CERT_KEY_PEM)

if ssl_context is None:
    print("❌ SSL-Zertifikat konnte nicht geladen werden!")

@app.route("/")
def index():
    """Zeigt die HTML-Webseite mit den verbundenen Clients an."""
    logging.debug("📄 HTML-Seite mit aktuellen Clients geöffnet.")
    print(f"Aktuelle Clients: {clients}")
    return render_template("index.html", clients=clients)


@app.route("/client/<client_id>")
def client_details(client_id):
    """Zeigt die Details eines bestimmten Clients an."""
    client = Asset.query.filter_by(client_id=client_id).first()  # 🔄 Asset statt Client

    if not client:
        logging.error(f"❌ Client nicht gefunden: {client_id}")
        return "Client nicht gefunden", 404

    tables = get_tables()
    return render_template("client_details.html", client=client, tables=tables, client_id=client_id)


@app.route("/table/<table_name>")
def table_data(table_name):
    """Zeigt alle Daten aus einer bestimmten Tabelle an."""
    
    # 🛠️ Prüfen, ob die Tabelle existiert
    db.Model.metadata.reflect(bind=db.engine)
    if table_name not in db.Model.metadata.tables:
        logging.error(f"❌ Fehler: Tabelle `{table_name}` existiert nicht in der Datenbank!")
        return f"Fehler: Tabelle `{table_name}` nicht gefunden!", 404

    logging.info(f"📊 Lade alle Daten aus `{table_name}`")

    try:
        table = db.Model.metadata.tables[table_name]
        query = table.select()  # 🔄 KEINE Filterung auf client_id oder asset_id

        with db.engine.connect() as connection:
            result = connection.execute(query).fetchall()

        # 📌 Korrektur: Spaltennamen aus dem Table-Objekt holen
        column_names = [column.name for column in table.columns]

        return render_template("table_data.html", table_name=table_name, data=result, columns=column_names)

    except Exception as e:
        logging.error(f"❌ Fehler beim Abrufen der Daten für `{table_name}`: {str(e)}")
        return f"Interner Serverfehler: {str(e)}", 500


def get_tables():
    """Gibt alle Tabellen in der Datenbank zurück."""
    try:
        logging.info("🔄 Lade Tabellen aus der Datenbank...")
        db.Model.metadata.reflect(bind=db.engine)
        tables = list(db.Model.metadata.tables.keys())
        logging.info(f"📋 Tabellen gefunden: {tables}")
        return tables
    except Exception as e:
        logging.error(f"❌ Fehler beim Abrufen der Tabellen: {str(e)}")
        return []



@app.route("/inbox", methods=["POST"])
def inbox():
    """Empfängt JSON-Daten und speichert sie ungeprüft in die Inbox-Tabelle zur späteren Verarbeitung."""
    try:
        data = request.get_json()
        if not data:
            return jsonify({"error": "❌ Leere Anfrage erhalten"}), 400

        # ✅ JSON als Zeichenkette speichern
        json_content = json.dumps(data, ensure_ascii=False)

        # 📥 Neuen Inbox-Eintrag erstellen
        new_entry = Inbox(
            acx_inbox_name=data.get("MetaData", {}).get("Name", "Unbekannt"),
            acx_inbox_description=data.get("MetaData", {}).get("Description", "Keine Beschreibung"),
            acx_inbox_creator=data.get("MetaData", {}).get("Creator", "Unbekannt"),
            acx_inbox_vendor=data.get("MetaData", {}).get("Vendor", "Unbekannt"),
            acx_inbox_content_type=data.get("MetaData", {}).get("ContentType", "unknown"),
            acx_inbox_content=json_content,  # Ungeprüftes JSON speichern
        )

        db.session.add(new_entry)
        db.session.commit()

        logging.info(f"✅ Neuer JSON-Eintrag gespeichert in Inbox-ID: {new_entry.acx_inbox_id}")
        return jsonify({"message": "✅ Daten erfolgreich gespeichert", "InboxID": str(new_entry.acx_inbox_id)}), 201

    except Exception as e:
        db.session.rollback()
        logging.error(f"❌ Fehler beim Speichern in Inbox: {str(e)}")
        return jsonify({"error": "Interner Serverfehler", "details": str(e)}), 500


def get_column_lengths(table_name):
    """Liest die maximale Feldlängen der Spalten einer Tabelle aus."""
    try:
        logging.info(f"🔍 Prüfe Spaltenlängen für Tabelle `{table_name}`")
        
        # Metadaten aktualisieren
        db.Model.metadata.reflect(bind=db.engine, raiseerr=True)
        
        # Tabelle abrufen
        table = db.Model.metadata.tables.get(table_name)

        if table is None:
            logging.error(f"❌ Tabelle `{table_name}` nicht gefunden!")
            raise ValueError(f"❌ Tabelle `{table_name}` nicht gefunden.")
        
        logging.info(f"✅ Tabelle `{table_name}` gefunden. Starte Spaltenprüfung...")

        column_lengths = {}

        for column in table.columns:
            logging.info(f"ℹ️ Prüfe Feld `{column.name}`...")
            
            if isinstance(column.type, (String, NVARCHAR)) and column.type.length:
                column_lengths[column.name] = column.type.length
                logging.info(f"✅ Feld `{column.name}` hat eine max. Länge von {column.type.length}")

        logging.debug(f"📊 Erkannte Feldlängen für `{table_name}`: {column_lengths}")
        return column_lengths

    except Exception as e:
        logging.error(f"🚨 Fehler in `get_column_lengths({table_name})`: {str(e)}")
        raise


def truncate_values(column_values, column_lengths):
    """Kürzt die Werte basierend auf den maximalen Feldlängen."""
    logging.info(f"🔍 truncate_values `{column_values}`")
    for column, max_length in column_lengths.items():
        if column in column_values and isinstance(column_values[column], str):
            original_value = column_values[column]
            truncated_value = original_value[:max_length]
            column_values[column] = truncated_value
            if original_value != truncated_value:
                logging.warning(f"⚠️ Wert für `{column}` wurde von {len(original_value)} auf {max_length} Zeichen gekürzt!")
    return column_values

def process_inbox():
    """Verarbeitet alle noch nicht bearbeiteten Einträge in der Inbox-Tabelle."""
    logging.info("🟢 Starte `process_inbox`-Thread...")
    
    while True:
        with app.app_context():
            try:
                db.Model.metadata.reflect(bind=db.engine)  # 🔄 Metadaten synchronisieren
                logging.info(f"📋 Tabellen: {db.Model.metadata.tables.keys()}")

                logging.info("🛠️ Filtere nach acx_inbox_processing_state='pending'")
                pending_entries = Inbox.query.filter(Inbox.acx_inbox_processing_state == literal("pending")).all()
                
                if not pending_entries:
                    logging.info("✅ Keine neuen Einträge zum Verarbeiten.")
                else:
                    for entry in pending_entries:
                        logging.info(f"🔄 Verarbeite Inbox-ID: {entry.acx_inbox_id}")
                        entry.acx_inbox_processing_state = "running"
                        entry.acx_inbox_processing_start = datetime.now(timezone.utc)
                        db.session.commit()

                        try:
                            logging.info(f"📥 Lade JSON-Inhalt für Inbox-ID: {entry.acx_inbox_id}")
                            json_content = json.loads(entry.acx_inbox_content)
                            logging.debug(f"📄 JSON-Inhalt: {json_content}")
                            
                            content_section = json_content.get("Content")
                            if not isinstance(content_section, dict):
                                raise ValueError("❌ `Content`-Bereich fehlt oder ist ungültig")
                            
                            table_name = content_section.get("TableName", "").strip()
                            data_entries = content_section.get("Data", [])
                            mappings = content_section.get("FieldMappings", [])
                            
                            if not table_name or not data_entries or not mappings:
                                raise ValueError("❌ Fehlende Daten oder Mappings im JSON")
                            
                            column_lengths = get_column_lengths(table_name)
                            
                            for record in data_entries:
                                column_values = {}
                                logging.info(f"📑 Verarbeite Datensatz: {record}")
                                
                                for mapping in mappings:
                                    db_field = mapping.get("TargetField", "").strip()
                                    expression = mapping.get("Expression", "").strip()

                                    if not db_field:
                                        raise ValueError("❌ `TargetField` fehlt in Mappings")
                                    if not expression:
                                        raise ValueError(f"❌ `Expression` fehlt für {db_field}")

                                    if expression == "NewGUID()":
                                        column_values[db_field] = str(uuid.uuid4())
                                    elif expression.startswith("{") and expression.endswith("}"):
                                        json_field = expression.strip("{}")
                                        column_values[db_field] = record.get(json_field, "")
                                    else:
                                        column_values[db_field] = expression

                                    logging.debug(f"🔄 Mapping `{db_field}`: `{expression}` -> `{column_values[db_field]}`")

                                column_values = truncate_values(column_values, column_lengths)
                                
                                insert_query = text(f"""
                                    INSERT INTO {table_name} ({', '.join(column_values.keys())}) 
                                    VALUES ({', '.join([f':{key}' for key in column_values.keys()])})
                                """)
                                
                                logging.info(f"📥 SQL Insert Query: {insert_query}")
                                logging.debug(f"📊 Insert-Daten: {column_values}")

                                db.session.execute(insert_query, column_values)
                            
                            db.session.commit()
                            entry.acx_inbox_processing_state = "success"
                            entry.acx_inbox_processing_end = datetime.now(timezone.utc)
                            entry.acx_inbox_processing_log = "Verarbeitung erfolgreich"
                            db.session.commit()
                            logging.info(f"✅ Verarbeitung für Inbox-ID {entry.acx_inbox_id} abgeschlossen!")
                        
                        except SQLAlchemyError as e:
                            logging.error(f"❌ SQL-Fehler bei Inbox-ID {entry.acx_inbox_id}: {str(e)}")
                            db.session.rollback()
                            entry.acx_inbox_processing_state = "error"
                            entry.acx_inbox_processing_log = str(e)
                            db.session.commit()
                        
                        except Exception as e:
                            logging.error(f"❌ Fehler bei Inbox-ID {entry.acx_inbox_id}: {str(e)}")
                            entry.acx_inbox_processing_state = "error"
                            entry.acx_inbox_processing_log = str(e)
                            db.session.commit()
            
            except Exception as e:
                logging.error(f"❌ Fehler in `process_inbox`: {str(e)}")
        
        time.sleep(10)  # Alle 10 Sekunden prüfen



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
    """Verarbeitet eingehende WebSocket-Verbindungen von Clients mit erweitertem Logging"""
    try:
        client_ip = websocket.remote_address[0] if websocket.remote_address else "Unbekannt"
        logging.info(f"🔌 Neuer Client verbunden von {client_ip}")

        async for message in websocket:
            try:
                logging.info(f"📩 Eingehende Nachricht von {client_ip}: {message}")

                data = json.loads(message)
                action = data.get("action")

                if action == "register":
                    client_id = data.get("client_id")
                    hostname = data.get("hostname")
                    ip_address = data.get("ip")

                    # **Datenvalidierung**
                    if not client_id or not hostname or not ip_address:
                        logging.warning(f"⚠️ Ungültige Registrierungsdaten von {client_ip}: {data}")
                        await websocket.send(json.dumps({"status": "error", "message": "Invalid registration data"}))
                        continue

                    # **Client im lokalen Dictionary speichern**
                    clients[client_id] = {"websocket": websocket, "hostname": hostname, "ip": ip_address}
                    logging.info(f"📥 Neuer Client zwischengespeichert: {clients[client_id]}")

                    # **✅ Datenbank-Operationen für `acx_asset`**
                    with app.app_context():
                        try:
                            logging.info(f"🔎 Suche nach bestehendem Asset für Client {client_id}...")
                            existing_asset = Asset.query.filter_by(client_id=client_id).first()

                            if existing_asset:
                                logging.info(f"🔄 Update bestehendes Asset: {existing_asset.client_id} (Last Seen: {existing_asset.last_seen})")
                                existing_asset.last_seen = datetime.now()
                            else:
                                logging.info(f"🆕 Neues Asset wird erstellt für Client {client_id} ({hostname}, {ip_address})")
                                new_asset = Asset(client_id=client_id, hostname=hostname, ip_address=ip_address)
                                db.session.add(new_asset)

                            db.session.commit()
                            check_for_refresh()  # 🔄 Überprüfe Client-Änderungen für Refresh
                            logging.info(f"✅ Client {client_id} erfolgreich registriert oder aktualisiert in `acx_asset`")

                        except SQLAlchemyError as e:
                            db.session.rollback()
                            logging.error(f"❌ Datenbankfehler bei Registrierung von {client_id}: {str(e)}")
                            await websocket.send(json.dumps({"status": "error", "message": f"Database error: {str(e)}"}))
                            continue
                        except Exception as e:
                            db.session.rollback()
                            logging.error(f"❌ Unerwarteter Fehler bei DB-Operation für {client_id}: {str(e)}")
                            await websocket.send(json.dumps({"status": "error", "message": f"Unexpected error: {str(e)}"}))
                            continue

                    # **✅ Bestätigung an den Client senden**
                    response = json.dumps({"status": "registered"})
                    await websocket.send(response)
                    check_for_refresh()  # 🔄 Überprüfe Client-Änderungen für Refresh
                    logging.info(f"📤 Registrierungsbestätigung an {hostname} ({ip_address}) gesendet")

                else:
                    logging.warning(f"⚠️ Unbekannte Aktion von {client_ip}: {data}")
                    check_for_refresh()  # 🔄 Überprüfe Client-Änderungen für Refresh
                    await websocket.send(json.dumps({"status": "error", "message": "Unknown action"}))

            except json.JSONDecodeError:
                logging.error(f"🚨 Ungültiges JSON von {client_ip}: {message}")
                await websocket.send(json.dumps({"status": "error", "message": "Invalid JSON"}))
            except Exception as e:
                logging.error(f"❌ Allgemeiner Fehler in der Nachricht von {client_ip}: {str(e)}")
                await websocket.send(json.dumps({"status": "error", "message": f"Processing error: {str(e)}"}))

    except websockets.exceptions.ConnectionClosed as e:
        logging.info(f"❌ WebSocket-Verbindung mit {client_ip} geschlossen: {e}")

    finally:
        # **Client aus der DB entfernen, wenn Verbindung verloren geht**
        with app.app_context():
            disconnected_clients = [key for key, value in clients.items() if value["websocket"] == websocket]

            for client_id in disconnected_clients:
                try:
                    logging.info(f"🚪 Entferne Client {client_id} aus Clientspeicher...")
                    clients.pop(client_id, None)

                    logging.info(f"🔎 Suche nach Asset {client_id} in `acx_asset` zur Löschung...")
                    asset = Asset.query.filter_by(client_id=client_id).first()
                    
                    if asset:
                        logging.info(f"🗑️ Lösche Asset {client_id} aus `acx_asset`")
                        # db.session.delete(asset)
                        # db.session.commit()
                        logging.info(f"✅ Client {client_id} erfolgreich aus `acx_asset` entfernt.")
                    else:
                        logging.warning(f"⚠️ Kein Asset-Eintrag für {client_id} gefunden. Kein Löschvorgang durchgeführt.")

                except SQLAlchemyError as e:
                    db.session.rollback()
                    logging.error(f"❌ Fehler beim Löschen von Client {client_id} aus `acx_asset`: {str(e)}")
                except Exception as e:
                    logging.error(f"❌ Allgemeiner Fehler beim Entfernen von Client {client_id}: {str(e)}")

        check_for_refresh()  # 🔄 Überprüfe Client-Änderungen für Refresh
        logging.info("🛑 Client-Entfernung abgeschlossen.")





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
    """Sendet ein Skript in mehreren Chunks an einen Client"""
    client_id = request.form.get("client_id")
    script_name = request.form.get("script_name")
    script_type = request.form.get("script_type")  # 🆕 Skripttyp erfassen

    if not client_id or not script_name or not script_type:
        return "Fehler: Client-ID, Skriptnamen oder Skripttyp fehlt!", 400

    script_path = os.path.join(SCRIPT_DIR, script_name)

    if not os.path.exists(script_path):
        return f"Fehler: Skript {script_name} nicht gefunden!", 404

    if client_id in clients:
        ws = clients[client_id]["websocket"]

        if ws.close_code is None:
            try:
                with open(script_path, "rb") as script_file:
                    script_content = script_file.read()
                    script_content_base64 = base64.b64encode(script_content).decode("utf-8")  # 🆕 Base64-Kodierung

                # Skript in Chunks aufteilen
                total_chunks = (len(script_content_base64) // CHUNK_SIZE) + 1

                for chunk_index in range(total_chunks):
                    start = chunk_index * CHUNK_SIZE
                    end = start + CHUNK_SIZE
                    script_chunk = script_content_base64[start:end]

                    chunk_message = json.dumps({
                        "action": "upload_script_chunk",
                        "script_name": script_name,
                        "chunk_index": chunk_index,
                        "total_chunks": total_chunks,
                        "script_chunk": script_chunk,
                        "script_type": script_type
                    }, ensure_ascii=False)

                    asyncio.run(ws.send(chunk_message))
                    print(f"📤 Gesendet: Chunk {chunk_index+1}/{total_chunks} ({len(script_chunk)} Bytes)")

                return "Skript in Chunks gesendet", 200
            except Exception as e:
                return f"Fehler beim Senden: {str(e)}", 500
        else:
            del clients[client_id]
            return "Client nicht mehr verbunden", 410

    return "Client nicht gefunden", 404


@app.route("/send_script_all", methods=["POST"])
def send_script_all():
    """Sendet ein Skript an alle Clients mit Skripttyp"""
    script_name = request.form.get("script_name")
    script_type = request.form.get("script_type")  # Skripttyp hinzufügen

    if not script_name or not script_type:
        return "Fehler: Skriptnamen oder Skripttyp fehlt!", 400

    script_path = os.path.join(SCRIPT_DIR, script_name)

    if not os.path.exists(script_path):
        return f"Fehler: Skript {script_name} nicht gefunden!", 404

    logging.info(f"📤 Sende Skript {script_name} ({script_type}) an alle Clients...")

    try:
        with open(script_path, "r", encoding="utf-8") as script_file:
            script_content = script_file.read()

        # Base64-Kodierung des Skriptinhalts
        script_content_base64 = base64.b64encode(script_content).decode("utf-8")  # 🆕 Base64-Kodierung

        script_message = json.dumps({
            "action": "execute_script",
            "script_name": script_name,
            "script_content": script_content_base64,
            "script_type": script_type  # Skripttyp mit einfügen
        })

        for client_id, client in clients.items():
            ws = client["websocket"]
            if ws.close_code is None:
                try:
                    asyncio.run(ws.send(script_message))
                    logging.info(f"✅ Skript an {client_id} gesendet.")
                except Exception as e:
                    logging.error(f"❌ Fehler beim Senden an {client_id}: {str(e)}")

    except Exception as e:
        logging.error(f"❌ Fehler beim Lesen des Skripts: {str(e)}")
        return f"Fehler beim Lesen des Skripts: {str(e)}", 500

    return "Skript an alle gesendet", 200

@app.route("/get_scripts", methods=["GET"])
def get_scripts():
    """Liefert eine Liste der verfügbaren Skripte im Skriptverzeichnis (inkl. Unterverzeichnisse 1. Ebene)"""
    allowed_extensions = {".ps1", ".bat", ".py", ".sh", ".txt"}
    ignored_dirs = {"_obsolete_"}

    if not os.path.exists(SCRIPT_DIR):
        return jsonify({"error": "Skriptverzeichnis nicht gefunden!"}), 500

    try:
        scripts = []

        # 🔍 Durchlaufe das Hauptverzeichnis
        for root, dirs, files in os.walk(SCRIPT_DIR):
            # Nur die erste Ebene der Unterverzeichnisse betrachten
            rel_root = os.path.relpath(root, SCRIPT_DIR)  # Relativer Pfad zum Root
            if rel_root in ignored_dirs:
                continue  # 🚫 Verzeichnis _obsolete_ überspringen

            # 🔄 Unterverzeichnisse nach erster Ebene abschneiden
            if os.path.dirname(rel_root):
                continue  # Verzeichnisse aus tieferen Ebenen ignorieren

            for file in files:
                ext = os.path.splitext(file)[1]
                if ext in allowed_extensions:
                    script_path = os.path.join(rel_root, file) if rel_root != "." else file  # 🎯 Pfad zusammensetzen
                    scripts.append({
                        "name": script_path.replace("\\", "/"),  # Konsistente Pfade für Windows/Linux
                        "type": "powershell" if ext == ".ps1" else
                                "bat" if ext == ".bat" else
                                "python" if ext == ".py" else
                                "linuxshell" if ext == ".sh" else
                                "text"
                    })

        return jsonify(scripts)

    except Exception as e:
        return jsonify({"error": f"Fehler beim Abrufen der Skripte: {str(e)}"}), 500


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
    
    if ssl_context:
        print("🔒 Starte sicheren WebSocket-Server (WSS)...")
        async with websockets.serve(handle_client, "0.0.0.0", 8765, ssl=ssl_context):
            print("🟢 WebSocket-Server wartet auf Clients...")
            logging.info("🟢 WebSocket-Server wartet auf Clients...")
            while True:
                await asyncio.sleep(1)

# 📌 **Server-Start**
def run_flask():
    if ssl_context:
        print("✅ Starte Flask mit HTTPS...")
        socketio.run(app, host="0.0.0.0", port=5001, debug=False, ssl_context=ssl_context)
    else:
        print("❌ Kein gültiges SSL-Zertifikat gefunden!")


def main():
    threading.Thread(target=run_flask, daemon=True).start()
    threading.Thread(target=process_inbox, daemon=True).start()  # 🟢 Starte den Inbox-Handler im Hintergrund
    asyncio.run(start_websocket_server())

if __name__ == "__main__":
    with app.app_context():
        db.create_all()

    main()