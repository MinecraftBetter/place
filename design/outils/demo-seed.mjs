// Données de démo pour relire le redesign en local (serveur lancé avec -devAuth -admins tiago,evan).
// Usage : node design/outils/demo-seed.mjs   (BASE=http://localhost:8080 par défaut)
// À lancer une seule fois sur une base de dev : les coups de cœur de musées sont des bascules,
// et les mots du livre d'or s'ajoutent à chaque passage.
const BASE = process.env.BASE ?? 'http://localhost:8080';
const jar = {};
async function login(pseudo) {
  const r = await fetch(BASE + '/auth/dev', {method: 'POST', body: new URLSearchParams({pseudo}), redirect: 'manual'});
  const ck = r.headers.getSetCookie().map(c => c.split(';')[0]).join('; ');
  jar[pseudo] = ck;
  return ck;
}
async function call(pseudo, method, path, body) {
  const r = await fetch(BASE + path, {method, headers: {Cookie: jar[pseudo], 'Content-Type': 'application/json', Origin: BASE}, body: body ? JSON.stringify(body) : undefined});
  const j = await r.json().catch(() => ({}));
  if (!r.ok) console.log('!', method, path, r.status, j.error ?? '');
  return j;
}
const players = ['Tiago', 'Evan', 'Brindille', 'Zozo42', 'Mamie Pixel', 'Lucie_px', 'Nyx', 'Poulpe', 'Kaelen', 'Sam'];
for (const p of players) await login(p);
const slug = p => p.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
// bios
const bios = {Brindille: "Des vagues, un labyrinthe, et beaucoup de bleu.", 'Zozo42': "Je dessine des blagues. Parfois elles sont drôles.", 'Mamie Pixel': "Un pixel par jour depuis 2022. Le sapin, c'est moi.", 'Lucie_px': "Couchers de soleil et dégradés."};
for (const [p, bio] of Object.entries(bios)) {
  await fetch(BASE + '/api/me/profile', {method: 'PATCH', headers: {Cookie: jar[p], 'Content-Type': 'application/json', Origin: BASE}, body: JSON.stringify({bio})});
}
const existing = (await call('Tiago', 'GET', '/api/oeuvres')).oeuvres;
const have = new Set(existing.map(o => o.titre));
const works = [
  ['Brindille', 'Bord de mer', [397, 250, 115, 122], ['pionnier', 'veteran']],
  ['Brindille', 'Labyrinthe doré', [257, 505, 64, 88], []],
  ['Zozo42', 'Pas vraiment', [846, 512, 88, 76], ['pionnier']],
  ['Mamie Pixel', 'Sapin de Noël', [980, 415, 44, 90], ['pionnier', 'indemodable']],
  ['Lucie_px', 'Dunes au couchant', [0, 513, 127, 204], ['pionnier', 'batisseur']],
];
for (const [p, titre, rect, badges] of works) {
  if (have.has(titre)) continue;
  await call('Tiago', 'POST', '/api/admin/zones/apply', {masque: {type: 'rect', rect}, action: 'assign', user: slug(p), titre, badges, motif: 'démo pour la relecture'});
}
const all = (await call('Tiago', 'GET', '/api/oeuvres')).oeuvres;
const id = t => all.find(o => o.titre === t)?.id;
// likes
const likes = {'Bord de mer': ['Tiago', 'Zozo42', 'Nyx', 'Lucie_px', 'Mamie Pixel'], 'Pas vraiment': ['Brindille', 'Tiago', 'Evan', 'Poulpe', 'Sam', 'Kaelen'], 'Sapin de Noël': ['Brindille', 'Lucie_px', 'Nyx'], 'Dunes au couchant': ['Mamie Pixel', 'Brindille', 'Zozo42'], 'Drapeau & baguette': ['Zozo42', 'Brindille', 'Sam'], 'Labyrinthe doré': ['Nyx']};
for (const [t, who] of Object.entries(likes)) {
  const o = all.find(x => x.titre === t);
  if (!o) continue;
  for (const p of who) await call(p, 'POST', `/api/oeuvres/${o.id}/like`, {on: true});
}
// exhibitions in the public museum
const expos = [['Brindille', 'Bord de mer', 'paysages', 'or', 'Ma toute première œuvre. La pizza est arrivée plus tard, je l\'adore.'],
  ['Zozo42', 'Pas vraiment', 'clins-d-oeil', 'bois', 'Ça me fait rire à chaque fois.'], ['Mamie Pixel', 'Sapin de Noël', 'motifs', 'or', 'Le sapin qui revient chaque hiver.'],
  ['Lucie_px', 'Dunes au couchant', 'paysages', 'bois', 'Les couleurs du soir, exactement.']];
for (const [p, t, salle, cadre, mot] of expos) if (id(t)) await call(p, 'POST', `/api/oeuvres/${id(t)}/exposition`, {salle, cadre, marge: 80, mot, timelapse: true, visite: true});
// museums
const W = (t, o = {}) => ({oeuvre_id: id(t), animation: 'aucune', son: 'aucun', voix_id: 0, texte: '', taille: 'moyenne', cadre: 'musee', spot: '#fff8b8', vedette: false, ...o});
const museums = {
  Brindille: {nom: 'Le musée de Brindille', theme: 'maree', particules: 'bulles', cadre: 'or', musique: 'vagues', accueil: 'Des vagues, un labyrinthe, et mes coups de cœur. Mettez le son !', phrase_guide: 'Howdy ! Suis-moi, les vagues c\'est par là.',
    salles: [{titre: 'Salle 1 · Mes œuvres', oeuvres: [W('Bord de mer', {animation: 'construction', texte: 'Ma toute première œuvre. La pizza est arrivée plus tard, je l\'adore.', son: 'vague', vedette: true, taille: 'grande'}), W('Labyrinthe doré', {animation: 'scintille', texte: 'Une soirée entière à deux sur le même PC.', son: 'clochette', spot: '#5eb3ff'})]},
      {titre: 'Salle 2 · Mes coups de cœur', oeuvres: [W('Pas vraiment', {texte: 'Mon coup de cœur : ça me fait rire à chaque fois.', son: 'piece', taille: 'petite', spot: '#ff63aa'}), W('Sapin de Noël', {animation: 'zoom', texte: 'Le sapin qui revient chaque hiver.', son: 'clochette'}), W('Dunes au couchant', {texte: 'Les couleurs du soir, exactement.', spot: '#ffa800'})]}]},
  'Zozo42': {nom: 'Le musée des blagues', theme: 'desert', particules: 'confettis', cadre: 'bois', musique: 'desert', accueil: 'Entrée gratuite, sortie payante. Je rigole.', phrase_guide: 'Bienvenue ! Ici, tout est très sérieux.', guide: 'avatar',
    salles: [{titre: 'La blague', oeuvres: [W('Pas vraiment', {animation: 'construction', vedette: true, son: 'piece', texte: 'Carrière accomplie.'}), W('Drapeau & baguette', {son: 'mouette', texte: 'Cocorico (je suis pas français).'})]}]},
  'Mamie Pixel': {nom: 'Le salon de Mamie', theme: 'galerie', particules: 'neige', cadre: 'or', musique: 'saloon', accueil: 'Essuyez vos pieds, il y a du thé.', phrase_guide: 'Entrez, entrez ! Mamie a préparé des gâteaux.', parcours: 'guide', eclairage: 'tamise',
    salles: [{titre: 'Le salon', oeuvres: [W('Sapin de Noël', {animation: 'scintille', vedette: true, texte: 'Chaque hiver je le repeins un peu.', spot: '#7eed38'}), W('Dunes au couchant', {texte: 'Celle de Lucie, ma voisine de pixels.'}), W('Bord de mer', {animation: 'zoom'})]}]},
  Tiago: {nom: 'Le saloon de Tiago', theme: 'saloon', particules: 'etoiles', cadre: 'bois', musique: 'saloon', accueil: 'Le premier drapeau du canvas, et les amis.', phrase_guide: 'Howdy partner !', entree: 'camera',
    salles: [{titre: 'Le bar', oeuvres: [W('Drapeau & baguette', {animation: 'construction', vedette: true, texte: 'Le tout premier soir, sur le canvas de 256 pixels.'}), W('Pas vraiment', {son: 'piece'})]}]},
};
for (const [p, cfg] of Object.entries(museums)) {
  const cur = (await call(p, 'GET', '/api/me/musee')).musee;
  await call(p, 'PUT', '/api/me/musee', {musee: {...cur, ...cfg}, publie: true});
}
// guestbooks, museum likes, visits
const book = [['Brindille', 'Tiago', 'La musique + les vagues, c\'est parfait. La pizza a trouvé sa place.'], ['Brindille', 'Nyx', 'Notre labyrinthe en salle 1, trop fière.'], ['Brindille', 'Zozo42', 'Ma blague est au musée. Carrière accomplie.'],
  ['Zozo42', 'Brindille', 'J\'ai ri. Ne le dis à personne.'], ['Zozo42', 'Sam', 'Le kazoo manquait, mais bon.'], ['Mamie Pixel', 'Lucie_px', 'Merci pour la place au salon !'], ['Mamie Pixel', 'Kaelen', 'Les gâteaux étaient pixelisés mais délicieux.'], ['Tiago', 'Evan', 'Le premier soir… que de souvenirs.']];
for (const [owner, who, texte] of book) await call(who, 'POST', `/api/musees/${slug(owner)}/livre-or`, {texte});
const mlikes = {Brindille: ['Tiago', 'Nyx', 'Zozo42', 'Lucie_px', 'Poulpe'], 'Zozo42': ['Brindille', 'Sam', 'Evan', 'Kaelen', 'Poulpe', 'Tiago', 'Nyx'], 'Mamie Pixel': ['Lucie_px', 'Brindille'], Tiago: ['Evan', 'Brindille', 'Zozo42']};
for (const [owner, who] of Object.entries(mlikes)) for (const p of who) await call(p, 'POST', `/api/musees/${slug(owner)}/like`);
for (const owner of Object.keys(museums)) for (const p of players) if (p !== owner) await call(p, 'GET', `/api/musees/${slug(owner)}?visite=1`);
console.log('seeded', (await call('Tiago', 'GET', '/api/musees')).musees.map(m => `${m.nom} (${m.n} œuvres, ${m.visites} visites, ${m.likes} ♥)`).join(' · '));
