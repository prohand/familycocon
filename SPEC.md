# SPEC — FamilyCocon

Application web d'organisation familiale, auto-hébergée, pensée pour le téléphone.
**Priorité absolue : la légèreté** (RAM, CPU, taille de l'image).

## 1. Choix techniques

| Sujet | Choix | Pourquoi |
|---|---|---|
| Langage | Go (bibliothèque standard) | même façon de faire que mirador-stack ; compilation croisée amd64/arm64 triviale |
| Serveur HTTP | `net/http` (routage Go ≥ 1.22) | aucun framework |
| Base | SQLite, `modernc.org/sqlite` (pur Go) | pas de cgo → binaire statique ; une seule connexion, mode WAL |
| Requêtes | SQL écrit à la main + **sqlc** | chaque requête est vérifiée contre le schéma à la génération ; code Go typé, sans dépendance à l'exécution |
| Gabarits | `html/template` embarqués (`embed`) | échappement automatique (pas d'injection HTML) |
| Front | htmx 2.0.11 + `app.js` (~3 Ko) + CSS à la main | ~20 Ko gzip au total ; pas de framework JS |
| Police | Nunito (variable, latin), embarquée | 39 Ko, licence OFL ; aucun appel à Google Fonts |
| Temps réel | Server-Sent Events (bibliothèque standard ; `EventSource` du navigateur, sans l'extension htmx) | plus simple que WebSocket, passe par tous les proxys, reconnexion automatique |
| Mots de passe | Argon2id (`golang.org/x/crypto`) | repris de mirador-stack (paramètres OWASP, 2 calculs simultanés au plus) |
| Web Push | RFC 8291/8292 écrit en Go (repris de mirador-stack) | aucune dépendance |
| Journal | `log/slog` (JSON en production) | bibliothèque standard |
| Image | `scratch` : binaire + certificats + licences | ni shell ni outil ; utilisateur 65532 |

Dépendances directes : `modernc.org/sqlite`, `golang.org/x/crypto`, `rsc.io/qr` (QR code
de la double authentification et des cartes de fidélité, ~30 Ko, BSD). Rien d'autre.

## 2. Configuration
Variables d'environnement (`.env.exemple`) : `FC_URL_PUBLIQUE`, `FC_PROXYS`,
`FC_INSCRIPTIONS`, `FC_NIVEAU_LOGS`, `FC_FORMAT_LOGS`, `FC_SAUVEGARDE_HEURE`,
`FC_SAUVEGARDE_GARDER`, `FC_DUREE_SESSION_JOURS`, `FC_CONTACT_PUSH`, `GOMEMLIMIT`.
Le fuseau horaire et la monnaie se règlent **dans l'interface** (réglages de la famille).

## 3. Base
- Dates en secondes Unix (UTC) ; l'affichage suit le fuseau de la famille.
- Migrations numérotées, embarquées, une transaction chacune ; copie `VACUUM INTO`
  avant de migrer une base existante (retour arrière = reprendre la copie).
- Jetons (sessions, invitations, appareils) : seule leur empreinte SHA-256 est en base.

## 4. Comptes et rôles
- Un parent crée la famille, puis invite les autres adultes par lien (usage unique, 7 jours).
- Les enfants sont créés par un parent, avec un code PIN (4 à 6 chiffres).
- Connexion enfant : sur un **appareil de confiance** (où un parent s'est connecté,
  ou ouvert avec un lien donné par un parent), l'enfant touche son avatar puis tape son PIN.
- Rôle enfant : ne peut rien supprimer ; pas d'accès au budget ni aux réglages.
- Modules activables / désactivables par un parent (réglages de la famille) :
  un module désactivé n'a plus de tuile, de raccourci ni de page (404).
- Raccourcis de la barre du bas : 4 cases autour du « + », choisies par chaque membre.

## 4 bis. Double authentification et passkeys
Pour les comptes à mot de passe (parents, ados). Code repris de mirador-stack
(TOTP, codes de secours, vérification WebAuthn avec son petit décodeur CBOR).
- **Application (TOTP)** : QR code généré par le serveur (rien ne sort), code à
  6 chiffres, ±30 s, anti-rejeu (le dernier pas accepté est gardé). 10 codes de
  secours à usage unique, gardés en empreinte, montrés une seule fois.
- **Passkeys** (WebAuthn) : jusqu'à 10 par compte.
  - seule, sans mot de passe : l'appareil doit vérifier la personne (code,
    empreinte, visage) ;
  - ou en second facteur après le mot de passe (un simple toucher suffit).
  - Domaine et origine tirés de `FC_URL_PUBLIQUE` ; impossible sur une adresse IP.
- Connexion en attente du second facteur : 5 minutes, en mémoire (cookie `fc_attente`).
- Désactiver la 2FA ou refaire des codes de secours : mot de passe redemandé.
- Téléphone perdu : un parent réinitialise la 2FA et les passkeys d'un membre
  (ses sessions sont fermées).
- Le secret TOTP est en clair dans la base (comme les empreintes de mots de
  passe, la base est à protéger) : le chiffrer avec une clé gardée à côté ne
  protégerait de rien, et une clé gardée ailleurs casserait les restaurations.

## 5. Sécurité (modèle de menaces)
| Menace | Parade |
|---|---|
| Vol de mot de passe en base | Argon2id ; jetons stockés en empreinte |
| Devinette de mot de passe / PIN | limite de tentatives par compte et par IP, en mémoire ; durée constante (empreinte leurre) |
| Mot de passe volé | double authentification (application ou passkey), codes à usage unique, anti-rejeu |
| Hameçonnage | passkeys : liées au domaine et à l'origine, vérifiées par le serveur |
| CSRF | `http.CrossOriginProtection` (Sec-Fetch-Site / Origin) **et** jeton par session envoyé par htmx (en-tête) et dans les formulaires |
| Vol de session | cookie `HttpOnly`, `Secure` (https), `SameSite=Lax`, préfixe `__Host-` |
| Injection HTML / script | `html/template` ; CSP stricte sans script ni style « inline » |
| IP falsifiée | `X-Forwarded-For` lu seulement si la requête vient de `FC_PROXYS` |
| Gros envois | corps limité à 1 Mio ; délais de lecture |
| Conteneur compromis | `scratch`, non root, lecture seule, `cap_drop: ALL` |

## 6. Déploiement
- Image `ghcr.io/prohand/familycocon` (amd64 + arm64), publiée par la CI sur un tag `vX.Y.Z`.
- Reverse proxy (Nginx ou Traefik) : transmettre l'en-tête `Host`, ne pas mettre
  en tampon `/direct` (SSE). Exemples dans `deploy/`.
- `/health` : JSON, 200 si la base répond, 503 sinon. `familycocon -sante` pour Docker.
- Sauvegarde : `familycocon -sauvegarde FICHIER` ou `-sauvegarde -` (sortie
  standard), à chaud et vérifiée ; sauvegarde automatique quotidienne
  (`FC_SAUVEGARDE_HEURE`, rotation `FC_SAUVEGARDE_GARDER`) ; script hôte
  `scripts/sauvegarde.sh`.
- Restauration : `familycocon -restaurer FICHIER`, serveur arrêté (vérifie
  l'intégrité et la version, garde l'ancienne base) ; `scripts/restauration.sh`.

## 7. Notifications push
- Clé VAPID créée au premier démarrage, gardée en base (table `serveur`) :
  elle suit les sauvegardes, les abonnements restent valides après une restauration.
- Envoi en arrière-plan (file de 64, pas d'attente côté requête) ; pas de push
  à un membre qui a une page ouverte (il voit déjà la notification à l'écran).
- Chaque membre choisit les modules qui le notifient (courses : non par défaut).
- Services de push acceptés : Google, Apple, Mozilla, Microsoft (liste fermée).

## 8. Modules ajoutés après la v0.2.0
- Ordre validé : Notes, Anniversaires, Adresses, Cartes de fidélité, Véhicules,
  Animaux, Prestataires, Projets. Inventaires : abandonné.
- Droits communs : tout le monde consulte, ajoute, corrige et coche ; seuls les
  parents suppriment. Notes privées : visibles par leur auteur seul.
- Anniversaires et Adresses partagent une table `contacts` ; la date de naissance
  des membres sert aussi aux anniversaires.
- Cartes de fidélité : code-barres dessiné en SVG par le serveur (EAN-13,
  Code 128), QR code en PNG (`rsc.io/qr`, déjà présent). Aucune image envoyée.
- Suivi commun (paquet `suivi`, tables `echeances` et `carnet`) pour Véhicules,
  Animaux, Prestataires et les dépenses des Projets :
  - une échéance peut revenir tous les N mois ; « C'est fait » écrit une ligne au
    carnet et reporte l'échéance de N mois à partir de la date où c'est fait ;
  - un kilométrage saisi au carnet met à jour le compteur (il ne recule jamais).
- Projets : étapes (date, responsable), objectif, budget ; les dépenses vont au
  carnet commun (sans échéances : les étapes en tiennent lieu).
- Rappels (`appli/rappels.go`) : chaque module fournit ses dates ; l'accueil
  affiche « À venir » (30 jours) ; une notification part à 9 h (fuseau de la
  famille) pour aujourd'hui et J+7. Réglable dans Moi › Notifications (« Rappels »).
- Liens tel:, sms:, mailto: construits côté serveur à partir de valeurs nettoyées
  (chiffres seulement pour le téléphone, http/https seulement pour un site).

## 9. Budget récurrent, abonnements, tâches dans l'agenda
- Table `recurrences` (migration 0015) : l'occurrence n°k tombe à début + k
  périodes (le 31 devient le dernier jour du mois) ; `faites` compte les
  occurrences écrites. Pas de tâche de fond : les occurrences dues sont écrites
  à l'ouverture du budget, de l'accueil ou des abonnements (une seule fois,
  même si deux pages s'ouvrent en même temps).
- Modifier une récurrence ne touche que les prochaines fois. Supprimer une
  récurrence garde les opérations passées.
- Les anciennes opérations « fixes » sont devenues des récurrences mensuelles.
- Abonnements : une récurrence + emoji, fin d'engagement, page de gestion,
  notes (jamais de mot de passe). Réservé aux parents ; leurs rappels et
  notifications ne vont jamais aux enfants.
- Agenda : les modules peuvent y montrer des éléments discrets (`AjouterCalendrier`).
  Les tâches « chaque jour » et « certains jours » y sont, à partir de leur
  création. Filtre « Toutes / Les miennes / Masquées » gardé dans un cookie.
