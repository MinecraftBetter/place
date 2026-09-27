#!/bin/bash
# Run ON the server when switch.sh stopped at « port is already allocated »:
#   sudo bash ~/place-deploy/finish.sh
# The v1 container was not created by this compose project (other name), so it still holds
# port 20003. It is stopped — kept, without automatic restart, for a rollback — and the new
# place starts. The code, compose file and backup are already in place (switch.sh).
set -euo pipefail
PLACE=${PLACE:-/mnt/JustFast/Configs/RPlace}
PORT=${PORT:-20003}
[ "$(id -u)" = 0 ] || { echo "À lancer avec sudo."; exit 1; }
cd "$PLACE"
NEW="$(basename "$PLACE" | tr '[:upper:]' '[:lower:]')-place-1"
mapfile -t OLD < <(docker ps --filter "publish=$PORT" --format '{{.Names}}' | grep -vx "$NEW" || true)
if [ ${#OLD[@]} -gt 0 ]; then
	echo "Conteneur qui tient le port $PORT :"
	for c in "${OLD[@]}"; do echo "  $c ($(docker inspect -f '{{.Config.Image}}, {{.State.Status}} depuis {{.State.StartedAt}}' "$c"))"; done
	read -r -p "L'arrêter (il est gardé) et démarrer le nouveau place ? [o/N] " ok
	[[ "$ok" =~ ^[oOyY]$ ]] || { echo "Rien n'a changé."; exit 1; }
	for c in "${OLD[@]}"; do
		docker update --restart=no "$c" >/dev/null
		docker stop "$c" >/dev/null
		echo "$c arrêté, sans redémarrage automatique."
	done
fi
docker compose up -d
for i in $(seq 1 60); do
	if curl -fs "http://127.0.0.1:$PORT/api/status" >/dev/null; then
		echo "Le nouveau place répond. Il indexe les sauvegardes en arrière-plan (quelques minutes)."
		[ ${#OLD[@]} -gt 0 ] && echo "Retour à l'ancien si besoin : sudo docker compose -f $PLACE/docker-compose.yml stop && sudo docker update --restart=always ${OLD[*]} && sudo docker start ${OLD[*]}"
		exit 0
	fi
	sleep 2
done
echo "Le nouveau place ne répond pas : sudo docker compose -f $PLACE/docker-compose.yml logs --tail 50 place"
[ ${#OLD[@]} -gt 0 ] && echo "Retour à l'ancien : sudo docker compose -f $PLACE/docker-compose.yml stop && sudo docker update --restart=always ${OLD[*]} && sudo docker start ${OLD[*]}"
exit 1
