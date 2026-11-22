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

Dépendances directes : `modernc.org/sqlite`, `golang.org/x/crypto`. Rien d'autre.

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

## 5. Sécurité (modèle de menaces)
| Menace | Parade |
|---|---|
| Vol de mot de passe en base | Argon2id ; jetons stockés en empreinte |
| Devinette de mot de passe / PIN | limite de tentatives par compte et par IP, en mémoire ; durée constante (empreinte leurre) |
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
- Sauvegarde : `familycocon -sauvegarde FICHIER` (à chaud, vérifiée), plus une
  sauvegarde automatique quotidienne (`FC_SAUVEGARDE_HEURE`).
