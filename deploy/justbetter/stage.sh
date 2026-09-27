#!/bin/bash
# Run on this PC: copies the code of the current commit to the server, in tiago's home
# (/mnt/JustFast/Home/tiago/place-deploy). Nothing of the running place is touched.
#   bash deploy/justbetter/stage.sh 2026-10-24T20:00 [~/Downloads/betterplace-trailer.mp4]
set -euo pipefail
AT="${1:?date de sortie, heure de Paris : 2026-10-24T20:00}"
VIDEO="${2:-$HOME/Downloads/betterplace-trailer.mp4}"
[ -f "$VIDEO" ] || { echo "Bande-annonce introuvable : $VIDEO"; exit 1; }
[[ "$AT" =~ ^20[0-9]{2}-[01][0-9]-[0-3][0-9]T[0-2][0-9]:[0-5][0-9]$ ]] || { echo "Date invalide : $AT"; exit 1; }
cd "$(git rev-parse --show-toplevel)"
[ -z "$(git status --porcelain)" ] || { echo "Des modifications ne sont pas commitées."; exit 1; }
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/src"
git archive HEAD | tar -x -C "$TMP/src"
rm -rf "$TMP/src/design"                    # maquettes : inutiles sur le serveur
sed "s/__LAUNCH_AT__/$AT/" deploy/justbetter/docker-compose.yml > "$TMP/docker-compose.yml"
cp deploy/justbetter/switch.sh deploy/justbetter/rollback.sh "$TMP/"
# the trailer and its picture (the last second: logo and mascot)
mkdir -p "$TMP/media"
cp "$VIDEO" "$TMP/media/bande-annonce.mp4"
DUR=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$VIDEO")
ffmpeg -hide_banner -loglevel error -y -ss "$(echo "$DUR - 1.5" | bc)" -i "$VIDEO" -frames:v 1 -q:v 3 "$TMP/media/bande-annonce.jpg"
git rev-parse --short HEAD > "$TMP/COMMIT"
SSH="ssh -o BatchMode=yes -o ConnectTimeout=15 -p 1025"
rsync -a --delete -e "$SSH" "$TMP/" tiago@justbetter.fr:/mnt/JustFast/Home/tiago/place-deploy/
echo "Prêt sur le serveur : ~/place-deploy (commit $(cat "$TMP/COMMIT"), sortie $AT)."
echo "Bascule : ssh -p 1025 tiago@justbetter.fr   puis   sudo bash ~/place-deploy/switch.sh"
