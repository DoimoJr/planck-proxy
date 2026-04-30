#!/bin/bash
# Veyon test rig entrypoint.
#
# Idempotente: alla prima run genera la chiave master, alle successive
# riusa quella esistente (utile se l'utente monta /etc/veyon come volume).
set -e

KEY_NAME="planck-test"
KEY_DIR="/etc/veyon/keys"
PUB_KEY="$KEY_DIR/public/$KEY_NAME/key"
PRIV_KEY="$KEY_DIR/private/$KEY_NAME/key"

mkdir -p "$KEY_DIR/public/$KEY_NAME" "$KEY_DIR/private/$KEY_NAME"

# Genera la coppia di chiavi se non esiste.
if [ ! -f "$PRIV_KEY" ]; then
    echo "[rig] Generazione chiave $KEY_NAME..."
    veyon-cli authkeys create "$KEY_NAME"
    # veyon-cli salva le chiavi in $KEY_DIR; verifico esistenza
    if [ ! -f "$PRIV_KEY" ]; then
        echo "[rig] WARN: chiave non trovata in $PRIV_KEY dopo create. Path effettivo:"
        find /etc/veyon -name "key" 2>/dev/null || true
    fi
fi

# Esporta la chiave pubblica e privata in /export per il client Planck.
mkdir -p /export
if [ -f "$PUB_KEY" ]; then
    cp "$PUB_KEY" /export/${KEY_NAME}_public.pem
    echo "[rig] Public key esportata in /export/${KEY_NAME}_public.pem"
fi
if [ -f "$PRIV_KEY" ]; then
    cp "$PRIV_KEY" /export/${KEY_NAME}_private.pem
    echo "[rig] Private key esportata in /export/${KEY_NAME}_private.pem"
fi

# Configura Veyon: keyfile auth, listening sulla porta default 11100.
# Authentication/Method e' un enum (VeyonCore::AuthenticationMethod):
#   0 = LogonAuthentication, 1 = KeyFileAuthentication.
veyon-cli config set "Authentication/Method" 1 || true
veyon-cli config set "Network/PrimaryServicePort" 11100 || true
# Override path chiavi (default e' %GLOBALAPPDATA% — broken su Linux).
veyon-cli config set "Authentication/PrivateKeyBaseDir" "/etc/veyon/keys/private" || true
veyon-cli config set "Authentication/PublicKeyBaseDir" "/etc/veyon/keys/public" || true
# Permetti la connessione di chiunque (in test rig non ci interessa l'ACL).
veyon-cli config set "Authentication/RequiredGroup" "" || true

# Avvia xvfb headless (display :99) — alcune funzioni Veyon richiedono X.
echo "[rig] Avvio Xvfb su :99..."
Xvfb :99 -screen 0 1024x768x24 &
export DISPLAY=:99
sleep 1

# D-Bus session per veyon-service.
echo "[rig] Avvio dbus..."
mkdir -p /run/dbus
dbus-daemon --system --fork || true
eval "$(dbus-launch --sh-syntax)"

# Lancia il server in foreground.
echo "[rig] Avvio veyon-server (porta 11100)..."
exec veyon-server
