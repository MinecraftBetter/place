#!/bin/bash
# Run ON the server:   sudo bash ~/place-deploy/rollback.sh ~/backups/place-v1-<date>
# Puts the v1 code and compose back and rebuilds it. The canvas keeps the pixels placed
# since (place.png is shared); data/ (accounts…) stays for a later retry.
set -euo pipefail
PLACE=/mnt/JustFast/Configs/RPlace
BACKUP="${1:?dossier de sauvegarde, ex. ~/backups/place-v1-20261024-190000}"
[ "$(id -u)" = 0 ] || { echo "À lancer avec sudo."; exit 1; }
[ -f "$BACKUP/src.tgz" ] && [ -f "$BACKUP/docker-compose.yml" ] || { echo "Sauvegarde incomplète : $BACKUP"; exit 1; }
cd "$PLACE"
docker compose stop                          # the canvas is written one last time
cp -a src/place.png /tmp/place-current.png
rm -rf src.new && mkdir src.new && tar -C src.new -xzf "$BACKUP/src.tgz"
rm -rf src.new-old && mv src src.new-old && mv src.new/src src && rmdir src.new
cp -a /tmp/place-current.png src/place.png
cp -a "$BACKUP/docker-compose.yml" docker-compose.yml
docker compose build && docker compose up -d
rm -rf src.new-old
echo "La v1 est revenue (canvas actuel conservé)."
