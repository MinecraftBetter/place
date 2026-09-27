#!/bin/sh
# /place.png: the canvas (a file mounted from the host), /bak: the captures of backup.sh,
# /data: accounts database, pixel owners, uploads, backup index (a directory mounted from the host).
# LLDAP_URL turns on the JustBetter login; ADMIN_GROUPS lists the LLDAP groups of the admins.
# LAUNCH_AT (Paris time, 2006-01-02T15:04) and LAUNCH_GATE (tout | accueil) set the release
# countdown at the first start; it is then managed in /admin/reglages.
crond -f -d 8 &
mkdir -p /data
exec /main -load /place.png -save /place.png -saveInterval 30 \
	-owners /data/owners.bin -db /data/betterplace.db -data /data \
	-backups /bak,/web/root/archives \
	${LLDAP_URL:+-lldap "$LLDAP_URL"} -adminGroups "${ADMIN_GROUPS:-admins,lldap_admin}" \
	${ADMINS:+-admins "$ADMINS"} ${LAUNCH_AT:+-launchAt "$LAUNCH_AT" -launchGate "${LAUNCH_GATE:-tout}"} "$@"
