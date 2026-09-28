#!/usr/bin/env bash
# Build script per Linux/macOS. Produce ./planck nella radice.
set -e

# La versione viene iniettata dal tag git, non tenuta a mano nel sorgente:
# era gia' andata storta una volta (il binario si dichiarava 2.9.25 fino
# alla 2.9.28) e l'auto-update confronta versioni. Se git non e'
# disponibile resta il fallback definito in main.go.
VERSIONE=$(git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//')
LDFLAGS="-s -w"
if [ -n "$VERSIONE" ]; then
    LDFLAGS="$LDFLAGS -X main.Versione=$VERSIONE"
fi

go build -o planck -trimpath -ldflags="$LDFLAGS" ./cmd/planck
echo "Built ./planck ${VERSIONE:+(v$VERSIONE)}"
