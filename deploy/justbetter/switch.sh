#!/bin/bash
# Run ON the server, by tiago:   sudo bash ~/place-deploy/switch.sh
# 1. saves the v1 (code, compose, canvas) in ~/backups/place-v1-<date>
# 2. puts the new code in /mnt/JustFast/Configs/RPlace/src (the canvas place.png and .git stay)
# 3. builds the image, then swaps the container (a few seconds of downtime)
# Undo: sudo bash ~/place-deploy/rollback.sh ~/backups/place-v1-<date>
set -euo pipefail
# (the variables below can be overridden to rehearse the switch elsewhere)
PLACE=${PLACE:-/mnt/JustFast/Configs/RPlace}
STAGE=$(dirname "$(readlink -f "$0")")
BACKUP=${BACKUP_ROOT:-/mnt/JustFast/Home/tiago/backups}/place-v1-$(date +%Y%m%d-%H%M%S)
OWNER=${OWNER:-evan:admin}
COMPOSE=${COMPOSE:-docker compose}
HEALTH=${HEALTH:-http://127.0.0.1:20003/api/status}
[ "$(id -u)" = 0 ] || [ -n "${REHEARSAL:-}" ] || { echo "À lancer avec sudo."; exit 1; }
[ -s "$PLACE/src/place.png" ] || { echo "Canvas introuvable : $PLACE/src/place.png"; exit 1; }
[ -f "$STAGE/src/cmd/place/main.go" ] || { echo "Code introuvable dans $STAGE/src (lance stage.sh d'abord)."; exit 1; }

echo "== Sauvegarde de la v1 dans $BACKUP"
mkdir -p "$BACKUP"
cp -a "$PLACE/docker-compose.yml" "$BACKUP/docker-compose.yml"
cp -a "$PLACE/src/place.png" "$BACKUP/place.png"
tar -C "$PLACE" -czf "$BACKUP/src.tgz" src
[ -n "${REHEARSAL:-}" ] || chown -R tiago:admin "$BACKUP"

echo "== Nouveau code (commit $(cat "$STAGE/COMMIT" 2>/dev/null || echo ?))"
rsync -a --checksum --delete --chown="$OWNER" --exclude /place.png --exclude /.git "$STAGE/src/" "$PLACE/src/"
cp "$PLACE/docker-compose.yml" "$PLACE/docker-compose.v1.yml"
install -o "${OWNER%:*}" -g "${OWNER#*:}" -m 644 "$STAGE/docker-compose.yml" "$PLACE/docker-compose.yml"
install -d -o "${OWNER%:*}" -g "${OWNER#*:}" "$PLACE/data" "$PLACE/data/media"
for f in bande-annonce.mp4 bande-annonce.jpg; do
	[ -f "$STAGE/media/$f" ] && install -o "${OWNER%:*}" -g "${OWNER#*:}" -m 644 "$STAGE/media/$f" "$PLACE/data/media/$f"
done

cd "$PLACE"
echo "== Construction de l'image (quelques minutes, l'ancien place tourne encore)"
$COMPOSE build
echo "== Bascule"
$COMPOSE up -d
for i in $(seq 1 60); do
	if curl -fs "$HEALTH" >/dev/null; then
		echo "Le nouveau place répond. Il indexe les sauvegardes en arrière-plan (quelques minutes)."
		echo "Retour arrière si besoin : sudo bash $STAGE/rollback.sh $BACKUP"
		exit 0
	fi
	sleep 2
done
echo "Le nouveau place ne répond pas : docker compose logs --tail 50 place"
echo "Retour arrière : sudo bash $STAGE/rollback.sh $BACKUP"
exit 1
