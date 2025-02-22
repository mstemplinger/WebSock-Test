param (
    [string]$JsonFile,      # JSON-Datei als Parameter
    [string]$ApiEndpoint = "http://85.215.147.108:5001/inbox"  # Standard-API-Endpunkt
)

# ✅ Prüfen, ob die JSON-Datei existiert
if (-Not (Test-Path $JsonFile)) {
    Write-Host "❌ Fehler: Die angegebene JSON-Datei existiert nicht: $JsonFile"
    Exit 1
}

# ✅ Dateiinhalt lesen und validieren
try {
    $JsonContent = Get-Content -Path $JsonFile -Raw -Encoding UTF8  # Sicherstellen, dass UTF-8 verwendet wird
    $ParsedJson = $JsonContent | ConvertFrom-Json  # Prüft die JSON-Struktur
    Write-Host "🔍 JSON-Datei erfolgreich geladen und validiert."
} catch {
    Write-Host "❌ Fehler beim Lesen oder Validieren der JSON-Datei: $_"
    Exit 1
}

# ✅ Upload an die API durchführen
try {
    Write-Host "📤 Sende JSON-Datei an API: $ApiEndpoint"
    $Response = Invoke-RestMethod -Uri $ApiEndpoint -Method Post -ContentType "application/json" -InFile $JsonFile
    Write-Host "✅ API-Antwort erhalten: $Response"
} catch {
    Write-Host "❌ Fehler beim Hochladen der JSON-Datei: $_"
    Exit 1
}
