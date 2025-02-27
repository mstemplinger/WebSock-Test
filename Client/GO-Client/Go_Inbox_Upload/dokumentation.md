Um deine Go-Anwendung **`InboxUpload.go`** zu einer ausführbaren Datei (`.exe`) zu kompilieren und mit Parametern aufzurufen, folge diesen Schritten:

---

### **1️⃣ Erstellen der ausführbaren Datei (`.exe`)**
Öffne eine **Eingabeaufforderung (`cmd`)** oder **PowerShell** und navigiere zum Projektverzeichnis:

```sh
cd C:\Pfad\zu\deinem\Projekt
```

Dann erstelle die `.exe` mit:

```sh
go build -o InboxUpload.exe InboxUpload.go
```

---

### **2️⃣ Aufrufen der `.exe` mit Parametern**
Nachdem die `.exe` erstellt wurde, kannst du sie mit den gewünschten Parametern aufrufen:

```sh
InboxUpload.exe -json "C:\Pfad\zur\datei.json" -url "http://85.215.147.108:5001/inbox"
```

👉 **Erklärung der Parameter:**
- `-json "C:\Pfad\zur\datei.json"` → Pfad zur JSON-Datei, die hochgeladen werden soll.
- `-url "http://85.215.147.108:5001/inbox"` → URL der API (optional, falls eine andere URL als die Standard-URL verwendet werden soll).

---

### **3️⃣ Automatisierung mit einer Batch-Datei (`.bat`)**
Falls du den Aufruf automatisieren möchtest, kannst du eine **Batch-Datei** (`upload.bat`) erstellen:

📌 **Erstelle eine Datei `upload.bat` mit folgendem Inhalt:**
```bat
@echo off
InboxUpload.exe -json "C:\Pfad\zur\datei.json" -url "http://85.215.147.108:5001/inbox"
pause
```
Speichere die Datei und doppelklicke darauf, um das Programm zu starten.

---

### **4️⃣ Testen der Parameterübergabe**
Falls du überprüfen willst, ob die Parameter korrekt übergeben wurden, kannst du das Programm einfach mit:

```sh
InboxUpload.exe -h
```

Dadurch wird die Hilfe der Parameter angezeigt.

---

🚀 **Fertig!** Dein Go-Programm ist nun kompiliert und kann mit Parametern aufgerufen werden. Falls du Fragen hast, sag Bescheid! 😊