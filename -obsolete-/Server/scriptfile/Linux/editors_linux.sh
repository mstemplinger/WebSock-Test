#!/bin/bash

# Liste möglicher Editoren
editors=("gedit" "nano" "vim" "vi")

# Überprüfen, welcher Editor verfügbar ist
for editor in "${editors[@]}"; do
    if command -v $editor &> /dev/null; then
        echo "Starte Texteditor: $editor"
        $editor &  # Editor im Hintergrund starten
        exit 0
    fi
done

echo "Kein unterstützter Texteditor gefunden! Installiere z.B. nano oder vim."
