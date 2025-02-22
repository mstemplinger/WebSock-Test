from flask_sqlalchemy import SQLAlchemy
from sqlalchemy import Column, String, Integer, DateTime, ForeignKey, Text, UniqueConstraint
from sqlalchemy.dialects.mssql import UNIQUEIDENTIFIER, NVARCHAR, BIGINT
from sqlalchemy.sql import func

db = SQLAlchemy()

# 📌 **Neue Tabelle für angemeldete Clients (Assets)**
class Asset(db.Model):
    __tablename__ = "acx_asset"
    id = Column(Integer, primary_key=True, autoincrement=True)  # 🔥 Auto-Increment ID
    asset_id = Column(UNIQUEIDENTIFIER, unique=True, nullable=False, default=func.newid())  # 🔗 UUID als UNIQUE
    client_id = Column(UNIQUEIDENTIFIER, unique=True, nullable=False)  # UUID des Clients
    hostname = Column(NVARCHAR(128), nullable=False)
    ip_address = Column(NVARCHAR(50), nullable=False)
    last_seen = Column(DateTime, nullable=False, default=func.now())  # Letzte Anmeldung

    __table_args__ = (UniqueConstraint("client_id", name="uq_client_id"),)

# 📌 **Inbox für JSON-Daten**
class Inbox(db.Model):
    __tablename__ = "acx_inbox"
    id = Column(Integer, primary_key=True, autoincrement=True)
    acx_inbox_id = Column(UNIQUEIDENTIFIER, unique=True, nullable=False, default=func.newid())  # 🔗 UUID als UNIQUE
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

    id = Column(Integer, primary_key=True, autoincrement=True)  # 🔥 Auto-Increment ID
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
    description = Column(NVARCHAR(1512), nullable=True)
    uid = Column(Integer, nullable=True)
    gid = Column(Integer, nullable=True)
    home_directory = Column(NVARCHAR(512), nullable=True)
    shell = Column(NVARCHAR(256), nullable=True)

# 📌 **System-Informationen mit Verweis auf `acx_asset`**
class SystemInfo(db.Model):
    __tablename__ = "usr_system_info"

    id = Column(Integer, primary_key=True, autoincrement=True)
    asset_id = Column(UNIQUEIDENTIFIER, ForeignKey("acx_asset.client_id"), nullable=False)  # 🔗 Verknüpfung mit Asset
    transaction_id = Column(UNIQUEIDENTIFIER, unique=True, nullable=False, default=func.newid())
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

    id = Column(Integer, primary_key=True, autoincrement=True)  # 🔥 Auto-Increment ID
    scan_id = Column(UNIQUEIDENTIFIER, primary_key=True, nullable=False, default=func.newid())  # 🔗 UUID als UNIQUE
    asset_id = Column(UNIQUEIDENTIFIER, ForeignKey("acx_asset.client_id"), nullable=False)  # 🔗 Verknüpfung mit Asset
    scan_date = Column(DateTime, nullable=False, default=func.now())
    update_id = Column(String(128), nullable=False)  # ❌ `unique=True` entfernt!
    title = Column(String(512), nullable=False)
    description = Column(String(4096), nullable=True)
    kb_article_ids = Column(String(256), nullable=True)
    support_url = Column(String(512), nullable=True)
    is_downloaded = Column(Integer, nullable=True, default=False)
    is_mandatory = Column(Integer, nullable=True, default=False)

# 📌 **WSUS-Downloads mit Verweis auf `acx_asset`**
class WSUSDownloadInfo(db.Model):
    __tablename__ = "usr_wsus_downloads"

    id = Column(Integer, primary_key=True, autoincrement=True)  # 🔥 Auto-Increment ID
    download_id = Column(UNIQUEIDENTIFIER, unique=True, nullable=False, default=func.newid())  # 🔗 UUID als UNIQUE
    asset_id = Column(UNIQUEIDENTIFIER, ForeignKey("acx_asset.client_id"), nullable=False)  # 🔗 Verknüpfung mit Asset
    scan_id = Column(UNIQUEIDENTIFIER, nullable=False)  # 🔗 Verknüpfung mit Scan
    update_id = Column(String(128), nullable=False)
    file_url = Column(String(1024), nullable=False)
    file_name = Column(String(512), nullable=True)
    file_size = Column(Integer, nullable=True)
    is_secure = Column(Integer, nullable=False, default=1)  # 1 = HTTPS, 0 = HTTP

# 📌 **Sicherheitsinventar mit Verweis auf `acx_asset`**
class SecurityInventory(db.Model):
    __tablename__ = "usr_security_inventory"

    id = Column(Integer, primary_key=True, autoincrement=True)
    asset_id = Column(UNIQUEIDENTIFIER, ForeignKey("acx_asset.client_id"), nullable=False)  # 🔗 Verknüpfung mit Asset
    transaction_id = Column(UNIQUEIDENTIFIER, unique=True, nullable=False, default=func.newid())
    scan_date = Column(DateTime, nullable=False, default=func.now())
    os_name = Column(NVARCHAR(255), nullable=False)
    os_version = Column(NVARCHAR(255), nullable=True)
    os_last_boot = Column(DateTime, nullable=True)
    firewall_status = Column(NVARCHAR(255), nullable=True)
    antivirus_installed = Column(Text, nullable=True)
    windows_defender = Column(NVARCHAR(255), nullable=True)
    bitlocker_status = Column(NVARCHAR(255), nullable=True)
    uac_status = Column(Integer, nullable=True)
    local_admins = Column(Text, nullable=True)
    remote_desktop = Column(NVARCHAR(255), nullable=True)
    smb_status = Column(NVARCHAR(255), nullable=True)
    guest_account = Column(NVARCHAR(255), nullable=True)
    user_accounts = Column(Text, nullable=True)
    open_ports = Column(Text, nullable=True)
    logon_events = Column(Text, nullable=True)
    failed_logins = Column(Text, nullable=True)
    last_patch_date = Column(DateTime, nullable=True)