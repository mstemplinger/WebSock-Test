import json
import logging
import time
from sqlalchemy.exc import SQLAlchemyError
from flask import Flask
from flask_sqlalchemy import SQLAlchemy
from config import Config
from models import db, Inbox  # ✅ Importiere `db` und `Inbox`
from datetime import datetime, timezone

# 🛠 Erstelle eine neue Flask-App, damit `db` funktioniert
app = Flask(__name__)
app.config.from_object(Config)
db.init_app(app)

# Logging konfigurieren
logging.basicConfig(
    filename="inbox_handler.log",
    level=logging.DEBUG,
    format="%(asctime)s - %(levelname)s - %(message)s"
)

logging.info("🟢 Inbox-Handler wird gestartet...")

def process_inbox():
    """Verarbeitet alle noch nicht bearbeiteten Einträge in der Inbox-Tabelle."""
    logging.info("🔄 Starte `process_inbox`-Funktion...")

    with app.app_context():  # ✅ App Context für DB-Zugriff notwendig
        try:
            pending_entries = Inbox.query.filter_by(acx_inbox_processing_state="pending").all()
            if not pending_entries:
                logging.info("✅ Keine neuen Einträge zum Verarbeiten.")
                return

            for entry in pending_entries:
                logging.info(f"🔄 Verarbeite Inbox-ID: {entry.acx_inbox_id}")

                # Setze Status auf "running"
                entry.acx_inbox_processing_start = datetime.now(timezone.utc)
                entry.acx_inbox_processing_end = datetime.now(timezone.utc)
                db.session.commit()

                # JSON-Inhalt verarbeiten
                json_content = json.loads(entry.acx_inbox_content)
                table_name = json_content.get("Content", {}).get("TableName")
                data_entries = json_content.get("Content", {}).get("Data", [])

                if not table_name or not data_entries:
                    logging.warning(f"⚠️ Ungültige Daten in Inbox-ID: {entry.acx_inbox_id}")
                    entry.acx_inbox_processing_state = "error"
                    entry.acx_inbox_processing_log = "Ungültige Datenstruktur"
                    db.session.commit()
                    continue

                # Füge Daten in die Ziel-Tabelle ein
                for record in data_entries:
                    insert_query = f"INSERT INTO {table_name} ({', '.join(record.keys())}) VALUES ({', '.join(['?' for _ in record.keys()])})"
                    logging.info(f"📥 SQL Query: {insert_query} | Daten: {tuple(record.values())}")
                    db.session.execute(insert_query, tuple(record.values()))

                db.session.commit()

                # Erfolg speichern
                entry.acx_inbox_processing_state = "success"
                entry.acx_inbox_processing_end = datetime.utcnow()
                entry.acx_inbox_processing_log = "Verarbeitung erfolgreich"
                db.session.commit()

                logging.info(f"✅ Verarbeitung für Inbox-ID {entry.acx_inbox_id} abgeschlossen!")

        except Exception as e:
            logging.error(f"❌ Fehler in `process_inbox`: {str(e)}")

if __name__ == "__main__":
    with app.app_context():  # ✅ App Context für DB-Zugriff notwendig
        logging.info("🟢 Inbox-Handler wurde gestartet.")
        while True:
            try:
                process_inbox()
            except Exception as e:
                logging.error(f"❌ Fehler in der Hauptschleife von `inbox_handler`: {str(e)}")

            time.sleep(10)  # Alle 10 Sekunden prüfen
