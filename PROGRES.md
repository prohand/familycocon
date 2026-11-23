# PROGRES — FamilyCocon

| Étape | État |
|---|---|
| 0. Maquette statique (accueil, courses, logo) | **fait**, validée |
| 1. Socle : config, base, serveur, /health, Docker, CI | **fait** |
| 2. Comptes, rôles, invitations, PIN, réglages (modules, raccourcis) | **fait** |
| 3. Courses + temps réel (SSE) + notifications | **fait** |
| 4. Tâches ménagères | **fait** |
| 5. Agenda (mois, semaine) | **fait** |
| 6. Menus de la semaine → courses | **fait** |
| 7. Budget + courbe SVG | **fait** |
| 8. Espace enfants (étoiles, récompenses) | **fait** |
| 9. PWA + Web Push | **fait** |
| 10. Sauvegarde / restauration, README, mesures finales | **fait** |
| MFA : TOTP, codes de secours, passkeys (v0.2.0) | **fait** |
| Sécurité plus visible (page Moi, conseil sur l'accueil) | **fait** |
| Notes (partagées et privées) | **fait** |
| Anniversaires, Adresses, rappels « À venir » + notification du matin | **fait** |
| Cartes de fidélité | **fait** |
| Véhicules, Animaux, Prestataires (échéances + carnet) | **fait** |
| Projets | **fait** |
| Budget : opérations qui reviennent (automatiques) | **fait** |
| Abonnements (parents), reliés au budget | **fait** |
| Tâches dans l'agenda | **fait** |
| Agenda : plus de répétitions + date de fin | **fait** |
| Contacts (carnet central, vCard, liens avec Prestataires, Animaux, Véhicules, Agenda) | **fait** |
| Revue sécurité, bugs, design, performance (v0.4.3, SPEC § 12) | **fait** |
| Suite de la revue : pages à jour toutes seules, `latest` après la Release, garde sur les modules (v0.4.4, SPEC § 13) | **fait** |
| Revue des fonctions : lot 1 (bugs), lot 2 (petites fonctions), lot 3 (socle et interface), lot 4 (grosses fonctions, module Garanties) (v0.5.0, SPEC § 14) | **fait** |
| Cartes de fidélité : type détecté, scan au téléphone ; avatars des membres (v0.6.0, SPEC § 15) | **fait** |
| Photos des tickets et factures dans le carnet ; reprise depuis Fuelio (v0.6.0, SPEC § 15) | **fait** (essayée par l'utilisateur sur sa vraie sauvegarde) |
| Notes : listes faciles à créer et à remplir, photos (v0.7.0, SPEC § 16) | **fait** |
| Véhicules : onglets Entretien / Pleins, chiffres et 4 graphiques SVG (v0.7.0, SPEC § 17) | **fait** |
| Véhicules réservés aux parents ; « Voir les pleins plus anciens » ; scan du ticket de carburant (v0.7.0, SPEC § 17-18) | **fait** |

## Mesures

| Date | Étape | Binaire | Image | RAM au repos | RAM en charge |
|---|---|---|---|---|---|
| 22/11/2026 | 1 | 12,7 Mo | 19 Mo (affiché par Docker) | 6,6 Mio | — |
| 22/11/2026 | 10 | 15,7 Mo (amd64), 14,7 Mo (arm64) | ~16 Mo installée (couches), 6,8 Mo compressée | 8,3 Mio au démarrage, 7,3 Mio après usage | 19,4 Mio (30 SSE + 20 clients, 190 req/s à 0,5 cœur, 0 erreur) |
| 22/11/2026 | modules | 16,3 Mo (amd64), 15,3 Mo (arm64) | non remesurée (≈ +0,4 Mo) | +2 Mio environ (10 Mio de mémoire propre contre 8 avant, hors Docker) | — |
| 23/11/2026 | revue v0.4.3 | 16,9 Mo (amd64), contre 16,6 avant | non remesurée | 11,2 Mio de mémoire propre au démarrage, comme avant (hors Docker) | 27,6 Mio de mémoire totale du processus après un parcours de toutes les pages (hors Docker) |
| 23/11/2026 | v0.5.0 (4 lots) | 17,4 Mo (amd64), contre 16,9 avant (+0,6) | non remesurée | 14,2 Mio de mémoire propre au démarrage, contre 11,3 mesurés au même moment pour v0.4.4 (hors Docker) | 16,0 Mio de mémoire propre (31,3 Mio en tout) après un parcours de toutes les pages, contre 12,9 (27,3) pour v0.4.4 |
| 23/11/2026 | v0.6.0 (photos du carnet, Fuelio) | 17,9 Mo (amd64), contre 17,4 pour v0.5.0 | non remesurée | 30 Mio (RSS) au démarrage | 62 Mio (RSS) au plus fort d'une reprise Fuelio de 85 Mo (300 pleins, 12 photos de 12 Mpx, ~10 s), 34 Mio juste après (mémoire rendue au système) |
| 23/11/2026 | v0.6.0, format réel de Fuelio | 17,9 Mo | non remesurée | — | 66 Mio (RSS) au plus fort pour un `.fuelio` de 296 Mo (200 pleins, 40 photos PNG de 7 Mpx dans pictures.data), ~30 s ; 33 Mio juste après ; 287 Mo de disque pris le temps de la reprise |
| 23/11/2026 | v0.7.0 (notes, pleins, graphiques, scan du ticket) | 19,9 Mo (amd64), contre 17,9 pour v0.6.0 (+1,8 Mo : lecteur de tickets compressé, servi depuis le binaire sans être chargé en mémoire) | non remesurée | 33 Mio (RSS) au démarrage, comme v0.6.0 | scan d'un vrai ticket : ~5 s dans le navigateur (le serveur ne fait que l'analyse du texte) |

Notes :
- v0.6.0 : une seule photo décodée à la fois (pour la reprise Fuelio comme
  pour l'envoi d'une photo) ; le fichier Fuelio est écrit sur le disque,
  jamais gardé en mémoire.
- v0.5.0 : les +3 Mio au démarrage viennent probablement des gabarits (7 pages de
  plus, chacune analysée avec tous les partiels) et des nouveaux modules ;
  aucune donnée n'est gardée en mémoire en plus (la « Corbeille » des
  annulations est vide au repos).
- La première mesure de l'étape 10 donnait 38,9 Mio au démarrage : l'empreinte
  « leurre » Argon2id (19 Mio) était calculée au lancement. Corrigé : elle est
  écrite en dur, et la mémoire d'Argon2id est rendue au système 5 s après le
  dernier calcul.
- Le gros du binaire est SQLite en Go pur (modernc), le prix d'un binaire
  statique sans cgo et d'une compilation arm64 sans émulation.

## Non vérifié ici

- Envoi réel d'une notification push vers Google/Apple/Mozilla : le
  chiffrement et la signature sont testés (code repris de mirador-stack, qui
  les vérifie contre une autre implémentation), l'envoi est testé avec un
  faux service. À essayer sur un vrai téléphone, en HTTPS.

Vérifié par la CI GitHub : contrôles, tests et image Docker amd64 + arm64
réussis à chaque étape (le build multi-arch passe sans émulation).

## Idées pour la suite

- Glisser-déposer pour réordonner les listes ; listes de courses « modèles ».
- Export du budget (CSV) (l'export JSON de toute la famille existe).
- Sauvegardes distantes (rclone, SFTP) comme dans mirador-stack.
