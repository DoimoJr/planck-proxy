# Veyon test rig — server headless per validare il client Phase 3.
#
# NON e' una immagine production: solo per dev, Wireshark trace, e test
# del wire protocol. Niente desktop env, niente VNC, niente sessione utente.
#
# Build:  docker build -t planck-veyon-rig test/veyon-rig
# Run:    docker run --rm -p 11100:11100 planck-veyon-rig
#
# Espone:
#   - 11100/tcp -> veyon-server
# Volumi:
#   - /etc/veyon -> config + chiavi (puoi montarci una dir host se vuoi
#     persistere le chiavi master tra run)

FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# Veyon ufficiale via PPA (richiede add-apt-repository).
RUN apt-get update && apt-get install -y --no-install-recommends \
        software-properties-common \
        gnupg \
        ca-certificates \
        curl \
    && add-apt-repository -y ppa:veyon/stable \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
        veyon \
        xvfb \
        dbus-x11 \
    && rm -rf /var/lib/apt/lists/*

# Script di entrypoint: genera chiave master se non esiste, configura il
# server per usare keyfile auth, lancia xvfb + dbus + veyon-server.
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 11100

ENTRYPOINT ["/entrypoint.sh"]
