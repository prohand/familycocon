# FamilyCocon 🪺

L'organisation de la famille, **auto-hébergée** et **très légère** : courses, agenda,
tâches ménagères, menus, budget et espace enfants, sur le téléphone de chacun.

- Un seul binaire Go, image Docker `scratch` d'environ **16 Mo** (6,8 Mo à télécharger).
- **~8 Mo de RAM** au repos, ~20 Mo au plus fort d'un test de charge.
- amd64 et arm64 (Raspberry Pi).
- Interface en français, pensée pour le téléphone. Installable comme une appli (PWA).

| Accueil | Courses | Espace enfants | Budget (mode sombre) |
|---|---|---|---|
| ![Accueil](docs/captures/accueil.webp) | ![Courses](docs/captures/courses.webp) | ![Espace enfants](docs/captures/enfants.webp) | ![Budget](docs/captures/budget-sombre.webp) |

## Sommaire

- [Fonctions](#fonctions)
- [Installation](#installation)
- [Premier lancement](#premier-lancement)
- [Configuration](#configuration)
- [Mise à jour](#mise-à-jour)
- [Sauvegarde](#sauvegarde)
- [Restauration](#restauration)
- [Supervision](#supervision)
- [Mesures](#mesures)
- [Développement](#développement)

## Fonctions

- **Comptes** : un parent crée la famille et invite les autres adultes par un lien (7 jours, usage unique).
- **Rôles** : parent ou enfant. Un enfant ne supprime rien et ne voit ni le budget ni les réglages.
- **Double authentification et passkeys** (comptes à mot de passe) :
  - code d'une application (Aegis, FreeOTP, Google Authenticator…) avec codes de secours ;
  - passkeys : connexion sans mot de passe (empreinte, visage, code du téléphone), ou en second facteur ;
  - un parent peut tout réinitialiser pour un membre qui a perdu son téléphone.
- **Connexion enfant par code PIN** : sur un appareil où un parent s'est déjà connecté, l'enfant touche son prénom puis tape son code.
- **Courses** :
  - listes partagées en temps réel, produits rangés par rayon, avec un emoji ;
  - « 2 baguettes » et « 1 kg de pommes » sont compris tout seuls ;
  - le rayon choisi pour un produit est retenu.
- **Agenda** : vue semaine et vue mois, couleur par personne, répétition chaque semaine ou chaque année.
- **Tâches ménagères** : à qui, quand (une fois, chaque jour, certains jours, chaque semaine), qui a fait quoi.
- **Menus de la semaine** : midi et soir ; un clic envoie les ingrédients dans la liste de courses.
- **Budget** (parents) :
  - « reste à vivre » du mois et courbe des dépenses ;
  - catégories et filtres ;
  - opérations qui reviennent chaque mois.
- **Espace enfants** :
  - missions à étoiles (1 à 5) ; l'enfant coche, un parent valide ou refuse ;
  - récompenses échangées contre des étoiles ;
  - progrès par semaine.
- **Personnalisation** :
  - modules activables ou désactivables par un parent ;
  - barre du bas personnalisable par chacun ;
  - fuseau horaire et monnaie réglables dans l'interface.
- **Notifications** : petites notifications en bas de l'écran en temps réel, et notifications Web Push sur le téléphone.

## Installation

Prérequis : Docker avec Docker Compose, et un nom de domaine en HTTPS (reverse proxy).

1. Créer un dossier et y récupérer les fichiers :

   ```bash
   mkdir -p /opt/familycocon && cd /opt/familycocon
   ```

   ```bash
   curl -fsSLO https://raw.githubusercontent.com/prohand/familycocon/main/docker-compose.yml
   ```

   ```bash
   curl -fsSL -o .env https://raw.githubusercontent.com/prohand/familycocon/main/.env.exemple
   ```

2. Régler `.env`. Au minimum `FC_URL_PUBLIQUE` (votre adresse en `https://`) et `FC_PROXYS`. Voir [Configuration](#configuration).

3. Démarrer :

   ```bash
   docker compose up -d
   ```

4. Brancher le reverse proxy sur `127.0.0.1:8080` :
   - **Nginx** : exemple dans [`deploy/nginx.conf`](deploy/nginx.conf). Pensez à `proxy_set_header Host $host;` et à `proxy_buffering off` pour `/direct` (le temps réel).
   - **Traefik** : exemple dans [`deploy/docker-compose.traefik.yml`](deploy/docker-compose.traefik.yml).

Le conteneur tourne sans privilège : utilisateur 65532, système de fichiers en lecture seule, `cap_drop: ALL`, 128 Mo de RAM au plus. Toutes les données sont dans le volume `familycocon-donnees` (`/donnees`).

## Premier lancement

1. Ouvrez votre adresse : la page **Créer ma famille** s'affiche. Ensuite, la création de famille se ferme toute seule (`FC_INSCRIPTIONS=premiere`).
2. Dans **Ma famille** :
   - ajoutez les enfants (prénom et code PIN) ;
   - invitez les autres adultes (lien à envoyer) ;
   - choisissez les modules.
3. **Pour la tablette ou le téléphone d'un enfant** : connectez-vous une fois comme parent sur cet appareil, puis déconnectez-vous. L'enfant y trouve ensuite son prénom.
4. **Installer l'appli** :
   - Android (Chrome) : menu ⋮ › *Installer l'application* ;
   - iPhone (Safari) : *Partager* › *Sur l'écran d'accueil*.
5. **Notifications** : page **Moi** › *Activer sur cet appareil*. HTTPS obligatoire. Sur iPhone, il faut d'abord avoir installé l'appli (étape 4).
6. **Sécurité** : page **Moi** › *Double authentification et passkeys*. Les passkeys demandent un nom de domaine en HTTPS (pas une adresse IP).

## Configuration

Variables d'environnement, dans le fichier `.env` (modèle : [`.env.exemple`](.env.exemple)).

| Variable | Défaut | Rôle |
|---|---|---|
| `FC_URL_PUBLIQUE` | `http://localhost:8080` | Adresse vue par les navigateurs. En `https://`, les cookies sont `Secure` et `__Host-`. |
| `FC_PROXYS` | (vide) | Reverse proxys de confiance (IP ou réseau, séparés par des virgules). `X-Forwarded-For` n'est lu que s'il vient d'eux. Docker : `172.16.0.0/12`. |
| `FC_INSCRIPTIONS` | `premiere` | `premiere` (une seule famille), `ouverte` (plusieurs familles) ou `fermee`. |
| `FC_SAUVEGARDE_HEURE` | `03:30` | Sauvegarde automatique quotidienne (`non` pour la couper). |
| `FC_SAUVEGARDE_GARDER` | `7` | Nombre de sauvegardes automatiques gardées. |
| `FC_DUREE_SESSION_JOURS` | `90` | Durée d'une session (prolongée à chaque visite). |
| `FC_NIVEAU_LOGS` / `FC_FORMAT_LOGS` | `info` / `json` | Journal (`debug`, `info`, `warn`, `error` ; `json` ou `texte`). |
| `FC_CONTACT_PUSH` | `mailto:admin@<domaine>` | Contact transmis aux services de notification. |
| `TZ` | `UTC` | Fuseau du serveur (heure des sauvegardes et des logs), ex. `Europe/Paris`. |
| `GOMEMLIMIT` | `48MiB` | Mémoire visée par le ramasse-miettes de Go. |

Le fuseau horaire de l'agenda et la monnaie se règlent dans l'interface (**Ma famille** › *Réglages*).

## Mise à jour

```bash
docker compose pull
```

```bash
docker compose up -d
```

Au démarrage, la base est migrée toute seule. Avant toute migration, une copie est faite dans `/donnees/sauvegardes/avant-migration-…db`. Pour fixer une version, remplacez `latest` par `0.1.0` (par exemple) dans `docker-compose.yml`.

## Sauvegarde

La base SQLite est copiée **à chaud**, le serveur continue de tourner (`VACUUM INTO`). Chaque copie est vérifiée (`PRAGMA integrity_check`).

**1. Sauvegarde automatique** : chaque jour à `FC_SAUVEGARDE_HEURE`, dans le volume (`/donnees/sauvegardes/auto-….db`), avec rotation.

**2. Sauvegarde vers l'hôte** (recommandée, à mettre dans cron) :

```bash
./scripts/sauvegarde.sh /srv/sauvegardes/familycocon
```

- Le script fait une copie `familycocon-AAAAMMJJ-HHMMSS.db` sur l'hôte.
- Il en garde 30 (variable `GARDER`).
- Exemple de ligne cron : `15 2 * * * /opt/familycocon/scripts/sauvegarde.sh /srv/sauvegardes/familycocon`.

**À la main**, en une commande :

```bash
docker exec familycocon /familycocon -sauvegarde - > familycocon.db
```

Pensez à copier ces fichiers ailleurs (autre machine, stockage externe).

## Restauration

Dans le dossier du `docker-compose.yml` :

```bash
./scripts/restauration.sh /srv/sauvegardes/familycocon/familycocon-20261122-021500.db
```

Le script :

1. arrête le conteneur ;
2. vérifie la sauvegarde (intégrité, version pas plus récente que le binaire) ;
3. la met en place, en gardant l'ancienne base dans le volume (`familycocon.db.avant-restauration-…`) ;
4. redémarre le conteneur.

Pour restaurer une sauvegarde automatique du volume, copiez-la d'abord sur l'hôte :

```bash
docker cp familycocon:/donnees/sauvegardes/auto-20261122-033000.db .
```

## Supervision

**Santé** : `GET /health` répond en JSON (200 si la base répond, 503 sinon) :

```json
{"etat":"ok","version":"0.1.0","schema":8,"depuis_s":3600}
```

**Docker** : le `HEALTHCHECK` appelle `familycocon -sante` (l'image n'a ni curl ni wget).

**Logs** : en JSON sur la sortie standard (`docker logs familycocon`). Les échecs de connexion sont écrits avec l'IP. Exemple de filtre fail2ban :

```ini
[Definition]
failregex = "msg":"(échec de connexion|mauvais code PIN|connexion refusée : trop d'essais)".*"ip":"<HOST>"
```

**Protections intégrées** :
- limite de tentatives par IP et par compte, avec verrou progressif (15 min, puis le double) ;
- protection CSRF (origine des requêtes, plus un jeton par session) ;
- CSP stricte ;
- mots de passe et codes PIN hachés en Argon2id.

## Mesures

Mesuré le 22/11/2026, image amd64, compose fourni (`cpus: 0.5`, `mem_limit: 128m`).

| Mesure | Valeur |
|---|---|
| Binaire | 15,7 Mo (amd64), 14,7 Mo (arm64) |
| Image | ~16 Mo installée, 6,8 Mo à télécharger |
| RAM au démarrage | 8,3 Mio |
| RAM au repos (après usage) | 7,3 Mio |
| RAM en charge (30 pages temps réel + 20 clients) | 19,4 Mio au plus |
| Débit (limité à 0,5 cœur) | ~190 pages/s, 0 erreur |

## Développement

Prérequis : Go 1.27.

| Commande | Rôle |
|---|---|
| `make lancer` | serveur local sur http://localhost:8080 (données dans `./donnees`) |
| `make test` | tests, avec détection des accès concurrents |
| `make verif` | formatage, `go vet`, code SQL généré à jour |
| `make sql` | régénère l'accès à la base après un changement dans `internal/donnees/requetes/` |
| `make binaires` | binaires statiques linux amd64 et arm64 dans `dist/` |
| `make image` | image Docker locale |

```
cmd/familycocon/      point d'entrée (serveur, -sante, -sauvegarde, -restaurer)
internal/appli/       cœur : sessions, CSRF, gabarits, temps réel, push, comptes, réglages
internal/base/        SQLite (WAL), migrations embarquées, sauvegarde, restauration
internal/donnees/     requêtes SQL (requetes/*.sql) et code généré par sqlc
internal/courses/     un paquet par module : courses, agenda, taches, menus, budget, enfants
internal/repetition/  règles de répétition (tâches, missions)
internal/webpush/     Web Push (RFC 8291/8292), repris de mirador-stack
web/gabarits/         pages HTML (html/template)
web/statique/         CSS, htmx, app.js, police Nunito, icônes, manifeste, service worker
deploy/               exemples Nginx et Traefik
scripts/              sauvegarde et restauration
```

Publier une version, sans terminal : sur GitHub, *Actions* › *Release* › *Run workflow*, puis saisir la version (ex. `0.1.0`). On peut aussi pousser un tag `vX.Y.Z`. La CI refait les tests, publie l'image amd64 + arm64 sur `ghcr.io`, crée le tag et la Release avec les binaires.

Les choix techniques et le modèle de menaces sont dans [`SPEC.md`](SPEC.md), l'avancement dans [`PROGRES.md`](PROGRES.md).

## Licence

GPL-3.0-or-later ([`LICENSE`](LICENSE)). Composants tiers : [`THIRD-PARTY.md`](THIRD-PARTY.md).
