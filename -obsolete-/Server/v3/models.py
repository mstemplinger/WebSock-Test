from flask_sqlalchemy import SQLAlchemy
from sqlalchemy import Column, String, Integer, DateTime, ForeignKey, Text, UniqueConstraint
from sqlalchemy.dialects.mssql import UNIQUEIDENTIFIER, NVARCHAR, BIGINT
from sqlalchemy.sql import func

db = SQLAlchemy()

# 📌 **Neue Tabelle für angemeldete Clients (Assets)**
class Asset(db.Model):
    __tablename__ = "acx_asset"

    asset_id = Column(UNIQUEIDENTIFIER, primary_key=True, nullable=False, default=func.newid())
    client_id = Column(UNIQUEIDENTIFIER, unique=True, nullable=False)  # UUID des Clients
    hostname = Column(NVARCHAR(128), nullable=False)
    ip_address = Column(NVARCHAR(50), nullable=False)
    last_seen = Column(DateTime, nullable=False, default=func.now())  # Letzte Anmeldung

    __table_args__ = (UniqueConstraint("client_id", name="uq_client_id"),)

    def __init__(self, client_id, hostname, ip_address):
        self.client_id = client_id
        self.hostname = hostname
        self.ip_address = ip_address
        self.last_seen = func.now()

# 📌 **Inbox für JSON-Daten**
class Inbox(db.Model):
    __tablename__ = "acx_inbox"

    acx_inbox_id = Column(UNIQUEIDENTIFIER, primary_key=True, nullable=False, default=func.newid())
    acx_inbox_name = Column(NVARCHAR(128), nullable=True)
    acx_inbox_description = Column(NVARCHAR(256), nullable=True)
    acx_inbox_creator = Column(NVARCHAR(128), nullable=True)
    acx_inbox_vendor = Column(NVARCHAR(128), nullable=True)
    acx_inbox_content_type = Column(NVARCHAR(32), nullable=False, default='unknown')
    acx_inbox_content = Column(Text, nullable=True)
    acx_inbox_processing_state = Column(NVARCHAR(32), nullable=False, default='pending')

# 📌 **Client User Tabelle mit Verweis auf `acx_asset`**
class ClientUser(db.Model):
    __tablename__ = "usr_client_users"

    id = Column(UNIQUEIDENTIFIER, primary_key=True, nullable=False, default=func.newid())
    asset_id = Column(UNIQUEIDENTIFIER, ForeignKey("acx_asset.client_id"), nullable=False)  # 🔗 Verknüpfung mit Asset
    transaction_id = Column(UNIQUEIDENTIFIER, nullable=False, default=func.newid())
    username = Column(NVARCHAR(128), nullable=False)
    client = Column(NVARCHAR(128), nullable=False)
    usercount = Column(String(10), nullable=True)
    permissions = Column(NVARCHAR(512), nullable=True)
    sid = Column(NVARCHAR(128), nullable=True)
    full_name = Column(NVARCHAR(256), nullable=True)
    account_status = Column(NVARCHAR(32), nullable=True)
    last_logon = Column(DateTime, nullable=True)
    description = Column(NVARCHAR(512), nullable=True)
    uid = Column(Integer, nullable=True)
    gid = Column(Integer, nullable=True)
    home_directory = Column(NVARCHAR(512), nullable=True)
    shell = Column(NVARCHAR(256), nullable=True)

# 📌 **System-Informationen mit Verweis auf `acx_asset`**
class SystemInfo(db.Model):
    __tablename__ = "usr_system_info"

    id = Column(Integer, primary_key=True, autoincrement=True)
    asset_id = Column(UNIQUEIDENTIFIER, ForeignKey("acx_asset.client_id"), nullable=False)  # 🔗 Verknüpfung mit Asset
    transaction_id = Column(UNIQUEIDENTIFIER, default=func.newid(), nullable=False, unique=True)
    os_name = Column(NVARCHAR(255), nullable=False)
    os_version = Column(NVARCHAR(255), nullable=True)
    kernel_version = Column(NVARCHAR(255), nullable=True)
    cpu_model = Column(NVARCHAR(255), nullable=False)
    cpu_cores = Column(Integer, nullable=False)
    ram_total = Column(NVARCHAR(500), nullable=False)
    disk_total = Column(NVARCHAR(500), nullable=False)
    disk_free = Column(NVARCHAR(500), nullable=False)
    ip_address = Column(NVARCHAR(500), nullable=False)
    mac_address = Column(NVARCHAR(500), nullable=False)
    created_at = Column(DateTime, server_default=func.now())

# 📌 **WSUS-Scan Ergebnisse mit Verweis auf `acx_asset`**
class WSUSScanResult(db.Model):
    __tablename__ = "usr_wsus_scan_results"

    scan_id = Column(UNIQUEIDENTIFIER, primary_key=True, nullable=False, default=func.newid())
    asset_id = Column(UNIQUEIDENTIFIER, ForeignKey("acx_asset.client_id"), nullable=False)  # 🔗 Verknüpfung mit Asset
    scan_date = Column(DateTime, nullable=False, default=func.now())
    update_id = Column(String(128), nullable=False, unique=True)
    title = Column(String(512), nullable=False)
    description = Column(String(4096), nullable=True)
    kb_article_ids = Column(String(256), nullable=True)
    support_url = Column(String(512), nullable=True)
    is_downloaded = Column(Integer, nullable=True, default=False)
    is_mandatory = Column(Integer, nullable=True, default=False)

# 📌 **WSUS-Downloads mit Verweis auf `acx_asset`**
class WSUSDownloadInfo(db.Model):
    __tablename__ = "usr_wsus_downloads"

    download_id = Column(UNIQUEIDENTIFIER, primary_key=True, nullable=False, default=func.newid())
    asset_id = Column(UNIQUEIDENTIFIER, ForeignKey("acx_asset.client_id"), nullable=False)  # 🔗 Verknüpfung mit Asset
    scan_id = Column(UNIQUEIDENTIFIER, ForeignKey("usr_wsus_scan_results.scan_id"), nullable=False)
    update_id = Column(String(128), nullable=False)
    file_url = Column(String(1024), nullable=False)
    file_name = Column(String(512), nullable=True)
    file_size = Column(Integer, nullable=True)
    is_secure = Column(Integer, nullable=False, default=1)  # 1 = HTTPS, 0 = HTTP
