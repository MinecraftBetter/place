#!/usr/bin/env python3
"""
analyse_sauvegardes.py — analyse la timelapse de BetterPlace à partir des sauvegardes PNG.

Les sauvegardes sont produites par backup.sh : une image du canvas (1024x768 aujourd'hui) par minute (seulement si le
canvas a changé), rangée dans bak/AAAA-MM/AAAA-MM-JJ_HH-MM-SS.png, puis zippée chaque mois dans
web/root/archives/archive-AAAA-MM.zip par timelapse.sh. Le script lit l'un ou l'autre.

Comme il n'existe aucun historique par pixel avant la migration, tout ce qu'on sait vient des
différences entre deux sauvegardes successives. Ce script :

  1. indexer  — parcourt toutes les sauvegardes dans l'ordre et enregistre chaque changement de pixel
                (instant, position, couleur) dans un index compact (.npz) ;
  2. zone     — pour une zone revendiquée : date d'apparition, période de construction, sessions de
                dessin, heures d'activité, pixels posés (estimés), retouches et restaurations, stabilité,
                badges proposés et répartition des « pixels pionniers » entre co-auteurs ;
  3. conflits — chevauchements entre plusieurs revendications (fichier JSON) ;
  4. stats    — statistiques globales : pixels par jour et par heure, couleurs les plus utilisées,
                zones les plus actives et les plus retouchées (heatmaps PNG) ;
  5. apercu   — mini-timelapse d'une zone (GIF + bande PNG) pour la file de claims admin.

Dépendances : Python 3.9+, numpy, Pillow.

Exemples :
  python3 analyse_sauvegardes.py indexer bak/ --sortie index.npz
  python3 analyse_sauvegardes.py indexer web/root/archives/*.zip --sortie index.npz
  python3 analyse_sauvegardes.py zone index.npz --rect 600,620,160,96 --auteurs brindille,nyx
  python3 analyse_sauvegardes.py zone index.npz --masque zone.png --migration 2026-10-01
  python3 analyse_sauvegardes.py conflits index.npz claims.json
  python3 analyse_sauvegardes.py stats index.npz --dossier stats/
  python3 analyse_sauvegardes.py apercu index.npz --rect 600,620,160,96 --sortie creeper.gif
"""
from __future__ import annotations

import argparse
import glob
import io
import json
import os
import re
import sys
import zipfile
import calendar
from datetime import datetime, timedelta

import numpy as np
from PIL import Image

W, H = 1024, 768   # taille actuelle du canvas en production ; recalculée depuis les sauvegardes
BLANC = 0xFFFFFF
NOM_RE = re.compile(r"(\d{4})-(\d{2})-(\d{2})_(\d{2})-(\d{2})-(\d{2})\.png$")

# Seuils (ajustables en ligne de commande)
PAUSE_SESSION_MIN = 45          # une pause plus longue démarre une nouvelle session de dessin
SEUIL_APPARITION = 0.10         # 10 % de l'état de référence présent = l'œuvre apparaît
SEUIL_TERMINEE = 0.90           # 90 % = l'œuvre est terminée
SEUIL_RETOUCHE = 0.90           # repasse sous 90 % après avoir été terminée = retouchée par quelqu'un
SEUIL_RESTAUREE = 0.95          # remonte à 95 % = restaurée
BADGES = {
    "pionnier":   "Œuvre d'avant la migration, revendiquée et validée",
    "veteran":    "Œuvre apparue dès la première saison (2022)",
    "batisseur":  "Œuvre de plus de 1 000 pixels posés",
    "noctambule": "Au moins 100 pixels posés entre 2 h et 5 h",
    "indemodable": "Œuvre restée intacte à 90 % pendant plus d'un an",
    "millier":    "1 000 pixels posés (pixels pionniers compris)",
}


# Les horodatages sont ceux des noms de fichiers (heure du serveur), traités comme une heure « murale »
# sans fuseau : on les convertit en secondes sans appliquer le fuseau de la machine qui lance le script.
def vers_epoch(d: datetime) -> int:
    return calendar.timegm(d.timetuple())


def depuis_epoch(s) -> datetime:
    return datetime(1970, 1, 1) + timedelta(seconds=int(s))


# --------------------------------------------------------------------------- lecture des sauvegardes
def _horodatage(nom: str) -> datetime | None:
    m = NOM_RE.search(nom)
    if not m:
        return None
    return datetime(*map(int, m.groups()))


def lister_sauvegardes(sources: list[str]):
    """Renvoie [(datetime, lecteur)] triés ; lecteur() -> bytes de la PNG."""
    items = []
    for src in sources:
        if os.path.isdir(src):
            for chemin in glob.glob(os.path.join(src, "**", "*.png"), recursive=True):
                if "corrupted" in chemin.split(os.sep):
                    continue
                t = _horodatage(os.path.basename(chemin))
                if t:
                    items.append((t, lambda c=chemin: open(c, "rb").read()))
        elif src.endswith(".zip"):
            z = zipfile.ZipFile(src)
            for nom in z.namelist():
                t = _horodatage(os.path.basename(nom))
                if t:
                    items.append((t, lambda z=z, n=nom: z.read(n)))
        elif src.endswith(".png"):
            t = _horodatage(os.path.basename(src))
            if t:
                items.append((t, lambda c=src: open(c, "rb").read()))
    items.sort(key=lambda it: it[0])
    return items


def decoder(octets: bytes, largeur=None, hauteur=None) -> np.ndarray | None:
    """Décode une sauvegarde en tableau HxW d'entiers 0xRRGGBB.
    Le canvas a grandi au fil du temps (256×256 → … → 1024×768), toujours vers la droite et le bas :
    une sauvegarde plus petite est donc placée en haut à gauche et complétée en blanc."""
    try:
        im = Image.open(io.BytesIO(octets)).convert("RGB")
    except Exception:
        return None  # sauvegarde corrompue : ignorée (timelapse.sh fait déjà le tri)
    a = np.asarray(im, dtype=np.uint32)
    a = (a[..., 0] << 16) | (a[..., 1] << 8) | a[..., 2]
    if largeur is None:
        return a
    out = np.full((hauteur, largeur), BLANC, dtype=np.uint32)
    h, w = min(a.shape[0], hauteur), min(a.shape[1], largeur)
    out[:h, :w] = a[:h, :w]
    return out


# --------------------------------------------------------------------------- 1. index
def cmd_indexer(args):
    items = lister_sauvegardes(args.sources)
    if not items:
        sys.exit("Aucune sauvegarde trouvée (noms attendus : AAAA-MM-JJ_HH-MM-SS.png).")
    global W, H
    derniere = decoder(items[-1][1]())
    if derniere is None:
        sys.exit("La dernière sauvegarde est illisible.")
    H, W = derniere.shape
    temps, ev_t, ev_pos, ev_col = [], [], [], []
    premiere, prec, ignorees = None, None, 0
    for i, (t, lire) in enumerate(items):
        img = decoder(lire(), W, H)
        if img is None:
            ignorees += 1
            continue
        k = len(temps)
        temps.append(vers_epoch(t))
        plat = img.ravel()
        if prec is None:
            premiere = plat.copy()
        else:
            diff = np.flatnonzero(plat != prec)
            if diff.size:
                ev_t.append(np.full(diff.size, k, dtype=np.int32))
                ev_pos.append(diff.astype(np.int32))
                ev_col.append(plat[diff].astype(np.uint32))
        prec = plat
        if args.verbeux and i % 500 == 0:
            print(f"{i}/{len(items)} {t:%Y-%m-%d %H:%M}", file=sys.stderr)
    cat = lambda xs, dt: np.concatenate(xs).astype(dt) if xs else np.zeros(0, dt)
    np.savez_compressed(
        args.sortie,
        temps=np.array(temps, dtype=np.int64),
        premiere=premiere.astype(np.uint32),
        derniere=prec.astype(np.uint32),
        ev_t=cat(ev_t, np.int32), ev_pos=cat(ev_pos, np.int32), ev_col=cat(ev_col, np.uint32),
        taille=np.array([W, H], dtype=np.int32),
    )
    n = sum(x.size for x in ev_t)
    print(json.dumps({
        "sauvegardes": len(temps), "ignorees": ignorees, "taille": f"{W}x{H}", "changements_de_pixel": int(n),
        "du": depuis_epoch(temps[0]).isoformat(), "au": depuis_epoch(temps[-1]).isoformat(),
        "index": args.sortie,
    }, ensure_ascii=False, indent=2))


def charger(chemin):
    global W, H
    d = np.load(chemin)
    idx = {k: d[k] for k in d.files}
    if "taille" in idx:
        W, H = int(idx["taille"][0]), int(idx["taille"][1])
    return idx


# --------------------------------------------------------------------------- zones
def masque_zone(args) -> np.ndarray:
    m = np.zeros((H, W), dtype=bool)
    if args.rect:
        x, y, w, h = map(int, args.rect.split(","))
        m[max(y, 0):min(y + h, H), max(x, 0):min(x + w, W)] = True
    elif args.masque:
        im = np.asarray(Image.open(args.masque).convert("L"))  # PNG de la taille du canvas, blanc = zone
        m = np.zeros((H, W), dtype=bool)
        m[:min(H, im.shape[0]), :min(W, im.shape[1])] = im[:H, :W] > 127
    else:
        sys.exit("Indiquer --rect x,y,l,h ou --masque fichier.png")
    return m


def _replay(idx, masque, ref=None):
    """Rejoue les changements dans la zone. Renvoie (t_evts, pos_evts, col_evts, taux_par_evt, ref)."""
    pos_zone = np.flatnonzero(masque.ravel())
    ref_plein = idx["derniere"] if ref is None else ref
    cible = ref_plein[pos_zone]
    # on ignore les pixels blancs de la référence : ce sont des trous, pas l'œuvre
    utiles = cible != BLANC
    n_utiles = max(int(utiles.sum()), 1)
    dans = np.isin(idx["ev_pos"], pos_zone)
    t, p, c = idx["ev_t"][dans], idx["ev_pos"][dans], idx["ev_col"][dans]
    ordre = np.argsort(t, kind="stable")
    t, p, c = t[ordre], p[ordre], c[ordre]
    # position absolue -> indice dans la zone
    rang = np.searchsorted(pos_zone, p)
    etat = idx["premiere"][pos_zone].copy()
    ok = (etat == cible) & utiles
    n_ok = int(ok.sum())
    taux = np.empty(t.size, dtype=np.float32)
    vers_cible = np.zeros(t.size, dtype=bool)
    for i in range(t.size):
        r = rang[i]
        avant = ok[r]
        apres = bool(utiles[r] and c[i] == cible[r])
        vers_cible[i] = apres and not avant
        if avant != apres:
            n_ok += 1 if apres else -1
            ok[r] = apres
        etat[r] = c[i]
        taux[i] = n_ok / n_utiles
    taux_initial = int(((idx["premiere"][pos_zone] == cible) & utiles).sum()) / n_utiles
    return t, p, c, taux, vers_cible, taux_initial, n_utiles


def _dt(idx, k):
    return depuis_epoch(idx["temps"][k])


def analyser_zone(idx, masque, auteurs, migration, decalage_h=0, ref=None, maintenant=None):
    t, p, c, taux, vers_cible, taux0, n_utiles = _replay(idx, masque, ref)
    temps = idx["temps"]
    maintenant = maintenant or depuis_epoch(temps[-1])
    res = {
        "zone": {"pixels": int(masque.sum()), "pixels_de_l_oeuvre": n_utiles},
        "changements_dans_la_zone": int(t.size),
    }
    if t.size == 0:
        res["remarque"] = "Aucun changement enregistré dans cette zone."
        return res

    # apparition / fin de construction
    def premier_passage(seuil, depuis=0):
        for i in range(depuis, t.size):
            if taux[i] >= seuil:
                return i
        return None
    i_app = 0 if taux0 >= SEUIL_APPARITION else premier_passage(SEUIL_APPARITION)
    i_fin = premier_passage(SEUIL_TERMINEE, i_app or 0) if i_app is not None else None
    app = _dt(idx, t[i_app]) if i_app is not None else None
    if taux0 >= SEUIL_APPARITION:
        app = _dt(idx, 0)
        res["remarque_apparition"] = "Déjà présente dans la première sauvegarde : date réelle antérieure."
    fin = _dt(idx, t[i_fin]) if i_fin is not None else None
    res["apparition_estimee"] = app.isoformat() if app else None
    res["terminee_vers"] = fin.isoformat() if fin else None
    res["etat_actuel_pct"] = round(float(taux[-1]) * 100, 1)

    # construction = changements qui rapprochent de l'état de référence, jusqu'à la fin de la session
    # pendant laquelle l'œuvre a atteint 90 % (les finitions de cette session comptent aussi)
    if i_fin is not None:
        j = i_fin
        while j + 1 < t.size and temps[t[j + 1]] - temps[t[j]] <= PAUSE_SESSION_MIN * 60:
            j += 1
        borne = t[j]
    else:
        borne = t[-1]
    constr = vers_cible & (t <= borne)
    tc = t[constr]
    res["pixels_poses_estimes"] = int(constr.sum())

    # sessions de dessin
    heures = [(_dt(idx, k) + timedelta(hours=decalage_h)) for k in tc]
    sessions = []
    for h in heures:
        if sessions and (h - sessions[-1]["fin"]).total_seconds() <= PAUSE_SESSION_MIN * 60:
            sessions[-1]["fin"] = h
            sessions[-1]["pixels"] += 1
        else:
            sessions.append({"debut": h, "fin": h, "pixels": 1})
    res["sessions"] = [
        {"debut": s["debut"].isoformat(), "fin": s["fin"].isoformat(),
         "duree_min": int((s["fin"] - s["debut"]).total_seconds() // 60), "pixels": s["pixels"]}
        for s in sessions
    ]
    histo = [0] * 24
    for h in heures:
        histo[h.hour] += 1
    res["pixels_par_heure"] = histo
    nuit = sum(histo[2:5])
    res["pixels_entre_2h_et_5h"] = nuit

    # retouches par d'autres (blagues, ajouts…) et restaurations, après la fin de construction
    retouches = []
    if i_fin is not None:
        en_cours, debut, pire = False, None, 1.0
        for i in range(i_fin, t.size):
            if not en_cours and taux[i] < SEUIL_RETOUCHE:
                en_cours, debut, pire = True, i, float(taux[i])
            elif en_cours:
                pire = min(pire, float(taux[i]))
                if taux[i] >= SEUIL_RESTAUREE:
                    retouches.append({"le": _dt(idx, t[debut]).isoformat(), "min_pct": round(pire * 100, 1),
                                   "restauree_le": _dt(idx, t[i]).isoformat(),
                                   "duree_h": round((temps[t[i]] - temps[t[debut]]) / 3600, 1)})
                    en_cours = False
        if en_cours:
            retouches.append({"le": _dt(idx, t[debut]).isoformat(), "min_pct": round(pire * 100, 1), "restauree_le": None})
    res["retouches"] = retouches

    # stabilité sur la dernière année
    un_an = maintenant - timedelta(days=365)
    recents = [float(taux[i]) for i in range(t.size) if _dt(idx, t[i]) >= un_an]
    min_an = min(recents) if recents else float(taux[-1])
    res["stabilite_min_12_mois_pct"] = round(min_an * 100, 1)

    # badges proposés
    badges = []
    if app and (migration is None or app < migration):
        badges.append("pionnier")
    if app and app.year == 2022:
        badges.append("veteran")
    if res["pixels_poses_estimes"] >= 1000:
        badges.append("batisseur")
    if nuit >= 100:
        badges.append("noctambule")
    if fin and fin <= un_an and min_an >= 0.90:
        badges.append("indemodable")
    res["badges_proposes"] = [{"id": b, "raison": BADGES[b]} for b in badges]

    # répartition des pixels pionniers entre co-auteurs (parts égales par défaut, ou poids nom:3)
    if auteurs:
        poids = {}
        for a in auteurs:
            nom, _, w = a.partition(":")
            poids[nom] = float(w or 1)
        tot = sum(poids.values())
        part = {n: int(round(res["pixels_poses_estimes"] * w / tot)) for n, w in poids.items()}
        res["pixels_pionniers_par_auteur"] = part
        res["badges_par_auteur"] = {
            n: [b for b in badges] + (["millier"] if part[n] >= 1000 else []) for n in poids
        }
    return res


def cmd_zone(args):
    idx = charger(args.index)
    masque = masque_zone(args)
    migration = datetime.fromisoformat(args.migration) if args.migration else None
    auteurs = [a for a in (args.auteurs or "").split(",") if a]
    res = analyser_zone(idx, masque, auteurs, migration, args.decalage_heures)
    print(json.dumps(res, ensure_ascii=False, indent=2))


# --------------------------------------------------------------------------- conflits
def cmd_conflits(args):
    """claims.json : [{"id": "c1", "joueur": "kaelen", "rect": [720,96,48,48]}, …]"""
    claims = json.load(open(args.claims, encoding="utf-8"))
    masques = {}
    for c in claims:
        m = np.zeros((H, W), dtype=bool)
        x, y, w, h = c["rect"]
        m[y:y + h, x:x + w] = True
        masques[c["id"]] = m
    sortie = []
    ids = list(masques)
    for i, a in enumerate(ids):
        for b in ids[i + 1:]:
            inter = int((masques[a] & masques[b]).sum())
            if inter:
                union = int((masques[a] | masques[b]).sum())
                sortie.append({"claims": [a, b], "pixels_communs": inter, "recouvrement_pct": round(100 * inter / union, 1)})
    print(json.dumps(sortie, ensure_ascii=False, indent=2))


# --------------------------------------------------------------------------- stats globales
def cmd_stats(args):
    idx = charger(args.index)
    os.makedirs(args.dossier, exist_ok=True)
    temps, ev_t, ev_pos, ev_col = idx["temps"], idx["ev_t"], idx["ev_pos"], idx["ev_col"]
    quand = temps[ev_t] + args.decalage_heures * 3600
    jours = (quand // 86400).astype(np.int64)
    uj, nj = np.unique(jours, return_counts=True)
    par_jour = {depuis_epoch(int(j) * 86400).strftime("%Y-%m-%d"): int(n) for j, n in zip(uj, nj)}
    heures = ((quand % 86400) // 3600).astype(np.int64)
    par_heure = np.bincount(heures, minlength=24).tolist()
    uc, nc = np.unique(ev_col, return_counts=True)
    top = np.argsort(-nc)[:15]
    couleurs = [{"hex": f"#{int(uc[i]):06X}", "pixels": int(nc[i])} for i in top]
    # activité : nombre de changements par pixel ; retouches : nombre de couleurs différentes par pixel
    activite = np.bincount(ev_pos, minlength=W * H).reshape(H, W)
    cle = ev_pos.astype(np.int64) * (1 << 24) + ev_col.astype(np.int64)
    distinctes = np.bincount((np.unique(cle) >> 24).astype(np.int64), minlength=W * H).reshape(H, W)

    def heatmap(a, nom):
        v = np.log1p(a.astype(np.float64))
        v = (255 * v / max(v.max(), 1e-9)).astype(np.uint8)
        Image.fromarray(v, "L").save(os.path.join(args.dossier, nom))
    heatmap(activite, "heatmap_activite.png")
    heatmap(distinctes, "heatmap_retouches.png")

    # zones les plus retouchées (cases de 32 px)
    case = 32
    grille = distinctes.reshape(H // case, case, W // case, case).sum(axis=(1, 3))
    ordre = np.dstack(np.unravel_index(np.argsort(-grille.ravel())[:10], grille.shape))[0]
    retouchees = [{"x": int(cx * case), "y": int(cy * case), "taille": case, "score": int(grille[cy, cx])} for cy, cx in ordre]
    res = {"pixels_par_jour": par_jour, "pixels_par_heure": par_heure, "couleurs_les_plus_utilisees": couleurs,
           "zones_les_plus_retouchees": retouchees, "total_changements": int(ev_t.size)}
    with open(os.path.join(args.dossier, "stats.json"), "w", encoding="utf-8") as f:
        json.dump(res, f, ensure_ascii=False, indent=2)
    print(json.dumps({k: v for k, v in res.items() if k != "pixels_par_jour"}, ensure_ascii=False, indent=2))


# --------------------------------------------------------------------------- mini-timelapse
def cmd_apercu(args):
    idx = charger(args.index)
    x, y, w, h = map(int, args.rect.split(","))
    masque = np.zeros((H, W), dtype=bool)
    masque[y:y + h, x:x + w] = True
    t, p, c, taux, _, _, _ = _replay(idx, masque)
    etat = idx["premiere"].reshape(H, W)[y:y + h, x:x + w].copy()
    if t.size == 0:
        sys.exit("Aucun changement dans la zone.")
    reperes = set(np.linspace(0, t.size - 1, args.images).astype(int).tolist())
    images = []
    for i in range(t.size):
        py, px = divmod(int(p[i]), W)
        etat[py - y, px - x] = c[i]
        if i in reperes:
            rgb = np.stack([(etat >> 16) & 255, (etat >> 8) & 255, etat & 255], axis=-1).astype(np.uint8)
            images.append(Image.fromarray(rgb).resize((w * args.echelle, h * args.echelle), Image.NEAREST))
    images[0].save(args.sortie, save_all=True, append_images=images[1:], duration=350, loop=0)
    bande = Image.new("RGB", (len(images) * images[0].width, images[0].height))
    for i, im in enumerate(images):
        bande.paste(im, (i * im.width, 0))
    bande.save(os.path.splitext(args.sortie)[0] + "_bande.png")
    print(json.dumps({"gif": args.sortie, "images": len(images)}, ensure_ascii=False))


def main():
    ap = argparse.ArgumentParser(description="Analyse des sauvegardes BetterPlace")
    sp = ap.add_subparsers(dest="cmd", required=True)
    a = sp.add_parser("indexer", help="indexer les sauvegardes (dossiers bak/ ou archives .zip)")
    a.add_argument("sources", nargs="+")
    a.add_argument("--sortie", default="index.npz")
    a.add_argument("--verbeux", action="store_true")
    a.set_defaults(f=cmd_indexer)
    for nom, f in (("zone", cmd_zone), ("apercu", cmd_apercu)):
        z = sp.add_parser(nom)
        z.add_argument("index")
        z.add_argument("--rect", help="x,y,largeur,hauteur")
        if nom == "zone":
            z.add_argument("--masque", help="PNG noir/blanc de la taille du canvas (blanc = zone)")
            z.add_argument("--auteurs", help="pseudos séparés par des virgules, poids optionnel : tiago:2,evan")
            z.add_argument("--migration", help="date de mise en service des comptes (AAAA-MM-JJ)")
        else:
            z.add_argument("--sortie", default="apercu.gif")
            z.add_argument("--images", type=int, default=6)
            z.add_argument("--echelle", type=int, default=4)
        z.add_argument("--decalage-heures", type=int, default=0, help="décalage vers l'heure de Paris si les noms sont en UTC")
        z.set_defaults(f=f)
    cf = sp.add_parser("conflits")
    cf.add_argument("index")
    cf.add_argument("claims")
    cf.set_defaults(f=cmd_conflits)
    st = sp.add_parser("stats")
    st.add_argument("index")
    st.add_argument("--dossier", default="stats")
    st.add_argument("--decalage-heures", type=int, default=0)
    st.set_defaults(f=cmd_stats)
    args = ap.parse_args()
    args.f(args)


if __name__ == "__main__":
    main()
