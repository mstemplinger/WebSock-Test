#!/bin/bash

# Countdown starten
echo "🔄 Das System wird in 3 Sekunden neu gestartet..."
for i in {3..1}; do
    echo "$i..."
    sleep 1
done

# Neustart ausführen
echo "🔄 Neustart jetzt!"
sudo reboot
