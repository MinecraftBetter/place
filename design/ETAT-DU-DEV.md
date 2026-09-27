# État du développement — refonte EvenBetter

*Mis à jour le 27 septembre 2026. Branche locale `phase1-socle` (partie de `redesign`), **non poussée**. Rien n'a été déployé.*

Les six étapes du HANDOFF (§9) sont codées, plus les trois demandes ajoutées en cours de route :

- une bannière de profil dessinée pixel par pixel ;
- aucun délai entre deux pixels ;
- les modèles (« blueprints »), une image qui guide pour savoir quelle couleur poser où.

| Étape | Contenu | Commits |
|---|---|---|
| 1. Socle | comptes, sessions, WebSocket authentifié avec délai côté serveur, propriétaires des pixels (`owners.bin`), SQLite, inspecteur, invité en lecture seule, nouveau `/ace` mobile et desktop, `/login`, `/home` | `67d7de5` → `28423da` |
| 2. Profils | `/u/:pseudo`, `/moi/profil`, statistiques, carte des contributions, badges en direct, bannière dessinée | `26d21d8`, `79f5eaf` |
| + | index des 12 013 captures des archives en Go (portage de `analyse_sauvegardes.py`), modèles, pas de délai et anti-flood | `7330700`, `79f5eaf`, `2bfc3d8` |
| 3. Revendications | outil rectangle, lasso ou baguette ; analyse tirée des sauvegardes ; fil et votes ; file d'attente admin ; badges pionniers | `d987950`, `fb030c1` |
| 4. Communauté | activité, alertes « ton œuvre a bougé », classement, musée public, pages œuvre (avant/après, construction), visite guidée, timelapse sur les vraies captures | `b21df7b`, `fb3e4dc` |
| 5. Admin complet | tableau de bord, utilisateurs, outil zone, modération, journal annulable avec export CSV, réglages ; côté joueur : lecture seule, maintenance, bannière d'annonce, compte suspendu, « Signaler » | `9f802a4`, `45038e5` |
| 6. Musées perso | annuaire `/musees`, visite `/u/:pseudo/musee`, éditeur `/moi/musee`, livre d'or, coups de cœur, sons modérés | `07ec425` |

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

1. **`/ace`** sur desktop et sur mobile. Poser, pipette, raccourcis `G` `L` `I` `M` et les flèches, modèle (icône plan), inspecteur, fil en direct.
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

- **Connexion JustBetter (OIDC)** : seul le fournisseur de dev existe pour l'instant. Tant qu'elle n'est pas branchée, on ne peut pas déployer (voir `PLAN-PHASE-1.md` §8, point 1).
- **Déploiement et push** : rien n'a été fait, j'attends ton feu vert.
- **Admin** : le motif d'une suspension et le message à un joueur se saisissent encore dans les fenêtres natives du navigateur (`prompt()`). Une vraie modale serait plus propre.
- **Musées** : l'animation « Rejouer sa construction » n'existe que pour les œuvres présentes dans les sauvegardes. Pour les autres, l'œuvre reste fixe.
