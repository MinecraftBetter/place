# Mise en ligne sur justbetter.fr

Le place tourne dans `/mnt/JustFast/Configs/RPlace` (Docker, port 20003, nginx `justbetter-place.conf` inchangé).
Ces fichiers appartiennent à evan : la bascule se fait avec `sudo`, par tiago.

## 1. Préparer (depuis le PC, sans rien toucher au place en ligne)

```bash
cd ~/BetterPlace/place
bash deploy/justbetter/stage.sh 2026-10-24T20:00     # date et heure de sortie, heure de Paris
# (2e argument facultatif : la bande-annonce, par défaut ~/Downloads/betterplace-trailer.mp4)
```

Ces fichiers sont copiés dans `~/place-deploy` sur le serveur :
- le code du commit actuel ;
- le `docker-compose.yml` avec la date ;
- la bande-annonce et son image d'attente, qui iront dans `data/media`.

## 2. Basculer (sur le serveur)

```bash
ssh -p 1025 tiago@justbetter.fr
sudo bash ~/place-deploy/switch.sh
```

- La v1 est sauvegardée dans `~/backups/place-v1-<date>` : code, compose et canvas.
- Le nouveau code remplace `src/`. `src/place.png` (le canvas) et `src/.git` restent en place.
- L'image est construite pendant que l'ancien place tourne encore, puis le conteneur est remplacé. La coupure dure quelques secondes.
- Si l'ancien conteneur porte un autre nom (erreur « port is already allocated »), `finish.sh` l'arrête après confirmation, le garde sans redémarrage automatique, puis démarre le nouveau. On peut le relancer seul : `sudo bash ~/place-deploy/finish.sh`.
- La bande-annonce est installée dans `data/media`. Elle passe à zéro sur le compte à rebours, et on la retrouve sur `/bande-annonce`.
- Au premier démarrage :
  - les sauvegardes (`bak/` et `archives/`) sont indexées en quelques minutes ;
  - le compte à rebours s'active avec la date choisie ;
  - les membres des groupes LLDAP `Admins` et `lldap_admin` sont admins.

## 3. Après

- **Compte à rebours** : il se règle dans `/admin/reglages` → Lancement (date, « Tout fermé » ou « Accueil seulement »). Les admins voient le site normalement pendant le décompte.
- **Retour arrière** : `sudo bash ~/place-deploy/rollback.sh ~/backups/place-v1-<date>`. La v1 revient avec le canvas actuel.
- **Nginx** : rien à changer. Le site transmet déjà l'hôte, le HTTPS et le WebSocket, et `client_max_body_size 0` autorise les sons des musées.
