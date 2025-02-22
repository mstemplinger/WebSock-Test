from flask_sqlalchemy import SQLAlchemy
from sqlalchemy import Column, String, Integer, DateTime, UniqueConstraint, ForeignKey, Text
from sqlalchemy.dialects.mssql import UNIQUEIDENTIFIER, NVARCHAR, BIGINT
from sqlalchemy.sql import func

db = SQLAlchemy()

class Inbox(db.Model):
    __tablename__ = "acx_inbox"

    acx_inbox_id = Column(UNIQUEIDENTIFIER, primary_key=True, nullable=False, default=func.newid())
    acx_inbox_name = Column(NVARCHAR(128), nullable=True)
    acx_inbox_description = Column(NVARCHAR(256), nullable=True)
    acx_inbox_creator = Column(NVARCHAR(128), nullable=True)
    acx_inbox_vendor = Column(NVARCHAR(128), nullable=True)
    acx_inbox_content_type = Column(NVARCHAR(32), nullable=False, default='unknown')
    acx_inbox_content = Column(Text, nullable=True)  # Text statt `NVARCHAR(None)` für lange Inhalte
    acx_inbox_processing_state = Column(NVARCHAR(32), nullable=False, default='pending')

    def __init__(self, acx_inbox_name, acx_inbox_description, acx_inbox_creator, 
                 acx_inbox_vendor, acx_inbox_content_type, acx_inbox_content):
        self.acx_inbox_name = acx_inbox_name
        self.acx_inbox_description = acx_inbox_description
        self.acx_inbox_creator = acx_inbox_creator
        self.acx_inbox_vendor = acx_inbox_vendor
        self.acx_inbox_content_type = acx_inbox_content_type
        self.acx_inbox_content = acx_inbox_content
        self.acx_inbox_processing_state = 'pending'

class ClientUser(db.Model):
    __tablename__ = "usr_client_users"

    id = Column(UNIQUEIDENTIFIER, primary_key=True, nullable=False, default=func.newid())
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

class InboxProcessing(db.Model):
    __tablename__ = "acx_inbox_processing"

    acx_inbox_processing_id = Column(UNIQUEIDENTIFIER, primary_key=True, nullable=False, default=func.newid())
    acx_inbox_id = Column(UNIQUEIDENTIFIER, ForeignKey("acx_inbox.acx_inbox_id"), nullable=False)
    processing_state = Column(NVARCHAR(32), nullable=False, default="pending")
    processing_start = Column(DateTime, nullable=True)
    processing_end = Column(DateTime, nullable=True)
    processing_log = Column(Text, nullable=True)

class SystemInfo(db.Model):
    __tablename__ = "usr_system_info"

    id = Column(Integer, primary_key=True, autoincrement=True)
    transaction_id = Column(UNIQUEIDENTIFIER, default=func.newid(), nullable=False, unique=True)
    os_name = Column(NVARCHAR(255), nullable=False)
    os_version = Column(NVARCHAR(255), nullable=True)
    kernel_version = Column(NVARCHAR(255), nullable=True)
    cpu_model = Column(NVARCHAR(255), nullable=False)
    cpu_cores = Column(Integer, nullable=False)
    ram_total = Column(NVARCHAR(50), nullable=False)
    disk_total = Column(NVARCHAR(50), nullable=False)
    disk_free = Column(NVARCHAR(50), nullable=False)
    ip_address = Column(NVARCHAR(50), nullable=False)
    mac_address = Column(NVARCHAR(50), nullable=False)
    created_at = Column(DateTime, server_default=func.now())  # Korrektur von `func.getdate()`

    __table_args__ = (UniqueConstraint("transaction_id", name="uq_transaction_id"),)

class WSUSScanResult(db.Model):
    __tablename__ = "usr_wsus_scan_results"

    scan_id = Column(UNIQUEIDENTIFIER, primary_key=True, nullable=False, default=func.newid())
    scan_date = Column(DateTime, nullable=False, default=func.now())
    update_id = Column(String(128), nullable=False, unique=True)
    title = Column(String(512), nullable=False)
    description = Column(String(4096), nullable=True)
    kb_article_ids = Column(String(256), nullable=True)
    support_url = Column(String(512), nullable=True)
    is_downloaded = Column(Integer, nullable=True, default=False)
    is_mandatory = Column(Integer, nullable=True, default=False)

    __table_args__ = (UniqueConstraint("update_id", name="uq_update_id"),)

    def __init__(self, update_id, title, description, kb_article_ids, support_url, is_downloaded, is_mandatory):
        self.update_id = update_id
        self.title = title
        self.description = description
        self.kb_article_ids = kb_article_ids
        self.support_url = support_url
        self.is_downloaded = is_downloaded
        self.is_mandatory = is_mandatory

class WSUSDownloadInfo(db.Model):
    __tablename__ = "usr_wsus_downloads"

    download_id = Column(UNIQUEIDENTIFIER, primary_key=True, nullable=False, default=func.newid())
    scan_id = Column(UNIQUEIDENTIFIER, ForeignKey("usr_wsus_scan_results.scan_id"), nullable=False)
    update_id = Column(String(128), nullable=False)
    file_url = Column(String(1024), nullable=False)
    file_name = Column(String(512), nullable=True)
    file_size = Column(Integer, nullable=True)
    is_secure = Column(Integer, nullable=False, default=1)  # 1 = HTTPS, 0 = HTTP
