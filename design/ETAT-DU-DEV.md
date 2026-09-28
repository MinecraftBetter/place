# État du développement — refonte EvenBetter

*Mis à jour le 28 septembre 2026. Branche `phase1-socle` (partie de `redesign`), poussée sur GitHub, sans PR. En ligne sur place.justbetter.fr derrière le compte à rebours (voir `deploy/justbetter/README.md`).*

Les six étapes du HANDOFF (§9) sont codées, plus les trois demandes ajoutées en cours de route :

- une bannière de profil dessinée pixel par pixel ;
- aucun délai entre deux pixels ;
- les modèles (« blueprints »), une image qui guide pour savoir quelle couleur poser où (retirés le 28/09, voir plus bas).

| Étape | Contenu | Commits |
|---|---|---|
| 1. Socle | comptes, sessions, WebSocket authentifié avec délai côté serveur, propriétaires des pixels (`owners.bin`), SQLite, inspecteur, invité en lecture seule, nouveau `/ace` mobile et desktop, `/login`, `/home` | `67d7de5` → `28423da` |
| 2. Profils | `/u/:pseudo`, `/moi/profil`, statistiques, carte des contributions, badges en direct, bannière dessinée | `26d21d8`, `79f5eaf` |
| + | index des 12 013 captures des archives en Go (portage de `analyse_sauvegardes.py`), modèles, pas de délai et anti-flood | `7330700`, `79f5eaf`, `2bfc3d8` |
| 3. Revendications | outil rectangle, lasso ou baguette ; analyse tirée des sauvegardes ; fil et votes ; file d'attente admin ; badges pionniers | `d987950`, `fb030c1` |
| 4. Communauté | activité, alertes « ton œuvre a bougé », classement, musée public, pages œuvre (avant/après, construction), visite guidée, timelapse sur les vraies captures | `b21df7b`, `fb3e4dc` |
| 5. Admin complet | tableau de bord, utilisateurs, outil zone, modération, journal annulable avec export CSV, réglages ; côté joueur : lecture seule, maintenance, bannière d'annonce, compte suspendu, « Signaler » | `9f802a4`, `45038e5` |
| 6. Musées perso | annuaire `/musees`, visite `/u/:pseudo/musee`, éditeur `/moi/musee`, livre d'or, coups de cœur, sons modérés | `07ec425` |

## Sortie : compte à rebours, bande-annonce, connexion, mise en ligne

- **Connexion JustBetter** : `/login` demande l'identifiant et le mot de passe JustBetter, vérifiés auprès de LLDAP comme sur justbetter.fr et JUST-TCG (`justbetter.go`, option `-lldap`).
  - Les membres des groupes `Admins` et `lldap_admin` sont admins.
  - Après 5 échecs en 10 minutes, la même adresse doit attendre.
- **Compte à rebours** (`/lancement`, le lien à envoyer sur Discord) : la mascotte, le décompte, « Ajouter à mon agenda », et un son facultatif pour les 10 dernières secondes.
  - À zéro, ta bande-annonce se lance (préchargée pendant la dernière minute), puis le bouton « Entrer sur le place ».
  - Pendant l'attente, le serveur renvoie les visiteurs vers le décompte : en mode « Tout fermé », personne sauf les admins ne dessine ; « Accueil seulement » laisse le reste ouvert.
  - À l'heure pile, tout s'ouvre sans intervention.
  - La date se règle dans `/admin/reglages` → Lancement, ou au premier démarrage avec `LAUNCH_AT`.
- **Aperçu Discord** : titre, description avec la date et image `img/og-lancement.png`. `/bande-annonce` annonce aussi la vidéo (`og:video`).
- **Bande-annonce** : ta vidéo (`~/Downloads/betterplace-trailer.mp4`), servie depuis `data/media/bande-annonce.mp4`. Elle est aussi sur `/bande-annonce`.
- **Mise en ligne** : voir `deploy/justbetter/README.md`.
  - Deux étapes : `stage.sh` depuis le PC, puis `sudo bash ~/place-deploy/switch.sh` sur le serveur.
  - La v1 est sauvegardée, le canvas conservé, et le retour arrière se fait avec `rollback.sh`.
  - L'image Docker a été construite et essayée en local avec podman.

## Retours du 28/09/2026 (tiago et Mika)

- **Revendications** : plus de limite de 5 demandes en attente.
- **Zones déjà revendiquées** : `/revendiquer` les montre dès l'ouverture, teintées et entourées selon leur vraie forme, avec leur titre. Or = œuvre validée, bleu = demande en attente.
  - Un bouton permet de les masquer.
  - Survoler une zone ou commencer une sélection dedans dit à qui elle est.
  - Les données viennent de `/api/zones` et `/api/zones.png` (`zones.go`).
- **Carte du profil** : elle éclaire aussi les œuvres revendiquées, pas seulement les pixels posés avec le compte.
- **Bouton « Revendiquer une œuvre »** : doré, dans la barre d'outils du canvas (ordi) et en haut (mobile). Il ouvre `/revendiquer` là où on regarde.
- **Modèles supprimés** : bouton, fenêtre, `blueprint.js`, `blueprint-ui.js`, `?modele=` et `/api/oeuvres/:id/reference.png`.
  - « Retoucher » (alerte « Ton œuvre a bougé ») laisse maintenant devant l'œuvre, avec un lien vers son avant / après.
- **« X connectés »** : compte les personnes (un compte = 1, peu importe le nombre d'onglets ; un invité = 1 par adresse). Le tableau de bord admin montre aussi le nombre d'onglets.
- **Inspecteur** : le cadre de l'œuvre n'apparaît qu'une fois le pixel épinglé (touche `I`) ou ouvert sur mobile, jamais au survol ni en zoomant, et jamais celui d'un pixel précédent. Pendant le chargement, il affiche « Recherche de son œuvre… » au lieu de « pas encore d'auteur ».
- **Musée** : les œuvres sont découpées selon leur forme (`/api/oeuvres/:id/image.png`, transparente autour), posées sur un fond sombre au lieu du blanc.
  - Les grands cadres (affiche, page de l'œuvre) suivent les proportions de l'œuvre.
  - « Rejouer la construction » utilise le même cadrage, découpé lui aussi. L'animation n'est plus déformée et se superpose exactement à l'œuvre dans les musées perso.
- **Timelapse** : le canvas entier tient à l'écran avec ses contrôles.
  - On zoome à la molette, en pinçant, en double-cliquant ou avec − ×1 +, et on se déplace en glissant.
  - Les pixels restent nets.

## Données réelles (mises à jour le 27/09/2026)

- **Canvas** : celui de place.justbetter.fr au 27/09/2026. L'ancien canvas de dev (copie du 28/03/2024) est remplacé.
- **Sauvegardes indexées : 17 489 captures, du 10/09/2022 au 27/09/2026.**
  - Les 30 zips mensuels du site (`archive-2022-09` → `archive-2025-04`) sont dans `~/BetterPlace/archives/`.
  - Le serveur ne fait plus de zips depuis avril 2025, mais `bak/` continue d'enregistrer les captures. Elles ont été copiées en lecture seule dans `~/BetterPlace/archives/serveur-bak/` (1 917 fichiers, 2025-05 → 2026-09-27).
  - 407 captures sont ignorées parce qu'elles sont vides ou tronquées, dont 371 dans `bak/` depuis 2025. L'ancien `timelapse.sh` les rangeait dans `corrupted/`.
- **Réanalyse automatique** : dès que l'index gagne des captures (vérification toutes les 10 min), les œuvres et les revendications en attente sont réanalysées.
  - Un badge d'or devenu vrai grâce aux nouvelles captures est donné aux auteurs, par exemple « Indémodable » après un an intact, ou « Bâtisseur ». Ils sont prévenus par une alerte.
  - Les badges déjà proposés à la validation restent la décision de l'admin.
  - Un bouton « Réanalyser maintenant » est dans `/admin/reglages`.
  - Première passe : 6 œuvres, et Zozo42 a gagné « Bâtisseur » pour « Pas vraiment ».
- **Timelapse** : un mois qui a des captures mais pas de zip n'est plus proposé en téléchargement. Il reste affiché, avec un cadre en pointillés.

Pour remettre à jour plus tard :

```bash
rsync -a -e "ssh -p 1025" --include='20??-??/***' --exclude='*' tiago@justbetter.fr:/mnt/JustFast/Configs/RPlace/bak/ ~/BetterPlace/archives/serveur-bak/
curl -o place.png https://place.justbetter.fr/place.png   # serveur de dev arrêté
```

## Lancer en local

```bash
cd ~/BetterPlace/place
GOTOOLCHAIN=local go build -o ./place-dev ./cmd/place
./place-dev -root ./web/root -load ./place.png -save ./place.png -devAuth -admins tiago,evan \
    -backups ~/BetterPlace/archives -port :8080
# une seule fois, dans un autre terminal : joueurs, œuvres, musées et livres d'or de démo
node design/outils/demo-seed.mjs
```

- Le fournisseur `-devAuth` laisse entrer avec n'importe quel pseudo, depuis `/login`. Les pseudos `tiago` et `evan` sont admins.
- Au premier lancement, l'index des sauvegardes met environ 3 minutes à se construire (`data/backups.idx`, 3 Mo). Il est ensuite rechargé tout de suite, puis complété toutes les 10 minutes.
- `place.png`, `owners.bin`, `data/` et `media/` sont des fichiers d'état ignorés par git.
- Tests : `GOTOOLCHAIN=local go test -race ./` (66 tests Go) et `node --test web/test/*.test.mjs`. Tout passe.

## Ce qu'il y a à regarder

1. **`/ace`** sur desktop et sur mobile. Poser, pipette, raccourcis `G` `L` `I` et les flèches, bouton « Revendiquer une œuvre », inspecteur, fil en direct.
2. **`/u/brindille`** et **`/moi/profil`** : bannière dessinée, badges épinglés, carte des contributions.
3. **`/revendiquer`**, **`/revendications`**, **`/admin/revendications`** : demande, votes, validation. L'analyse vient des vraies sauvegardes.
4. **`/activite`**, **`/classement`**, **`/musee`**, **`/musee/visite`**, **`/oeuvre/1`**, **`/timelapse`**.
5. **`/admin`** (connecté en `tiago`), toutes les entrées de la colonne de gauche.
   - Outil zone : essaie « Pixels d'un joueur » avec Zozo42, puis Restaurer.
   - Réglages : passe en « Lecture seule » ou en « Maintenance », puis regarde `/ace` dans un autre navigateur.
6. **`/musees`**, **`/u/brindille/musee`** (mets le son), **`/u/mamie-pixel/musee`** (visite guidée), puis **`/moi/musee`**.

## Choix faits sans toi (faciles à changer)

- **Délai entre deux pixels** : 0 s par défaut, comme tu l'as demandé. Un anti-flood invisible bloque au-delà de 30 px/s (`-maxRate`). Les deux se règlent aussi dans `/admin/reglages`, avec un délai séparé pour les comptes de moins d'une heure.
- **Musique des musées** : les trois airs (Vagues 8-bit, Saloon au piano, Désert de nuit) et les sons au passage (vague, clochette, pièce, mouette, pas) sont **synthétisés dans le navigateur** (`web/root/js/chiptune.js`). Il n'y a donc aucun fichier audio à héberger ni à modérer. Les sons importés par les joueurs passent par la modération. Un son en attente n'est audible que par son propriétaire, et « Garder » dans la modération vaut acceptation.
- **Visibilité d'un musée** : la maquette propose « Amis », mais le site n'a pas de liste d'amis. J'ai mis « Joueurs connectés » à la place.
- **`/moi/musee/oeuvre/:id`** ouvre l'éditeur avec l'œuvre déjà sélectionnée, plutôt qu'un écran séparé : sur mobile, le panneau de l'œuvre est juste en dessous.
- **Livre d'or** : un message qui contient un mot interdit est refusé tout de suite, sans passer par la file de modération. Les autres messages se signalent avec le drapeau.
- **Visites** : chaque visiteur compte pour une visite par jour et par musée. Le propriétaire ne compte pas.
- **Journal** : chaque action admin garde l'état d'avant et reste annulable 30 jours. Cela vaut pour les zones, les attributions, les sanctions, la modération (y compris « Garder » un son) et les réglages.

## Ce qui reste à faire

- **Déploiement** : tout est prêt, il ne manque que la date de sortie pour lancer `stage.sh`.
- **Serveur actuel** : `timelapse.sh` ne produit plus d'archives zip (ni sans doute `timelapse.mp4`) depuis avril 2025. Seul `backup.sh` tourne encore. À relancer côté serveur si tu veux les zips des mois récents.
- **Admin** : le motif d'une suspension et le message à un joueur se saisissent encore dans les fenêtres natives du navigateur (`prompt()`). Une vraie modale serait plus propre.
- **Musées** : l'animation « Rejouer sa construction » n'existe que pour les œuvres présentes dans les sauvegardes. Pour les autres, l'œuvre reste fixe.
