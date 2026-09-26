# BetterPlace — HANDOFF du redesign

Ce document sert à développer le redesign conçu sur le canvas Design **« BetterPlace — Redesign »** (lien : https://claude.ai/artifact/5b72xSs2oFvWF5sV2Q4TJ2, privé tant qu'il n'est pas partagé). Il liste les écrans, les composants, les interactions et les données ou API dont chaque écran a besoin.

Tout le contenu de `/design` est une **référence**. Aucun fichier existant du site n'est modifié.

```
design/
├── HANDOFF.md                 ← ce fichier
├── README.md
├── canvas/                    ← sources exactes des 62 écrans (.dc.html) + canvas.json + bp.css
├── assets/                    ← mascotte, badges, avatars d'exemple, bannières, images tirées des vraies archives
│   └── blobs.json             ← correspondance /_blob/<id> des maquettes → fichier local
├── outils/analyse_sauvegardes.py   ← analyse des sauvegardes (stats, dates, badges) — testé
└── donnees/                   ← statistiques réelles calculées sur les archives + index des captures
```

> Les maquettes chargent leurs images via `/_blob/<id>` (stockage du canvas Design). Pour les ouvrir hors du canvas, remplace ces URL grâce à `assets/blobs.json`.

---

## 1. Ce qui ne change pas (contraintes)

| Élément | Aujourd'hui | À garder |
|---|---|---|
| URLs | `/home` (holdup.html), `/ace` (desktop.html), `/timelapse` (timelapse.html), `*` → `./home` via `web/root/_filters.txt` | Les trois URL restent valides. `/ace` sans paramètre ouvre la vue d'ensemble comme avant. |
| Canvas | Une PNG servie par `GET /place.png`, rendue en WebGL (`glwindow.js`) | Même rendu WebGL, même image. **Attention : le canvas de prod fait 1024 × 768** (il a grandi : 256×256 le 10/09/22 → 512×256 → 512×512 → 1024×512 le 21/09/22 → 1024×768 le 25/12/22), pas 1024 × 1024. |
| Temps réel | WebSocket `/ws`, message `{"x","y","color":{"R","G","B","A"}}`, `ping`/`pong` toutes les 30 s | Même format de base (voir §6 pour les champs ajoutés, tous optionnels). |
| Palette | 30 couleurs nommées dans `main.js` + champ hex libre | Identique, **noms compris** (`noir`, `bleu evian`, `bleu sale de bain`, deux `rouge`…). Le champ hex libre reste. |
| Timelapse | `/timelapse.mp4`, `/archives/archive-AAAA-MM.zip`, mois sans activité barrés (« Il n'y a pas eu d'activité sur la toile ») | Mêmes fichiers et mêmes URL de téléchargement. |
| Sauvegardes | `backup.sh` (cron chaque minute) → `bak/AAAA-MM/AAAA-MM-JJ_HH-MM-SS.png` si le canvas a changé ; `timelapse.sh` → zips + mp4 | Inchangées : elles alimentent l'analyse des revendications. |
| Contrôles desktop | Clic gauche déplacer, clic molette pipette, clic droit poser, `+`/`-`/PageUp/PageDown/pavé num. zoom, molette zoom, grille | Tous conservés, on ajoute seulement des raccourcis (§5). |
| Identité | Crédit « Fait avec ❤ par Evan et Tiago », mascotte `farwest.png`, logo cube vert | Présents sur l'accueil, la maintenance et le design system. |

## 2. Direction et ton

- **Le canvas est la star.** L'interface flotte en verre dépoli (`.glass`), repliable, jamais pleine largeur.
- **Sombre par défaut** (`data-theme="sombre"`), mode clair `data-theme="clair"`. Suivre `prefers-color-scheme` au premier chargement puis mémoriser le choix.
- **Pixel en clin d'œil** : titres en *Pixelify Sans*, UI en *Instrument Sans*, chiffres et coordonnées en *JetBrains Mono*. Coins crénelés (`.btn-px`) réservés aux actions qui marquent le canvas : Poser, Revendiquer, Entrer, Exposer.
- **Western par la mascotte**, le cuir doré (`--gold`) et le ton des textes, pas par des décors.
- **Entre amis, pas de bataille.** On peut dessiner par-dessus les autres et faire des blagues ; le but est le plus beau place possible. Vocabulaire : « retouché », « restaurer », « à départager ». À proscrire : vandale, attaque, défendre, guerre, contester.
- Interface **100 % en français**.

## 3. Design system (tokens)

La source de vérité est `canvas/bp.css` (variables CSS + classes de composants). Résumé :

| Token | Sombre | Clair | Usage |
|---|---|---|---|
| `--bg` / `--bg-2` | `#11100E` / `#161512` | `#F3EEE3` / `#EDE6D8` | fond |
| `--surface` → `--surface-3` | `#1C1B18` `#24221E` `#2E2B26` | `#FFFCF6` `#F6F0E4` `#EBE3D3` | cartes, champs |
| `--line` / `--line-2` | `#36332D` / `#4B463E` | `#DDD3C0` / `#C8BCA5` | bordures |
| `--text` / `--text-2` / `--text-3` | `#F3EFE6` `#BFB7A8` `#979083` | `#1C1A16` `#544C40` `#6C6457` | texte (3 niveaux, tous ≥ 4,5:1) |
| `--accent` (+ `--accent-text`) | `#3EE06C` (`#62F08E`) | `#2FD064` (`#13793A`) | vert JustBetter du logo |
| `--gold` (+ `--gold-text`) | `#F0B75A` (`#F5C77A`) | `#C98A2B` (`#8E5A0C`) | Pionnier, revendication, cuir |
| `--danger` `--warn` `--info` | `#FF6B6B` `#F5A524` `#5EB3FF` | `#D63B3B` `#C77A00` `#1F6FD1` | statuts (toujours avec texte ou icône) |
| `--void` | `#0C0B0A` + trame de points | `#E4DCCB` | autour du canvas |

- Espacements base 4 (4, 8, 12, 16, 24, 32, 48, 64). Rayons 4 / 8 / 12 / 16 / 24 + crénelé.
- Cibles tactiles ≥ 44 px ; 36 px autorisé en desktop dense.
- Icônes : trait 2 px, extrémités carrées, grille 24 (voir « Fondations »).

### Composants (classes dans `bp.css`, démontrés sur « Composants · 1 » et « Composants · 2 »)

| Composant | Classes | Notes |
|---|---|---|
| Bouton | `.btn` + `.btn-primary` `.btn-gold` `.btn-danger` `.btn-ghost` `.btn-outline-danger` `.btn-sm` `.btn-lg` `.btn-icon` `.btn-round` `.btn-px` `.btn-block` | « Poser » désactivé = compte à rebours du délai |
| Pastilles / statuts | `.chip` `.chip-on` `.pill` + `.st-attente` `.st-valide` `.st-refuse` `.st-conflit` `.st-info` `.st-neutre` | `.st-conflit` = « À départager » |
| Coordonnées, touches, compteur | `.coord` `.kbd` `.count` `.live` | |
| Avatars, badges | `.av .av-20…96` `.av-ring` `.av-stack` `.badge-ico` `.badge-tile` `.badge-lock` | sprites pixel, `image-rendering: pixelated` |
| Palette | `.swatch` `.swatch-sel` `.swatch-s` `.swatch-xs` | nom de la couleur en `title` + `aria-label` |
| Champs | `.field` `.label` `.hint` `.input` `.input-mono` `.textarea` `.search` `.toggle` `.toggle-on` `.seg` | |
| Navigation | `.nav-bottom` `.tab` `.tab-on` (mobile) · barre en verre (desktop) · `.sidenav` (admin) | |
| Tiroir / modale / toast / bannière | `.sheet` `.grab` `.scrim` `.modal` `.toast` `.banner-gold/-info/-danger` | tiroir : aperçu ≈180 px, moitié, plein écran |
| Tableaux admin, stats | `.table` `.kpi` `.kpi-v` `.delta-up/-down` `.bar-track` `.bar-fill` | |
| Canvas | `.reticle` `.loupe` `.sel-rect` `.zone-box` `.handle` | |
| Musée | `.cadre` `.cadre-or` `.passe` `.cartel` `.spot` `.rideau` `.eq` `.part` `.bulle` `.onde` `.bonus` | |
| Chargement | `.pbar` `.pfill` `.cooldown` | |

### Micro-animations

Toutes en `steps()` pour garder un rendu pixel, coupées par `prefers-reduced-motion` (règle en fin de `bp.css`).

| Moment | Classe | Détail |
|---|---|---|
| Pose de pixel | `.anim-pop` + `.anim-ring` | 420 ms steps(6) + onde carrée 600 ms |
| Revendication validée | `.stamp.anim-stamp` | tampon 500 ms steps(8) |
| Nouveau badge | `.anim-shine` + `.anim-bob` | reflet 2,2 s steps(12) |
| Fil d'activité | `.anim-feed` | entrée 350 ms steps(5) |
| Musée | `.anim-cam` (vol de caméra), `.cam` (transition), `.rideau-ouvert`, `.part-monte/-tombe/-scint`, `.anim-zoomlent` | |

## 4. Écrans

Pages du canvas : **Design system · Joueur mobile · Joueur desktop · Admin · États**. Chaque écran est conçu en 375 px puis étendu au desktop (1440 px). Les chiffres « en direct » des maquettes sont des exemples ; tout ce qui est marqué **« réel »** vient des vraies archives (voir `donnees/`).

### 4.1 Joueur

| Écran | Route proposée | Artboards | Contenu et interactions | Données / API |
|---|---|---|---|---|
| Accueil | `/home` | `m-accueil`, `d-accueil` | Aperçu du canvas actuel, nb de connectés, Entrer / Timelapse / Musée, avertissement « accès libre, entre amis », contrôles, lien Revendiquer, crédit | `GET /stat` (existe), `GET /place.png` |
| Chargement | `/ace` (au démarrage) | `m-chargement`, `e-chargement-d` | Barre pixel avec % réel (Content-Length, comme `place.js`), étapes : serveur → image → « qui a posé quoi » (chargé après) | `GET /place.png`, `GET /api/owners` (différé) |
| Canvas invité | `/ace` | `m-canvas-invite`, `d-canvas-invite` | Lecture seule, bandeau « Tu regardes en invité · Entrer », inspecteur au toucher/survol | WS `/ws` (lecture), `GET /api/pixel` |
| Connexion | `/login` ou modale | `m-connexion`, `d-connexion` | « Se connecter avec JustBetter », continuer en invité, ce que le compte apporte | OAuth/OIDC JustBetter (à définir) |
| Canvas connecté | `/ace?x=&y=&z=` | `m-canvas`, `d-canvas` | **Mobile** : toucher = viser (réticule + loupe 7×7), glisser un doigt = ajuster la visée, bouton **Poser** pour confirmer, pincer = zoom, deux doigts = déplacer, palette récente + tiroir complet (30 couleurs + hex + pipette), délai visible sur le bouton. **Desktop** : contrôles historiques, palette latérale, inspecteur au survol, grille `G` | WS `/ws` (écriture authentifiée), `GET /api/me` |
| Inspecteur de pixel | (panneau) | `m-inspecteur`, `m-inspecteur-origine`, carte dans `d-canvas` | Couleur + nom, coordonnées, auteur (avatar, lien profil), date, œuvre liée, 3 derniers changements, Prendre la couleur, Partager. Pixel d'avant la migration : « Œuvre d'origine » + auteurs si revendiquée | `GET /api/pixel?x&y` |
| Partage de position | (tiroir) | `m-partage` | Aperçu, lien `/ace?x=470&y=300&z=6`, inclure le zoom, copier, partager | — (client) |
| Alerte « œuvre retouchée » | (tiroir + notif) | `m-alerte` | Qui, combien de pixels, calque de l'original, **Retoucher** (pipette qui suit l'original) ou **Je garde, c'est drôle** | WS `alert`, `GET /api/oeuvres/:id/diff` |
| Musée | `/musee` | `m-musee`, `d-musee` | Œuvre à l'affiche avec vol de caméra, salles (coups de cœur, pionnières, paysages…), grille de cadres, musées des joueurs, « Exposer » | `GET /api/oeuvres?salle=`, `GET /api/musee/affiche` |
| Visite guidée | `/musee/visite` | `m-visite`, `d-visite` | Plein écran, caméra qui vole d'œuvre en œuvre (8 s), segments façon story, pause, précédent/suivant, coup de cœur | `GET /api/musee/visite` |
| Page œuvre | `/oeuvre/:id` | `m-oeuvre`, `d-oeuvre` | Cartel, badges, contributeurs, **avant/après** tiré des sauvegardes (curseur), histoire datée, voir sur le canvas | `GET /api/oeuvres/:id` (+ images avant/après) |
| Exposer | `/musee/exposer` | `m-exposer` | Choix de l'œuvre, cadrage (marge), cadre (bois/or/sans), salle, mot de l'artiste, rejouer la construction | `POST /api/oeuvres/:id/exposition` |
| Profil public | `/u/:pseudo` | `m-profil`, `d-profil`, `e-vides-m` | Bannière, avatar (anneau couleur d'accent), pseudo, bio, couleur préférée, rang, badges, stats (pixels posés, encore visibles, arrivée, rang), œuvres, **carte des contributions** (canvas assombri + ses pixels), lien vers son musée | `GET /api/users/:pseudo`, `GET /api/users/:pseudo/contributions.png` |
| Édition du profil | `/moi/profil` | `m-profil-edition`, `d-profil-edition` | **Aperçu en direct** : avatar (import / pixel-avatar), bannière, pseudo (→ URL), bio 160 car., couleur d'accent, couleur préférée (palette), badges épinglés (3), options | `PATCH /api/me/profile`, `POST /api/me/avatar` |
| Revendiquer | `/revendiquer` | `m-claim-zone` → `m-claim-form`, `d-claim` | Outils **rectangle / lasso / baguette** (couleurs contiguës, tolérance), encart « d'après les sauvegardes » (dates, pixels, autres demandes), titre, message/preuve, co-auteurs, répartition des pixels pionniers, badges attendus | `POST /api/claims/analyse` (aperçu), `POST /api/claims` |
| Fil des revendications | `/revendications` | `m-revendications`, colonne de `d-activite` | En cours / validées / refusées, « Je l'ai vue la faire » (+1), « Émettre un doute » | `GET /api/claims`, `POST /api/claims/:id/votes` |
| Activité | `/activite` | `m-activite`, `d-activite` | Fil discret filtrable (pixels regroupés, œuvres, revendications, musées), chaque ligne recentre le canvas ; carte « où ça dessine » ; alertes | WS `activity`, `GET /api/activity?since=` |
| Classement | `/classement` | `m-classement`, `d-classement` | Jour / semaine / tout le temps, podium, joueurs et œuvres coup de cœur, « toi : #4 » | `GET /api/leaderboard?period=` |
| Timelapse & archives | `/timelapse` | `m-timelapse`, `d-timelapse` | **141 vraies captures quotidiennes** (10/09/2022 → 28/03/2024), lecture ×1/×4/×10, barres d'activité par jour, le canvas qui grandit, jours les plus actifs, vidéo, archives mensuelles (mois vides barrés) | `GET /api/snapshots` (voir `donnees/archives-index.json`), `/timelapse.mp4`, `/archives/*.zip` |

### 4.2 Bonus · Musées perso (à faire après le socle)

| Écran | Route | Artboards | Contenu |
|---|---|---|---|
| Musées des joueurs | `/musees` | `m-musees` | Annuaire (populaires, nouveaux, amis), recherche, « Au hasard », aperçu de l'ambiance de chaque musée |
| Visite d'un musée | `/u/:pseudo/musee` | `m-musee-perso`, `d-musee-perso` | Rideau d'entrée, mot d'accueil, musique (coupable), particules, guide pixel qui commente, commentaire vocal de l'artiste, cartels, livre d'or, coup de cœur |
| Mon musée (éditeur) | `/moi/musee` | `m-musee-editeur`, `d-musee-editeur` | Thème, couleur des murs (palette du canvas), éclairage, particules, cadres, musique + volume + sons, animation d'entrée, parcours libre/guidé, guide et sa phrase, salles, œuvres, mot d'accueil, visibilité, livre d'or. Desktop : réglages / aperçu en direct / œuvre sélectionnée |
| Personnaliser une œuvre | `/moi/musee/oeuvre/:id` | `m-musee-oeuvre-perso` | Animation (aucune, **rejouer sa construction** depuis les sauvegardes, zoom lent, scintille), texte du cartel, commentaire vocal (30 s), son au passage, taille, cadre, couleur du spot, œuvre vedette |

Sons importés et commentaires vocaux passent par la modération (§4.3). Données : §7.

### 4.3 Admin (desktop d'abord, utilisable sur mobile)

| Écran | Route | Artboards | Contenu et interactions | API |
|---|---|---|---|---|
| Tableau de bord | `/admin` | `a-dashboard`, `am-dashboard` | KPI (connectés/slots, pixels 1 h/24 h, joueurs actifs, nouveaux comptes, à valider), pixels par heure (survol = infobulle), top contributeurs, **activité historique réelle** par mois, couleurs les plus utilisées, **heatmap réelle**, zones les plus retouchées, jours les plus actifs | `GET /api/admin/stats?period=`, `GET /api/admin/heatmap.png?period=` |
| Revendications | `/admin/revendications` | `a-claims`, `am-claims` | File à traiter / validées / refusées ; pour chaque demande : aperçu de zone (+ zone concurrente en pointillés), **analyse des sauvegardes**, **mini-timelapse réel**, votes et doutes, cas « à départager » côte à côte, **badges proposés (décochables)**, actions **Valider · Fusionner en co-auteurs · Attribuer à un autre compte · Refuser** (motif), annulation | `GET /api/admin/claims`, `GET /api/claims/:id/analysis`, `POST /api/admin/claims/:id/decision` |
| Utilisateurs | `/admin/utilisateurs` | `a-utilisateurs`, `am-utilisateurs` | Recherche, filtres, tableau ; fiche : stats, carte de ses pixels, note ; suspendre (1 h / 24 h / 7 j), bannir, réinitialiser le profil | `GET /api/admin/users?q=`, `POST /api/admin/users/:id/{suspend,ban,reset}` |
| Outil zone | `/admin/zone` | `a-zone` | Sélection (rectangle, lasso, baguette, **tous les pixels d'un joueur**) ; **Restaurer** depuis une sauvegarde (liste datée, avant/après), **Effacer**, **Attribuer** directement (sans demande, badges calculés) ; impact chiffré, motif, confirmation | `GET /api/admin/backups?zone=`, `POST /api/admin/zones/{restore,erase,assign}` |
| Modération | `/admin/moderation` | `a-moderation` | Avatars (floutés par défaut), bios, noms d'œuvres et de musées, sons ; Garder / Masquer (valeur par défaut) / Écrire ; filtre automatique | `GET /api/admin/reports`, `POST /api/admin/reports/:id` |
| Journal | `/admin/journal` | `a-journal` | Toutes les actions admin, filtres, **annuler / rétablir** (30 jours), export CSV | `GET /api/admin/journal`, `POST /api/admin/journal/:id/undo` |
| Réglages | `/admin/reglages` | `a-reglages` | Délai entre deux pixels (+ délai nouveaux comptes), mode normal / lecture seule / maintenance (+ message, fin prévue), bannière d'annonce (texte, style, dates, aperçu), connexions max (`-count`), écriture de place.png (`-saveInterval`), état des sauvegardes, options des musées | `GET/PUT /api/admin/settings` |

### 4.4 États

| État | Artboards | Comportement |
|---|---|---|
| Chargement | `m-chargement`, `e-chargement-d` | % réel, étapes, astuce |
| Connexion perdue | `e-perdue-m`, `e-perdue-d` | Bandeau non bloquant, 3 essais automatiques (remplace l'`alert()` actuel), canvas figé en gris, **pixels en attente** envoyés au retour, puis modale « Réessayer / Recharger » |
| Compte suspendu / banni | `e-banni-m` | Date de fin, motif, regarder le canvas en lecture seule, écrire à l'équipe (`[ADRESSE DE CONTACT]` à fournir) |
| Maintenance | `e-maintenance-m`, `e-maintenance-d` | Mascotte, heure de retour, nouvel essai automatique, liens timelapse et musée restent ouverts |
| Lecture seule + annonce | `e-lecture-seule-m` | Bannières, bouton Poser grisé qui explique pourquoi |
| Vides | `e-vides-m`, `e-vides-d` | Un message utile et une action : musée, activité, classement du jour, admin « tout est traité », recherche sans résultat, livre d'or, musée à créer, alertes |

## 5. Contrôles et raccourcis

- **Desktop** (inchangés) : clic gauche + glisser = déplacer ; clic molette = pipette ; clic droit = poser (Ctrl + clic droit = pipette) ; molette, `+`/`-`, PageUp/PageDown, pavé numérique = zoom. **Ajouts** : survol 300 ms = carte d'inspection ; `I` = épingler l'inspecteur ; `G` = grille ; `L` = copier le lien de la vue ; `Échap` = fermer. Outils de sélection : `R` rectangle, `L` lasso, `B` baguette (dans l'écran Revendiquer, `L` n'a pas besoin du lien).
- **Mobile** : toucher = viser (réticule + loupe) ; glisser un doigt = affiner la visée ; **Poser** = confirmer ; pincer = zoom ; deux doigts = déplacer ; appui long 500 ms = inspecteur. Invité : un toucher ouvre directement l'inspecteur.
- **Délai** : appliqué côté serveur, affiché sur le bouton Poser (`0:04`) et par `.cooldown`.
- **Lien de position** : `/ace?x=<int>&y=<int>&z=<zoom>` ; lu au chargement, mis à jour sans recharger (`history.replaceState`).

## 6. Temps réel (WebSocket `/ws`)

Compatible avec le client actuel : les champs ajoutés sont optionnels.

```jsonc
// client → serveur (inchangé)
{"x": 470, "y": 300, "color": {"R": 94, "G": 179, "B": 255, "A": 255}}
"ping"

// serveur → clients : pixel diffusé (+ auteur et heure)
{"x": 470, "y": 300, "color": {...}, "u": 42, "t": 1790416853000}

// serveur → client : refus
{"type": "error", "err": "cooldown", "retry": 3.2}      // aussi "auth", "readonly", "suspended", "maintenance"

// serveur → clients : événements
{"type": "stat", "online": 27, "slots": 64}
{"type": "mode", "mode": "readonly", "msg": "…", "until": "…"}
{"type": "announce", "text": "…", "style": "event", "until": "…"}
{"type": "activity", "kind": "pixels|oeuvre|claim|badge|museum", "who": 42, "x": 470, "y": 352, "n": 12, "ref": "…"}
{"type": "alert", "kind": "retouche", "oeuvre": 7, "by": 13, "count": 136}   // seulement à l'auteur
```

- L'authentification du WebSocket se fait au handshake (cookie de session). Sans session : socket en lecture seule, toute écriture reçoit `auth`.
- Les pixels « groupés » de l'activité sont agrégés côté serveur (même joueur, même zone de 32 px, < 60 s).

## 7. Données

### 7.1 Propriété des pixels

- En mémoire, à côté de l'image : `owners []uint32` (W × H), l'index du joueur qui a posé le pixel visible, `0` = pixel d'avant la migration (« œuvre d'origine »). Sauvegardé avec `place.png` (`owners.bin`, même intervalle).
- Journal append-only `pixel_events(x, y, color, user_id, ts)` : historique par pixel, stats, classement, heatmap.
- Compteurs incrémentaux par joueur : pixels posés, pixels encore visibles (+1 à la pose, −1 pour l'ancien propriétaire).
- `GET /api/owners` : la carte des propriétaires en PNG (RGB = index 24 bits), chargée après l'image, pour un inspecteur instantané au survol ; `GET /api/users?ids=` pour les pseudos/avatars.

### 7.2 Schéma proposé (SQL)

```
users(id, justbetter_id, pseudo, slug, avatar_url, banner, bio, accent, couleur_pref, role[joueur|admin], statut, suspendu_jusqua, cree_le)
pixel_events(id, x, y, color, user_id, ts)
oeuvres(id, titre, description, masque(rect|polygone|bitmap), origine[avant_migration|compte], creee_le, statut)
oeuvre_auteurs(oeuvre_id, user_id, role[auteur|co_auteur], pixels_pionniers)
claims(id, oeuvre_id?, demandeur_id, masque, titre, message, co_auteurs[], repartition, statut, analyse_json, cree_le)
claim_votes(claim_id, user_id, type[confirme|doute], commentaire, ts)
badges(id, nom, regle, sprite) ; user_badges(user_id, badge_id, source[auto|claim|admin], ts)
likes(user_id, oeuvre_id) ; expositions(oeuvre_id, salle, cadrage, cadre, mot, ts)
alertes(user_id, type, payload, lu)
reports(id, type[avatar|bio|nom|son], cible, auteur_id, raison, statut)
admin_actions(id, admin_id, type, cible, avant_json, apres_json, motif, ts, annulee)
settings(cle, valeur)
-- bonus musées
musees(user_id, nom, theme, couleur_mur, eclairage, particules, cadre, musique, volume, entree, parcours, guide, phrase_guide, accueil, visibilite, livre_or)
musee_salles(id, user_id, titre, ordre) ; musee_oeuvres(salle_id, oeuvre_id, ordre, animation, son, voix_url, texte, taille, cadre, spot, vedette)
livre_or(musee_user_id, auteur_id, texte, ts) ; sons(id, user_id, url, duree, statut)
```

### 7.3 Badges

| Badge | Règle | Calcul |
|---|---|---|
| Premier pixel | 1er pixel posé | live |
| Millier | 1 000 pixels posés (pixels pionniers compris) | live |
| Restaurateur | 50 pixels restaurés à l'identique d'une version précédente (ses œuvres ou celles des autres) | live |
| Noctambule | 100 pixels posés entre 2 h et 5 h | live · pour une revendication : seulement avec les sauvegardes minute par minute |
| Top 10 | top 10 d'une semaine | hebdo |
| Témoin | 10 confirmations sur des revendications validées | à la validation |
| **Pionnier** | œuvre d'avant la migration, revendiquée et validée | revendication |
| **Vétéran 2022** | œuvre apparue en 2022 | revendication (sauvegardes) |
| **Bâtisseur** | œuvre de plus de 1 000 pixels posés | revendication (sauvegardes) |
| **Indémodable** | œuvre intacte à 90 % pendant plus d'un an | revendication (sauvegardes) |

Sprites 16 × 16 dans `assets/badges/` (dessinés avec la palette du canvas).

## 8. Analyse des sauvegardes (revendications, badges, stats)

`outils/analyse_sauvegardes.py` (Python 3.9+, numpy, Pillow) lit `bak/` ou les zips `archives/` et :

1. `indexer` rejoue toutes les sauvegardes et stocke chaque changement de pixel (instant, position, couleur) dans un `.npz`. Les sauvegardes plus petites (le canvas a grandi) sont placées en haut à gauche.
2. `zone --rect x,y,l,h` (ou `--masque`) : date d'apparition (10 % de l'état final), fin de construction (90 %), sessions de dessin (pause > 45 min), pixels par heure, pixels posés estimés, retouches puis restaurations, stabilité sur 12 mois, **badges proposés**, **répartition des pixels pionniers** (`--auteurs tiago:2,evan`).
3. `conflits claims.json` : recouvrement entre demandes.
4. `stats` : pixels par jour et par heure, couleurs, heatmaps (activité, retouches), zones les plus retouchées.
5. `apercu` : mini-timelapse d'une zone (GIF + bande PNG) pour la file admin.

Intégration conseillée : `indexer` chaque nuit après `timelapse.sh` ; l'API appelle `zone` (ou sa réimplémentation en Go) quand une demande est créée et stocke le résultat dans `claims.analyse_json`.

**Déjà lancé sur les vraies archives** (141 captures quotidiennes, 47 analysées pour le design) : résultats dans `donnees/stats-reelles.json` et dans les écrans admin marqués « réel ». Avec des captures quotidiennes, une « session » = un jour ; avec `bak/` minute par minute, l'analyse devient précise à la minute (et débloque Noctambule).

## 9. Ordre de développement suggéré

1. **Socle** : comptes JustBetter, sessions, WS authentifié + délai côté serveur, `owners`, `pixel_events`, inspecteur, invité en lecture seule, nouveau canvas mobile (viser / loupe / Poser), lien de position.
2. **Profils** : `/u/:pseudo`, édition, stats, carte des contributions, badges live.
3. **Revendications** : écran Revendiquer, fil + votes, analyse des sauvegardes, file admin, badges pionniers.
4. **Communauté** : activité, alertes de retouche, classement, musée public, pages œuvre, timelapse modernisé.
5. **Admin complet** : tableau de bord, utilisateurs, outil zone, modération, journal, réglages, états.
6. **Bonus** : musées perso.

## 10. Accessibilité

- Vrais `<button>`, `<a href>`, `<input>` + `<label>` ; `aria-label` sur les boutons-icônes (palette : nom français de la couleur).
- Contraste texte ≥ 4,5:1 dans les deux thèmes ; statuts toujours doublés d'un libellé.
- Cibles ≥ 44 px sur mobile ; le canvas reste utilisable au clavier (flèches = déplacer la visée, Entrée = poser).
- `prefers-reduced-motion` : animations coupées, seul l'état final reste.

## 11. Contenu fictif vs réel

- **Réel** (tiré des archives) : la capture du canvas (28/03/2024), les 141 dates du timelapse, les mini-timelapses, avant/après, analyses de zones, activité par mois, couleurs, heatmap, zones les plus retouchées, jours les plus actifs.
- **Fictif** : pseudos (Brindille, Kaelen, Zozo42, Mamie Pixel…), noms d'œuvres, avatars générés, chiffres « en direct » marqués « illustration », l'exemple de spam de `bot_404` (image fabriquée pour montrer l'outil zone).
