# /design — redesign de BetterPlace

Design complet du redesign : design system, 62 écrans (mobile d'abord, puis desktop), admin, états, bonus « musées perso ».

- **Canvas Design** (interactif, à ouvrir dans le navigateur) : https://claude.ai/artifact/5b72xSs2oFvWF5sV2Q4TJ2 (privé tant qu'il n'est pas partagé depuis le menu Partager).
- **`HANDOFF.md`** : tout ce qu'il faut pour développer (écrans, composants, interactions, données et API).
- **`DEMARRER-AVEC-CLAUDE-CODE.md`** : lancer le site en local et prompts prêts à coller, phase par phase.
- **`canvas/`** : sources des écrans (`.dc.html`), mise en page (`canvas.json`) et design system (`bp.css`).
- **`assets/`** : mascotte, logo, badges 16×16, avatars d'exemple, bannières, images tirées des vraies archives. `blobs.json` fait le lien entre les `/_blob/…` des maquettes et ces fichiers.
- **`outils/analyse_sauvegardes.py`** : analyse des sauvegardes du canvas (dates, sessions, retouches, badges proposés, heatmaps, mini-timelapses).
- **`donnees/`** : index des 141 captures quotidiennes, statistiques réelles et analyses des zones utilisées dans les maquettes.

Aucun fichier existant du dépôt n'est modifié par ce dossier.
