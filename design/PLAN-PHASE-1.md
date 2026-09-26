# Phase 1 · Socle — plan d'implémentation

> Rédigé le 26/09/2026 à partir de `design/HANDOFF.md` (§1, §5, §6, §7.1, §9, §10) et de `design/DEMARRER-AVEC-CLAUDE-CODE.md`.
> Branche : `phase1-socle`, créée depuis `redesign`. Rien n'est poussé sur GitHub ; `master` n'est pas touché.

**Objectif.** Ajouter les comptes : un fournisseur « dev » en attendant JustBetter, des sessions, un WebSocket authentifié avec un délai entre deux pixels appliqué par le serveur, des invités en lecture seule, la propriété des pixels et un inspecteur. Côté client : le nouveau canvas mobile et desktop avec le design system `bp.css`, et le lien de position.

**Architecture.** On garde le binaire Go actuel (`cmd/place` + package `place` à la racine), découpé en fichiers par responsabilité. Il gagne une base SQLite (pilote pur Go, sans CGO, donc compatible avec le Dockerfile `busybox`) et une API JSON sous `/api/`. Le client reste en fichiers statiques, sans étape de build : modules ES natifs, `bp.css` repris tel quel, `glwindow.js` conservé.

---

## 1. Choix techniques (simples et légers)

| Sujet | Choix | Pourquoi |
|---|---|---|
| Backend | Go, même module `github.com/minecraftbetter/place`, `go 1.21` dans `go.mod` (le Dockerfile `golang:1.21` compile toujours) | Rien à réapprendre ; le serveur actuel est déjà là |
| Base | **SQLite** via `modernc.org/sqlite` v1.36.1 (pur Go, `CGO_ENABLED=0` reste possible), WAL, `busy_timeout` | Un seul fichier `betterplace.db`, zéro service à côté |
| Dépendances ajoutées | `modernc.org/sqlite` seulement | Sessions, jetons et JSON viennent de la bibliothèque standard |
| Routage | `http.ServeMux` devant `httpfilter` : `/api/` et `/auth/` → Go ; le reste passe par `httpfilter` et `_filters.txt` comme avant | `/home`, `/ace`, `/timelapse`, `/ws`, `/place.png`, `/stat` inchangés |
| Front | HTML + CSS + **modules ES natifs**, sans framework ni bundler | Rien à compiler, se déploie comme aujourd'hui (copie de `web/`) |
| CSS | `web/root/css/bp.css` = copie de `design/canvas/bp.css` + `css/app.css` pour la mise en page réelle | La source de vérité reste le design system |
| Polices | Google Fonts (Pixelify Sans, Instrument Sans, JetBrains Mono), `display=swap` + polices système de repli | Comme les maquettes ; auto-hébergement possible plus tard |
| Rendu | `glwindow.js` conservé (mêmes shaders, même texture) ; ajout de quelques méthodes (caméra, conversions écran ↔ pixel) | Contrainte du HANDOFF §1 |

## 2. Structure des fichiers

```
server.go            → réduit au routage HTTP du package (ServeHTTP, /place.png, /stat)
canvas.go            → image + owners []uint32 + PNG en cache + owners.bin (lecture/écriture/agrandissement)
hub.go               → clients WebSocket, diffusion non bloquante, messages stat, délai par joueur
store.go             → SQLite : schéma, migrations, users, sessions, pixel_events, compteurs
auth.go              → sessions (cookie bp_session), interface Provider, fournisseur « dev »
api.go               → /api/me, /api/pixel, /api/owners, /api/users
*_test.go            → tests Go (voir §6)
cmd/place/main.go    → nouveaux flags, ServeMux, sauvegarde périodique place.png + owners.bin, arrêt propre

web/root/
  _filters.txt       → + « login login.html »
  desktop.html       → /ace : nouveau canvas (mobile et desktop dans le même fichier, responsive)
  login.html         → /login : « Mets ton chapeau » (+ comptes dev si activés)
  holdup.html        → /home : accueil refait (m-accueil / d-accueil), si le temps le permet
  glwindow.js        → conservé, devient un module ES (+ méthodes caméra)
  css/bp.css, css/app.css
  img/               → cube-logo.png, farwest-x8.png, desert.png (copiés depuis design/assets)
  js/
    app.js           → démarrage de /ace : thème, /api/me, chargement, WS, contrôles
    net.js           → WebSocket : reconnexion 3 essais, ping/pong, pixels en attente, messages typés
    palette.js       → les 30 couleurs et leurs noms, analyse hex, nom d'une couleur (sans DOM)
    view.js          → lien de position ?x&y&z, calculs caméra (sans DOM)
    owners.js        → carte des propriétaires (décodage PNG RGB 24 bits) + cache des joueurs
    inspector.js     → carte d'inspection (desktop) et tiroir (mobile)
    desktop.js       → souris et clavier historiques + raccourcis G, I, L, Échap, flèches/Entrée
    mobile.js        → viser, loupe 7×7, Poser, pincer, deux doigts, appui long
    ui.js            → petits utilitaires DOM : toast, tiroir, thème
  test/*.test.mjs    → tests node --test des modules sans DOM
```

Anciens fichiers : `main.js`, `place.js` et `desktop.css` sont remplacés par les modules ci-dessus. `timelapse.html` n'est pas modifié dans cette phase.

## 3. Stockage

### 3.1 En mémoire et sur disque, à côté de l'image

- `owners []uint32` (W × H) : index du joueur qui a posé le pixel visible, `0` = pixel d'avant les comptes (« œuvre d'origine »).
- `owners.bin` écrit au même rythme que `place.png` (`-saveInterval`) et à l'arrêt. En-tête : `"BPOW"`, version, largeur, hauteur, horodatage de sauvegarde (ms), puis W × H `uint32` little-endian.
- Si l'image et `owners.bin` n'ont pas la même taille (le canvas a grandi), les propriétaires sont recopiés en haut à gauche, comme dans l'analyse des sauvegardes.
- **Reprise après plantage** : au démarrage, les `pixel_events` postérieurs à l'horodatage de `owners.bin` sont rejoués sur l'image et sur les propriétaires. Aujourd'hui, un plantage perd jusqu'à `saveInterval` secondes de pixels ; avec ce rejeu, plus rien n'est perdu.

### 3.2 Tables SQLite de la phase 1

Sous-ensemble du schéma du HANDOFF §7.2 ; les autres tables arrivent avec leur phase.

```sql
CREATE TABLE users (
  id              INTEGER PRIMARY KEY,          -- = valeur stockée dans owners (jamais 0)
  fournisseur     TEXT NOT NULL,                -- 'dev' | 'justbetter'
  id_externe      TEXT NOT NULL,                -- identifiant chez le fournisseur (sub OIDC, ou pseudo en dev)
  pseudo          TEXT NOT NULL,
  slug            TEXT NOT NULL UNIQUE,         -- → /u/:pseudo (phase 2)
  avatar_url      TEXT NOT NULL DEFAULT '',
  banner          TEXT NOT NULL DEFAULT '',
  bio             TEXT NOT NULL DEFAULT '',
  accent          TEXT NOT NULL DEFAULT '',
  couleur_pref    TEXT NOT NULL DEFAULT '',
  role            TEXT NOT NULL DEFAULT 'joueur',   -- joueur | admin
  statut          TEXT NOT NULL DEFAULT 'actif',    -- actif | suspendu | banni
  suspendu_jusqua INTEGER,                          -- ms, NULL si pas suspendu
  pixels_poses    INTEGER NOT NULL DEFAULT 0,       -- compteur incrémental
  pixels_visibles INTEGER NOT NULL DEFAULT 0,       -- +1 à la pose, −1 pour l'ancien propriétaire
  cree_le         INTEGER NOT NULL,
  UNIQUE (fournisseur, id_externe)
);

CREATE TABLE sessions (
  token_hash BLOB PRIMARY KEY,       -- SHA-256 du jeton ; le jeton lui-même n'est que dans le cookie
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  cree_le    INTEGER NOT NULL,
  expire_le  INTEGER NOT NULL
);

CREATE TABLE pixel_events (          -- journal append-only
  id           INTEGER PRIMARY KEY,
  x            INTEGER NOT NULL,
  y            INTEGER NOT NULL,
  color        INTEGER NOT NULL,     -- 0xRRGGBB
  user_id      INTEGER NOT NULL,
  prev_color   INTEGER NOT NULL,     -- couleur remplacée (historique, badge Restaurateur plus tard)
  prev_user_id INTEGER NOT NULL,     -- propriétaire remplacé (0 = œuvre d'origine)
  ts           INTEGER NOT NULL      -- ms
);
CREATE INDEX pixel_events_xy   ON pixel_events (x, y, id);
CREATE INDEX pixel_events_user ON pixel_events (user_id, id);
CREATE INDEX pixel_events_ts   ON pixel_events (ts);

CREATE TABLE schema_version (version INTEGER NOT NULL);
```

Écart avec le HANDOFF : `justbetter_id` devient `fournisseur` + `id_externe`, pour que les faux comptes du mode dev ne puissent jamais se mélanger aux vrais comptes JustBetter.

## 4. Endpoints HTTP

| Méthode | Route | Réponse | Notes |
|---|---|---|---|
| GET | `/place.png`, `/stat`, `/ws` | inchangés | `/stat` garde `{"connections","slots"}` |
| GET | `/api/me` | `{"user": {id, pseudo, slug, avatar, accent, role} \| null, "cooldown": 5, "ready_in": 0, "auth": ["dev"], "width", "height"}` | 200 même en invité, pour éviter des erreurs dans la console |
| GET | `/api/pixel?x=&y=` | `{x, y, color:"#RRGGBB", owner: {…}\|null, origin: bool, placed_at: ms\|null, history: [{color, user, ts}] (3 derniers)}` | `owner` null + `origin` = œuvre d'origine |
| GET | `/api/owners` | PNG W × H, RGB = index du joueur sur 24 bits | Chargé après l'image, pour un inspecteur instantané au survol |
| GET | `/api/users?ids=1,2,3` | `{"users": {"1": {…}, …}}` | Pseudos et avatars pour l'inspecteur (50 ids au plus) |
| GET | `/auth/dev` | page de choix d'un faux compte | Seulement si `-devAuth` |
| POST | `/auth/dev` | `pseudo` → session → redirection vers `next` (par défaut `/ace`) | Seulement si `-devAuth` |
| GET | `/auth/justbetter`, `/auth/justbetter/callback` | — | **Pas dans cette phase** : l'interface `Provider` est prête |
| POST | `/auth/logout` | supprime la session → `/ace` | |
| GET | `/login` | `login.html` | Page statique ; elle demande `/api/me` pour savoir quels fournisseurs afficher |

Sécurité :
- Cookie `bp_session` : `HttpOnly`, `SameSite=Lax`, `Secure` si la requête arrive en HTTPS (TLS ou `X-Forwarded-Proto`), valable 30 jours.
- Les `POST` vérifient `Origin` quand l'en-tête est présent.
- Le WebSocket n'accepte la session que si `Origin` correspond à l'hôte. Sinon, la socket reste ouverte en lecture seule : un site tiers ne peut pas poser de pixels à la place d'un joueur.
- `next` n'accepte que des chemins locaux (pas de redirection ouverte).

## 5. WebSocket `/ws` (HANDOFF §6)

```jsonc
// client → serveur : inchangé
{"x": 470, "y": 300, "color": {"R": 94, "G": 179, "B": 255, "A": 255}}
"ping"                                   // → "pong"

// serveur → clients
{"x": 470, "y": 300, "color": {...}, "u": 42, "t": 1790416853000}   // pixel diffusé, avec auteur et heure
{"type": "error", "err": "cooldown", "retry": 3.2, "x": 470, "y": 300}  // err : auth | cooldown | suspended | banned | bounds
{"type": "stat", "online": 27, "slots": 64}                         // à la connexion, puis quand le nombre change (2 s max)
```

- Session lue au handshake. Sans session, la socket est en lecture seule et toute écriture reçoit `auth`.
- Délai par joueur (flag `-cooldown`, 5 s par défaut), vérifié par le serveur. Le refus renvoie `retry` et la position du pixel, pour que le client annule son affichage optimiste.
- Un message invalide (JSON illisible) coupe la connexion, comme aujourd'hui. Un pixel refusé ne coupe plus rien : le serveur répond avec une erreur.
- Diffusion non bloquante : chaque client a une file de 256 messages. Un client trop lent est déconnecté, au lieu de bloquer tout le monde comme aujourd'hui.
- `mode`, `announce`, `activity` et `alert` viendront avec les phases 4 et 5.

## 6. Tests

- **Go (`go test -race ./...`)** :
  - `canvas_test` : pose, hors limites, propriétaires ; aller-retour `owners.bin` ; agrandissement ; PNG en cache invalidé.
  - `store_test` : création et récupération d'un compte, slug unique ; sessions (création, lecture, expiration, suppression) ; `pixel_events` + compteurs (`pixels_visibles` −1 pour l'ancien propriétaire) ; événements après un instant donné.
  - `auth_test` : connexion dev (cookie posé, `next` local seulement) ; déconnexion ; `-devAuth` absent → 404.
  - `hub_test` (vrai serveur `httptest` + client gorilla) : invité → `auth` ; connecté → diffusion avec `u` et `t` ; second pixel trop tôt → `cooldown` + `retry` ; ping/pong ; `stat` ; client lent sans blocage ; ancien format de message accepté.
  - `api_test` : `/api/me` invité et connecté ; `/api/pixel` (origine, historique) ; `/api/owners` décodé ; `/api/users`.
  - `go vet ./...`.
- **JS (`node --test web/root/test/`)** : palette (noms, hex libre, `#abc`, casse), lien de position (lecture, bornes, écriture), calculs caméra, décodage des propriétaires.
- **Navigateur** : Chromium headless via Playwright (déjà dans le cache local) sur un serveur lancé avec `-devAuth`. Scénarios :
  - invité : chargement, bandeau, inspecteur ;
  - connexion dev ;
  - pose d'un pixel (desktop clic droit ; mobile viser + Poser en 375 × 812 tactile), puis délai affiché ;
  - lien `?x&y&z` ;
  - thème clair.

  Captures d'écran jointes au compte rendu.

## 7. Tâches (dans l'ordre)

1. **Canvas + owners** : `canvas.go` + tests. Mutex unique ; `Set(x, y, c, uid) → (prev, ok)` ; `owners.bin` ; agrandissement ; PNG en cache pour l'image et pour les propriétaires.
2. **Store SQLite** : `store.go` + tests (schéma v1, users, sessions, pixel_events, compteurs, rejeu).
3. **Auth** : `auth.go` (Provider, fournisseur dev, sessions, cookie, logout, protection de `next`) + tests.
4. **Hub WebSocket** : `hub.go` + tests. Auth au handshake, délai, erreurs, diffusion non bloquante, stat, compatibilité avec l'ancien format.
5. **API** : `api.go` + tests (`/api/me`, `/api/pixel`, `/api/owners`, `/api/users`).
6. **main.go** : flags `-db`, `-owners`, `-cooldown`, `-devAuth` ; ServeMux ; sauvegarde périodique et à l'arrêt (SIGINT/SIGTERM) ; rejeu au démarrage ; `_filters.txt` + `/login`.
7. **Front, socle** : `bp.css`, `app.css`, thème, modules sans DOM (palette, view, owners) + tests node ; `glwindow.js` en module + méthodes caméra.
8. **Front, desktop** : barre en verre, palette latérale, barre d'outils, inspecteur au survol (300 ms, `I` épingle), raccourcis, délai.
9. **Front, mobile** : réticule, loupe 7×7, Poser, pincer, deux doigts, appui long, tiroir palette, tiroir inspecteur, partage.
10. **Chargement, invité, connexion perdue** : écran de chargement avec le vrai %, bandeau invité, reconnexion 3 essais + pixels en attente, resynchronisation.
11. **Page /login** : « Mets ton chapeau » + choix des faux comptes en mode dev.
12. **Accueil /home** (bonus si le temps le permet) : m-accueil / d-accueil.
13. **Vérifications** : tests Go + node, scénarios navigateur, captures, relecture, commits.

## 8. Points à trancher par tiago (je choisis un défaut en attendant)

1. **Transition en prod** : tant que JustBetter n'est pas branché, un déploiement de la phase 1 mettrait tout le monde en lecture seule. → Défaut : pas de déploiement ; le mode dev n'est destiné qu'au local.
2. **Délai entre deux pixels** : 5 s (valeur des maquettes), réglable avec `-cooldown`.
3. **Pixel blanc jamais retouché depuis les comptes** : il appartient aussi à « l'œuvre d'origine » (`owner = 0`). → Défaut : l'inspecteur affiche « Pixel libre » s'il est blanc, et « Œuvre d'origine » pour toute autre couleur.
4. **Onglets mobiles** : Musée, Activité et Profil n'existent pas encore. → Défaut : la barre du bas n'affiche que ce qui existe (Canvas, Timelapse, Accueil, puis Connexion ou Moi). « Moi » ouvre un tiroir avec le pseudo, le thème et la déconnexion ; le profil complet arrive en phase 2.
5. **Glisser un doigt quand on est connecté** : le HANDOFF dit « ajuster la visée », pas « déplacer ». Pour se déplacer, il faut donc deux doigts. → Implémenté comme la maquette ; les invités se déplacent à un doigt, puisqu'ils n'ont pas de visée.
