import os

class Config:
    SECRET_KEY = 'your_secret_key'
    SQLALCHEMY_DATABASE_URI = 'mssql+pyodbc://sa:qOndeso2012.!%@cl60002s\\sqlexpress/ondeso_websock?driver=ODBC+Driver+18+for+SQL+Server&trusted_connection=yes&encrypt=no'
    SQLALCHEMY_TRACK_MODIFICATIONS = False
    MAIL_SERVER = 'smtp.ionos.de'
    MAIL_PORT = 587
    MAIL_USERNAME = 'grafana@martinstemplinger.de'
    MAIL_DEFAULT_SENDER = 'grafana@martinstemplinger.de'
    MAIL_PASSWORD = '&yfU%goB3tuENLWMrJ4'
    MAIL_DEBUG = True
    MAIL_USE_TLS = True
    UPLOAD_FOLDER = 'uploads'
    STATIC_FOLDER = 'static'
