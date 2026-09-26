# Démarrer le développement avec Claude Code

Ce fichier donne l'ordre des étapes et des prompts prêts à coller dans Claude Code, phase par phase. La référence complète reste `design/HANDOFF.md`.

## Lancer le site en local

Prérequis : Go (le Dockerfile utilise Go 1.21).

```bash
go run ./cmd/place -root ./web/root -load design/assets/reel/canvas-2024-03-28.png -save ./place.png
```

Puis ouvrir http://localhost:8080/home. `place.png` est ignoré par git ; la capture de `design/` n'est jamais modifiée (lecture seule, l'état courant est écrit dans `./place.png`).

## Règles à rappeler à Claude Code

- Travailler sur la branche `redesign` (ou des branches filles), jamais sur `master` qui fait tourner la prod.
- Garder `/home`, `/ace`, `/timelapse`, `/place.png`, `/ws`, `/stat`, `/timelapse.mp4`, `/archives/*.zip`.
- Garder la palette et ses noms, le rendu WebGL, le crédit « Fait avec ❤ par Evan et Tiago », la mascotte.
- WebSocket rétro-compatible : les nouveaux champs sont optionnels (HANDOFF §6).
- Interface entièrement en français, ton « entre amis » (pas de vocabulaire de bataille).
- Comptes : l'intégration JustBetter viendra plus tard. En attendant, une interface d'authentification avec un fournisseur « dev » (faux comptes) derrière une option de lancement.

## Phase 1 · Socle (prompt de départ)

```
Tu travailles sur BetterPlace, un canvas collaboratif façon r/place : backend Go + WebSocket (server.go, cmd/place), client WebGL dans web/root. Nous sommes sur la branche `redesign` ; ne touche jamais à master.

1. Lis en entier design/HANDOFF.md, puis design/DEMARRER-AVEC-CLAUDE-CODE.md, design/canvas/bp.css et les maquettes design/canvas/m-canvas.dc.html, m-canvas-invite.dc.html, m-inspecteur.dc.html, m-inspecteur-origine.dc.html, m-connexion.dc.html, m-chargement.dc.html, d-canvas.dc.html, d-canvas-invite.dc.html (ce sont des maquettes HTML : prends-y la structure, les textes et les styles, pas le runtime).
2. Objectif de la phase 1 (HANDOFF §9) : comptes (interface d'auth + fournisseur « dev » en attendant JustBetter), sessions, WebSocket authentifié avec délai entre deux pixels côté serveur, invités en lecture seule, propriété des pixels (owners en mémoire + owners.bin + journal pixel_events), GET /api/pixel, inspecteur de pixel (survol desktop, appui long mobile), nouveau canvas mobile (viser, loupe, bouton Poser, pincer, deux doigts), lien de position /ace?x=&y=&z=, reprise du design system bp.css.
3. Contraintes : garde les URL existantes, la palette et ses noms, le rendu WebGL (glwindow.js), le format WebSocket actuel (champs ajoutés optionnels), le crédit et la mascotte. Interface en français.
4. Avant d'écrire du code, propose-moi un plan : structure des dossiers du front, choix techniques (je veux rester simple et léger), stockage (SQLite conseillé), schéma des tables de la phase 1, liste des endpoints et messages WebSocket, et comment tu vas tester. Attends ma validation.
```

Astuce : dans Claude Code, `Maj+Tab` fait passer en mode plan ; Claude y lit et propose sans modifier de fichiers.

## Phases suivantes (à lancer une par une)

**Phase 2 · Profils**
```
Phase 2 du redesign (HANDOFF §4.1 et §9) : profil public /u/:pseudo et édition avec aperçu en direct, stats (pixels posés, encore visibles, arrivée, rang), carte des contributions (PNG généré côté serveur), badges live (Premier pixel, Millier, Restaurateur, Noctambule, Top 10). Maquettes : m-profil, d-profil, m-profil-edition, d-profil-edition, e-vides-m. Propose un plan puis attends.
```

**Phase 3 · Revendications**
```
Phase 3 : revendications d'œuvres (HANDOFF §4.1, §4.3, §7.3, §8). Écran Revendiquer (rectangle, lasso, baguette), fil public avec votes, analyse des sauvegardes branchée sur design/outils/analyse_sauvegardes.py (ou réécrite en Go, compare les deux options), file admin avec badges proposés, décisions Valider / Fusionner / Attribuer / Refuser, badge Pionnier. Maquettes : m-claim-zone, m-claim-form, d-claim, m-revendications, d-activite, a-claims, am-claims. Propose un plan puis attends.
```

**Phase 4 · Communauté** : activité en direct, alertes « œuvre retouchée », classement, musée public, pages œuvre (avant/après), timelapse modernisé (données : `design/donnees/archives-index.json`).

**Phase 5 · Admin complet et états** : tableau de bord, utilisateurs, outil zone, modération, journal annulable, réglages, écrans d'erreur et de maintenance.

**Phase 6 · Bonus musées perso** : annuaire, visite, éditeur, personnalisation par œuvre (HANDOFF §4.2).

## Outil d'analyse des sauvegardes

```bash
pip install numpy pillow
python3 design/outils/analyse_sauvegardes.py indexer bak/ --sortie index.npz
python3 design/outils/analyse_sauvegardes.py zone index.npz --rect 178,238,22,22 --auteurs kaelen --migration 2026-10-15
python3 design/outils/analyse_sauvegardes.py stats index.npz --dossier stats/
python3 design/outils/analyse_sauvegardes.py apercu index.npz --rect 172,232,34,34 --sortie creeper.gif
```
