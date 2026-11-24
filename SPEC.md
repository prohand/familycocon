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
| Front | htmx 2.0.11 + `app.js` (~4 Ko gzip) + CSS à la main | ~33 Ko gzip au total (htmx 17, CSS 12, `app.js` 4) ; pas de framework JS |
| Police | Nunito (variable, latin), embarquée | 39 Ko, licence OFL ; aucun appel à Google Fonts |
| Temps réel | Server-Sent Events (bibliothèque standard ; `EventSource` du navigateur, sans l'extension htmx) | plus simple que WebSocket, passe par tous les proxys, reconnexion automatique |
| Mots de passe | Argon2id (`golang.org/x/crypto`) | repris de mirador-stack (paramètres OWASP, 2 calculs simultanés au plus) |
| Web Push | RFC 8291/8292 écrit en Go (repris de mirador-stack) | aucune dépendance |
| Journal | `log/slog` (JSON en production) | bibliothèque standard |
| Image | `scratch` : binaire + certificats + licences | ni shell ni outil ; utilisateur 65532 |

Dépendances directes : `modernc.org/sqlite`, `golang.org/x/crypto`, `rsc.io/qr` (QR code
de la double authentification et des cartes de fidélité, ~30 Ko, BSD). Rien d'autre.

## 2. Configuration
Variables d'environnement (`.env.exemple`) : `FC_ADRESSE` et `FC_DONNEES` (déjà
réglées dans l'image), `FC_URL_PUBLIQUE` (adresse du site, sans chemin), `FC_PROXYS`,
`FC_INSCRIPTIONS`, `FC_NIVEAU_LOGS`, `FC_FORMAT_LOGS`, `FC_SAUVEGARDE_HEURE`,
`FC_SAUVEGARDE_GARDER`, `FC_DUREE_SESSION_JOURS`, `FC_SESSION_MAX_JOURS`, `FC_CONTACT_PUSH`, `GOMEMLIMIT`.
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
- `/health` : JSON, 200 si la base répond, 503 sinon (version et schéma donnés
  au réseau local seulement). `familycocon -sante` pour Docker.
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

## 10. Contacts, carnet central
- Une seule table `contacts` (migration 0017) : Contacts (tout le carnet),
  Adresses (ceux qui ont une adresse), Anniversaires (ceux qui ont une date)
  et Prestataires (ceux marqués « pro ») en sont des vues. Une seule fiche,
  modifiable partout.
- Les anciens prestataires sont devenus des contacts « pro » ; leurs
  échéances et interventions ont suivi (table `prestataires` supprimée).
- Animaux (`veterinaire_id`), Véhicules (`garage_id`, `assureur_id`) et Agenda
  (`evenements_contacts`) citent un contact ; s'il est supprimé, le lien
  disparaît, la fiche reste.
- Groupes : étiquettes libres dans une colonne (séparées par des virgules,
  10 au plus).
- vCard écrit à la main : export 3.0 (lignes pliées à 75 octets), import
  2.1 / 3.0 / 4.0 (quoted-printable, Latin-1, anniversaire sans année,
  « 1604 » d'Apple). Photos ignorées. Doublon = même nom et même téléphone.
  Import et export réservés aux parents ; 8 Mo au plus pour l'envoi.

## 11. Revue de sécurité et de bugs (23/11/2026)
Quatre relectures (authentification, isolation des familles, entrées, bugs),
chaque point vérifié puis corrigé avec un test :
- connexion : adresse de retour toujours locale ; défis de passkey anonymes
  limités par adresse et rangés à part ; mot de passe redemandé (et limité)
  pour ajouter une passkey ou activer le TOTP ; invitation à usage unique même
  en double envoi ; pas de code PIN pour un compte à mot de passe ; appareils
  de confiance révocables (bouton, et au retrait d'un parent) ;
- notifications et temps réel : désabonnement à la déconnexion, abonnement qui
  suit la personne connectée sur un appareil partagé, supprimé au retrait ou à
  la réinitialisation ; flux temps réel coupé dès que la session n'existe plus ;
  10 appareils par membre ; client d'envoi sans redirection ni adresse interne ;
- entrées : import vCard linéaire et borné ; agenda borné (dates, durée,
  occurrences) ; récurrences bornées (date de départ, nombre) ;
- bugs : étoiles d'une mission réattribuée, abonnement résilié réactivé,
  renouvellement du jour, report de fin de mois, journée au changement d'heure,
  rappels à 9 h dans le fuseau de chaque famille, menus « déjà aux courses ».
Aucun accès aux données d'une autre famille n'a été trouvé.

Non corrigé (choix à faire) : changer de mot de passe ne retire pas les
passkeys ; un inconnu peut bloquer un compte 15 min et plus en ratant le mot
de passe (la passkey reste possible) ; sessions glissantes sans durée maximale (corrigé : FC_SESSION_MAX_JOURS, 1 an par défaut).

## 12. Revue de sécurité, bugs, design et performance (23/11/2026, v0.4.3)
Quatre relectures (authentification, modules, socle/front, déploiement). Aucun
accès aux données d'une autre famille, aucun contournement de rôle trouvé.
Corrigé, avec un test par point quand c'est testable :
- **Sécurité** : pages privées en `Cache-Control: no-store`, historique htmx
  non gardé, cache du navigateur vidé à la déconnexion ; changement de mot de
  passe et suppression d'une passkey limités comme les autres actions sensibles
  (mot de passe redemandé, 5 essais) ; inscriptions limitées par adresse
  (5 par quart d'heure) ; l'identifiant tapé n'est plus écrit au journal ;
  listes de formulaire bornées avant toute requête ; plafonds de lignes par
  famille (listes, produits, tâches, événements, notes, contacts, cartes,
  projets, échéances, carnet) ; `/health` ne donne la version qu'au réseau
  local ; copies de la base non lisibles par les autres comptes de l'hôte ;
  actions GitHub épinglées par empreinte, publication depuis `main` seulement,
  `govulncheck` avant de publier.
- **Bugs** : totaux du carnet (projets, véhicules, animaux) calculés en SQL et
  non sur les 100 dernières lignes ; menus → courses dans une transaction, sans
  doublon ; rappels du matin envoyés même avec une page ouverte, rattrapés
  après un arrêt (jour noté dans la table `serveur`, jusqu'à 20 h) et jamais
  perdus par une file pleine ; retour sur la page affichée après une session
  expirée ; déconnexion qui n'attend plus le service worker ; kilométrage
  illisible refusé ; courbe du mois précédent à la bonne échelle ; membre
  retiré : tâches rendues à tous, notes privées, passkeys et codes effacés
  (migration 0018 pour les retraits passés) ; doubles envois sans effet
  (boutons désactivés pendant l'envoi, et contrôle côté serveur) ; dates
  bornées partout (1900 à aujourd'hui + 50 ans, 2200 pour l'agenda et les menus).
- **Exploitation** : migration en échec : 3 copies « avant-migration » au plus ;
  sauvegarde automatique écrite sous un nom provisoire puis renommée ;
  restauration en flux, avec `fsync`, refusée si un serveur tourne sur le même
  dossier (verrou `familycocon.verrou`), script qui relance le service si le
  fichier est refusé ; tâches de fond protégées des paniques ; exemple Nginx
  pour l'import de 8 Mio ; `FC_URL_PUBLIQUE` avec chemin et `FC_DONNEES` avec
  `?`, `#` ou apostrophe refusés au démarrage.
- **Performance** : membres lus une fois par requête ; tuile et bloc de
  l'accueil partagent leur calcul ; alertes du foyer, récurrences et rappels
  sans requête par ligne ; tâches et missions « une fois » terminées non
  relues ; agenda borné à la période ; liste des notes abrégée ; index
  manquants (migration 0019) ; import vCard en flux (allocations divisées par
  deux) ; envois push par 4 en parallèle ; cache du service worker renommé à
  chaque version (l'ancien est supprimé), police versionnée.
- **Design** : aides communes (`appli.MembresParID`, `TitreJour`,
  `PeutModifier`, `contacts.LienTel`/`LienMail`/`SiteSur`, `budget.LireJour`,
  `Replanifier`) à la place des copies ; requêtes par id filtrées aussi par
  famille ; requêtes mortes et dossier `mockup/` supprimés.

## 13. Suite de la revue (v0.4.4)
Les points laissés « à décider » par la revue sont traités :
- **Pages à jour toutes seules** : abonnements, contacts, adresses,
  anniversaires, cartes de fidélité, véhicules, animaux et prestataires
  (listes et fiches) se rechargent quand un autre membre change quelque chose.
  Pas de route en plus : le bloc redemande sa propre page (`Vue.Adresse`) et
  n'en garde que lui-même (`hx-select`). Sur une fiche qui contient des
  formulaires (`data-maj-prudent`), rien n'est rechargé pendant qu'on y écrit
  ou qu'un formulaire est déplié : la saisie n'est jamais perdue.
- **Publication** : l'étiquette `:latest` n'est posée qu'après la Release
  GitHub (dernier job, sans reconstruire l'image).
- **Modules** : toujours déclarés dans `Catalogue` et branchés dans
  `cmd/familycocon/modules.go` (l'ordre des tuiles reste lisible à un seul
  endroit), mais un oubli ne passe plus : une clé absente du `Catalogue` est
  refusée au démarrage, et un test vérifie que chaque module du `Catalogue` a
  sa page.
- **Image de construction** épinglée par empreinte (`golang:1.27.4-bookworm@sha256:…`).
- **Accueil** : abonnements et anniversaires lus une seule fois par requête.

## 14. Revue des fonctions et de l'interface (v0.5.0)
Relecture des modules du quotidien, de l'argent et du suivi, du socle et de
l'interface. Tout ce qui a été trouvé est fait, en quatre lots, avec des tests.
- **Rappels** : une seule notification par matin (elles se remplaçaient à
  l'écran), qui ouvre la page de l'échéance quand il n'y en a qu'une.
  L'agenda fournit ses événements « Me le rappeler » (migration 0020).
- **Notifications ciblées** (`PublierPour`) : participants d'un événement,
  personne(s) d'une tâche, enfant d'une mission, parents pour « à valider ».
  Les autres pages se mettent quand même à jour. Moi › Notifications ne
  montre que les modules qui notifient (`Notifie` dans le `Catalogue`).
- **Recherche sans accents** : fonction SQL `plier()` (enregistrée dans
  `internal/base/fonctions.go`, code `internal/texte`), pour les notes, le
  budget et la recherche globale (`/chercher`, une fonction par module :
  `AjouterRecherche`).
- **Temps réel** : après une coupure (veille du téléphone), tous les blocs se
  rechargent (`maj-tout`) ; l'accueil écoute `maj` (n'importe quel module).
  Pas de réseau : un message ; un lien ouvre la page hors ligne.
- **Hors ligne** : le service worker garde la dernière liste de courses
  ouverte (cache `familycocon-pages`, vidé par app.js à la déconnexion).
- **Annuler** : une suppression rapide (courses) se rattrape 15 s ; ce qu'il
  faut pour remettre est gardé en mémoire (`Corbeille`, 200 au plus), jamais
  en base.
- **Partager** (`share_target` du manifeste, GET `/partager`) : chaque module
  propose son action (`AjouterPartage`).
- **Thème** : cookie `fc_pref_theme` (clair, sombre, vide = téléphone),
  attribut `data-theme` ; chaque règle sombre du CSS existe deux fois (dans
  la requête média et forcée).
- **Export** `/famille/export.json` (parents) : toutes les tables de la
  famille, section par section, sans secret ni note privée d'un autre.
- **Serveur** : résultat de la sauvegarde automatique noté dans la table
  `serveur` ; montré dans `/health` (réseau local) et, sur un serveur d'une
  seule famille, aux parents (carte « Serveur »).
- **Agenda** : fois annulées (`evenements_exceptions`, migration 0023) ;
  lien secret iCalendar par membre (`agenda_ical`, empreinte seulement,
  migration 0024), flux public `/ical/{jeton}.ics` en lecture seule (3 mois
  avant, 1 an après, fois une par une), coupé au retrait du membre ou à la
  réinitialisation de sa sécurité.
- **Tâches** : « toutes les 2 semaines » (périodes comptées depuis le lundi
  1er janvier 2024) et « chaque mois » ; tour de rôle (colonne `tour`,
  migration 0025) : la fois n revient au membre n modulo leur nombre
  (`Regle.Numero`), sans tâche de fond.
- **Courses** : un produit déjà sur la liste n'est pas ajouté deux fois
  (quantités additionnées) ; « Souvent achetés » ; un parent oublie une
  suggestion.
- **Budget** : catégories du sens choisi seulement ; impôts, assurances,
  crédit, épargne ; plafonds par catégorie (`budget_plafonds`, migration
  0021) ; payé par ; recherche ; vue de l'année.
- **Espace enfants** : mission pour plusieurs enfants ; tout valider ; étoiles
  données ou retirées avec un motif (`etoiles_bonus`, migration 0022), jamais
  sous zéro.
- **Suivi** : échéance modifiable et reportable par tous ; ligne du carnet
  corrigeable ; « ajouter aussi au budget » (`carnet.operation_id`, migration
  0026 : la dépense suit les corrections et part avec la ligne) ; litres d'un
  plein, coût pour 100 km et consommation.
- **Garanties** (nouveau module, migration 0027, ajouté aux familles
  existantes) : table `garanties` ; la fin de garantie est une échéance du
  suivi, tenue à jour à chaque enregistrement. Pas de fichier envoyé : on note
  où est la facture.
- **Contacts** : « Cité par », doublon signalé (même nom sans accents ou
  même numéro), fusion par un parent (champs vides complétés, liens repris).
- **Menus** : ingrédients d'une recette remplis par htmx, cases « déjà à la
  maison », reprise de la semaine, page Recettes.
- **Notes** : listes à cocher (« [ ] ») cochables par tous ceux qui voient la
  note ; une modification sur une version dépassée est refusée (409).
- **Index** des nouveaux liens (migration 0028).
- **Accessibilité** : erreurs en `role="alert"` et affichées 8 s, contour du
  champ d'ajout, boutons de 44 px, `prefers-reduced-motion`, noms des
  boutons de validation et des cases du calendrier.

Choix validés ensuite :
- **Montants cachés aux enfants** (le budget leur est fermé) : coûts du
  carnet, totaux, coût au km, budget et dépensé des projets, prix des
  garanties. Un enfant ne saisit pas de montant ; s'il corrige une ligne ou
  un projet, le montant noté par un parent est gardé.
- **Liste de courses des menus** : choisie quand la famille en a plusieurs,
  retenue par l'appareil (cookie `fc_pref_menus_liste`) ; sinon la première.
- **Icônes** : la carte de fidélité est un dessin SVG écrit dans la page
  (carte à tampons ; emoji de secours 🪪 dans les textes), via la fonction de
  gabarit `emoji`. Icône d'application : nid crème sur fond corail
  (`icone.svg`, PNG 180, 192, 512 dont « maskable »), badge de notification
  monochrome (`badge-96.png`).

### Correctif v0.5.1 : pages vides après un clic
Les liens d'un bloc qui se recharge lui-même (`hx-select="#maj"`) héritaient
de ce réglage : un clic (navigation hx-boost) ne gardait de la page suivante
que son bloc `#maj`, ou rien. Visible dès le premier clic sur une tuile de
l'accueil (v0.5.0), et sur certaines listes depuis la v0.4.4. Ces blocs ont
maintenant `hx-disinherit="*"` ; un test (`web/web_test.go`) refuse un
`hx-select` sans lui.

## 15. Scan des cartes de fidélité, avatars (v0.6.0)
- **Type de code détecté** (`FormatAuto`) : EAN-13, UPC-A ou EAN-8 si les
  chiffres ont une clé juste, sinon Code 128, sinon QR code. Le choix manuel
  reste possible (replié). EAN-8 et UPC-A sont dessinés (UPC-A = EAN-13
  précédé d'un 0, lu pareil en caisse).
- **Scanner la carte** : sur Android, le navigateur lit le code en direct
  (`BarcodeDetector`) ; la caméra n'est permise que sur le formulaire des
  cartes (`Permissions-Policy: camera=(self)`, `camera=()` partout ailleurs).
  Sinon (iPhone), une photo réduite à 1 280 px (JPEG, sous la limite de
  1 Mio) est lue par le serveur (`fidelite.Lire`, sans bibliothèque :
  lignes horizontales et verticales, deux seuils, motifs EAN et Code 128,
  clé de contrôle, marges blanches, vote). Les QR codes ne sont lus que par
  le navigateur. La photo n'est jamais gardée ; image de plus de 16 Mpx
  refusée.
- **Avatars** : un emoji choisi dans une liste fermée (`appli.Avatars`)
  remplace l'initiale dans le rond de couleur du membre (colonne
  `membres.avatar`, migration 0029). Choisi dans Moi, ou par un parent dans
  Ma famille. Pas de photo (base légère).
- **Photos des tickets et factures** sur les lignes du carnet (parents
  seulement : elles montrent des prix). Gardées sur le disque, à côté de la
  base : `<données>/pieces/<famille>/<nom aléatoire>.jpg` ; la base n'a que
  le nom (table `pieces`, migration 0030). Réduites à 1 600 px et réécrites
  en JPEG qualité 80 (`internal/photo`, bibliothèque standard) : ~200 Kio
  par photo, informations cachées du téléphone (lieu GPS) retirées. Une
  photo décodée à la fois ; 24 Mpx au plus ; 10 par ligne, 5 000 par
  famille. Effacées avec leur ligne ou leur objet. Copiées par la
  sauvegarde automatique (`sauvegardes/pieces`, nouveaux fichiers
  seulement) et par `scripts/sauvegarde.sh`.
- **Reprise depuis Fuelio** (`internal/fuelio`, fiche du véhicule,
  parents) : sauvegarde complète `.fuelio` (ZIP), ZIP de synchro ou CSV.
  Les pleins (date, km, litres, prix, ville et notes) et les dépenses
  (titre ou catégorie ; modèles et recettes ignorés) vont au carnet, pas au
  budget. Miles et gallons convertis. Une ligne déjà présente n'est pas
  reprise deux fois. Le fichier (1 Gio au plus ; ~300 Mo pour quelques
  années de tickets) est écrit sur le disque le temps de la lecture, jamais
  gardé en mémoire, puis effacé ; une reprise à la fois, 30 min au plus.
  Format de la sauvegarde complète (vu sur une vraie, 11/2026) : un ZIP
  avec `vehicle-N-local.csv` (un par véhicule ; celui qui porte le nom du
  véhicule est repris) et `pictures.data`, lui-même un ZIP des photos
  (`<GUID>.png`). Un ZIP interne rangé sans compression est lu en place,
  sinon décompressé dans un fichier provisoire. Photos décodées par
  morceaux depuis le ZIP (`photo.PreparerFlux`), une à la fois.
  Rattachement : la partie `## Pictures` du CSV (`Filename`, `Type`,
  `target_id`) donne la ligne (guid ou UniqueId). Pleins et dépenses ont
  les mêmes UniqueId (1, 2…) : le type tranche. Ses valeurs n'étant pas
  publiées, elles sont déduites des photos sans ambiguïté (un seul type par
  sorte de ligne, à 4 contre 1 au moins) ; les autres (trajets…) et les
  cas douteux sont laissés de côté et comptés dans le bilan. Sans
  `## Pictures` : nom du fichier (identifiant, sinon date).

## 16. Notes : listes et photos (v0.7.0)
- **Listes** : bouton « ☑️ Liste » (note qui commence par une case). Dans le
  texte, Entrée au bout d'une case (ou d'une puce « - ») en commence une
  autre ; Entrée sur une case vide termine la liste (app.js). Une case vide
  n'est pas gardée. Sur la note : « Ajouter un élément » (tous ceux qui
  voient la note, comme pour cocher) et « Retirer les éléments cochés »
  (ceux qui peuvent la modifier). Le texte reste la seule source (lignes
  « [ ] » / « [x] ») : rien ne change dans la base. Écriture refusée si la
  note a changé entre-temps (pas d'écrasement). Les cartes de la liste
  dessinent les cases (☐ / ☑).
- **Photos dans une note** : ajoutées et retirées par ceux qui peuvent
  modifier la note, vues par ceux qui la voient (note privée : son auteur
  seul). Même traitement que les tickets du carnet (`photo.Preparer`,
  1 600 px, JPEG sans lieu GPS) et même dossier (`pieces/<famille>/`, copié
  par les sauvegardes) ; table `photos_notes` (migration 0031). 20 par
  note, 5 000 par famille. Effacées avec la note, et avec les notes
  privées d'un membre retiré. Rangement commun : `photo.Ranger`,
  `photo.Chemin`, `photo.Effacer`.

## 17. Véhicules : pleins à part, chiffres et graphiques (v0.7.0)
- **Deux onglets** sur la fiche d'un véhicule (`?onglet=pleins`) :
  « 🔧 Entretien » (échéances, carnet sans les pleins) et « ⛽ Pleins ».
  Un plein = une ligne du carnet avec des litres (rien ne change dans la
  base). Ajouter, corriger ou supprimer un plein ramène sur son onglet.
- **Onglet Pleins** : chiffres (consommation, prix moyen du litre,
  carburant acheté, dépensé en carburant, coût tout compris au km), 4
  graphiques, formulaire « Ajouter un plein » (complet ou partiel, litres
  obligatoires), liste des pleins par pages de 100 (« Voir les pleins plus
  anciens » ajoute la page suivante par htmx, `GET /suivi/{module}/{objet}/pleins?depuis=N`),
  ticket dans « Corriger ».
  Période : 12 mois (mois en cours compris) ou « Tout » (`&periode=tout`).
- **Graphiques** : SVG écrit par le serveur (`internal/graphique`, ~200
  lignes, bibliothèque standard), couleurs par classes CSS (thème clair et
  sombre, CSP inchangée) ; bulle au survol par `<title>`.
  - consommation à chaque plein complet (litres depuis le plein complet
    d'avant, partiels compris ; valeurs hors 1-60 L/100 écartées : plein
    oublié) ; moyenne en pointillé ;
  - prix du litre ; moyenne en pointillé ;
  - dépenses par mois, carburant et entretien empilés ;
  - kilomètres par mois (compteur le plus haut du mois moins le plus haut
    d'avant).
  Au-delà de 36 mois (« Tout »), les barres passent par année.
- **Véhicules réservés aux parents** (`Parents: true` au Catalogue, comme
  Budget et Abonnements) : ni tuile, ni recherche, ni échéances dans « À
  venir », ni rappels, ni mention sur la fiche d'un contact pour un enfant.
  Contrôle central : `Contexte.Voit(module)` (activé et permis à ce
  membre), appliqué par `App.Route` à toutes les pages d'un module réservé
  et par les routes communes du suivi (`/suivi/...`).

## 18. Scan du ticket de carburant (v0.7.0)
- **Bouton « 📷 Scanner le ticket »** dans « Ajouter un plein » (parents) :
  photo (appareil du téléphone), lecture du texte **dans le téléphone**
  par Tesseract.js 7 (Apache-2.0, WebAssembly, modèle français
  « best_int »). Rien ne part chez un tiers ; le serveur ne fait pas
  l'OCR.
- **Fichiers** : `web/ocr/v7/` (~1,8 Mo compressés : moteur SIMD seul,
  modèle 0,7 Mo), gardés en .gz dans le binaire et servis par
  `GET /ocr/v7/…` (Content-Encoding gzip, cache « immutable ») ; lus à la
  demande, jamais chargés en mémoire au démarrage. Téléchargés au premier
  scan seulement. Navigateur sans SIMD WebAssembly (téléphones d'avant
  2021 environ) : « Lecture impossible », on remplit à la main.
- **CSP** : `'wasm-unsafe-eval'` ajouté seulement aux réponses de
  `/ocr/v7/` (le lecteur tourne dans un worker, qui suit la politique de
  sa propre réponse). Le reste du site garde la politique stricte.
- **Lecture** : photo en largeur → essai d'un quart de tour d'abord (puis
  l'autre sens, puis droite) ; image à 1 600 px ; si la date ou un nombre
  manque, seconde lecture à 2 400 px, et les deux textes sont analysés
  ensemble. Pas de retouche de contraste (elle abîme les tickets
  thermiques à points). Essai sur un vrai ticket : ~5 s, tout est lu.
- **Analyse** en Go (`suivi.LireTicket`, testée sur le texte réellement
  lu) : POST `/suivi/ticket` (JSON). Les trois nombres qui vont ensemble
  (litres × prix au litre = prix payé, au centime près) ; les mots de la
  ligne et de la ligne d'avant départagent (« Quantité », « Prix unit. »,
  « Montant ») ; lignes de TVA et en francs écartées ; deux sur trois : le
  troisième se calcule. Date : la première valide, ni future ni de plus
  d'un an. Carburant (SP95, SP98 même lu « SPsB », E10, E85, gazole…) :
  champ « Carburant » ; station (première ligne faite de mots) : note.
- Les champs remplis sont **surlignés** ; le kilométrage reste à taper.
  La **photo** (réduite à 1 200 px par le téléphone) part avec le plein
  (champ caché, base64, 900 Kio au plus) et lui est jointe ; si elle
  n'a pas pu l'être, le plein est gardé et on le dit.
- **Type de carburant** d'un plein (colonne `carnet.carburant`, migration
  0032) : SP95-E10, SP95, SP98, Gazole, E85, GPL, ou non précisé. Proposé
  d'après le dernier plein ; affiché dans la liste ; rempli par le scan ;
  ignoré pour une ligne d'entretien ou une valeur hors liste. La reprise
  Fuelio ne le remplit pas (codes de carburant de Fuelio non publiés).

## 19. Budget perso, compte commun ; abonnements et projets privés (v0.7.0)
- **Deux budgets** (choix de l'utilisateur) : le compte commun (tous les
  parents, comme avant) et un budget perso par parent, **privé** (lui
  seul le voit, comme une note privée). Seulement dépenses et revenus (pas
  de solde bancaire). Colonne `perso_de` (0 : commun ; sinon le membre) sur
  `operations`, `recurrences`, `budget_plafonds` (clé refaite) et
  `projets` (migration 0033). Les données d'avant restent au commun.
- **Choix du budget** : onglets « 👨‍👩‍👧 Compte commun | 🔒 Mon budget » ;
  `?compte=commun|perso` dans les adresses et les formulaires, retenu par
  un cookie (`fc_compte`). Chaque budget a ses opérations, ses opérations
  qui reviennent, ses plafonds, son année, sa recherche. Une opération ou
  une récurrence d'un autre budget perso : 404. La recherche globale et
  le résumé de l'accueil comptent le commun et le perso du membre.
- **Virements** perso → commun (`POST /budget/virement`) : deux opérations
  liées (`virement_id`) : une dépense « Virement vers le commun » (perso),
  un revenu « Virement de Marie » (commun), catégories `virement` et
  `virement-recu` jamais proposées dans les formulaires. Une fois ou qui
  revient (récurrence `virement = 1` : chaque occurrence écrit les deux
  moitiés). Modifier une moitié : l'autre suit (montant, date) ; supprimer
  : les deux partent. Côté commun, les virements à venir sont prévus.
- **Carnet** (véhicules, animaux, prestataires, projets) : « Ajouter aussi
  au budget » devient un choix : Non (par défaut) / Compte commun / Mon
  budget perso.
- **Abonnement privé** (case « Privé ») : sa récurrence est dans le budget
  perso ; vu par son propriétaire seul (liste, fiche, rappels). Passé en
  privé ou rendu au commun : ses prélèvements déjà écrits suivent.
- **Projet privé** (case « Privé », à la création ; peut être partagé
  ensuite, pas l'inverse : les projets n'ont pas d'auteur noté) : vu par
  son créateur seul (liste, fiche, étapes, carnet, photos, recherche,
  rappels) ; son carnet propose d'abord le budget perso. Contrôle commun
  au carnet : `suivi.Objet.Voit`.
- **Rappels privés** : `Echeance.Pour` (membre). « À venir », l'agenda et
  la notification du matin ne les montrent qu'à lui ; il reçoit alors sa
  propre notification (avec les rappels communs), les autres la commune.
- **Départ d'un membre** : son budget perso (opérations, récurrences,
  plafonds), ses abonnements privés et ses projets privés (carnet et
  photos compris) sont effacés ; les virements reçus au commun restent.
  Crochet `App.AjouterDepart` pour les modules.
- **Export** : le budget perso, les abonnements et les projets privés des
  autres membres n'y sont pas.

## 20. Fiche véhicule repensée, photos dans le texte des notes (v0.8.0)
- **Haut de la fiche d'un véhicule** : résumé (compteur, modifiable d'un
  toucher ; prochaine échéance ; dépensé dans l'année ; coût tout compris
  au km) et deux boutons, « ⛽ Ajouter un plein » et « 🔧 Entretien », qui
  ouvrent une fenêtre (`<dialog>`) par-dessus la fiche, quel que soit
  l'onglet.
- **Fenêtre « Nouveau plein »** : scan du ticket en premier ; grands champs
  litres et prix ; prix au litre calculé ; compteur comparé au dernier
  relevé (alerte si plus petit, ou plus de 3 000 km d'écart) ; carburant
  et plein complet/partiel en boutons ; stations proposées en un toucher
  (les 4 plus fréquentes des 30 derniers pleins, notes de 40 caractères
  au plus) ; date du jour ; budget au choix.
- **Raccourci** `GET /vehicules/plein` (page « Ajouter », appui long sur
  l'icône de l'appli : `shortcuts` du manifeste) : fiche du véhicule du
  dernier plein (ou le premier), fenêtre ouverte (`?ajout=plein`, retiré
  de l'adresse ensuite) ; les autres véhicules en lien dans la fenêtre.
- **Trois onglets** : Entretien, Pleins (liste compacte : une ligne par
  plein avec sa consommation ; touchée, elle s'ouvre : ticket, corriger,
  supprimer), Chiffres (`?onglet=chiffres`).
- **Périodes des chiffres** (`&periode=`) : `3m`, `6m`, 12 mois (vide),
  une année (`2025` : du 1er janvier au 31 décembre, ou à aujourd'hui),
  `tout`, `dates` (`&du=&au=`, remises dans l'ordre, pas après
  aujourd'hui). Comparaison avec la période d'avant (carburant, dépensé),
  seulement si le carnet la couvre en entier.
- **Fuelio** : `Full = 2` (partiel dont Fuelio estime le niveau) repris
  comme partiel ; un plein déjà repris sous l'autre libellé est corrigé,
  pas doublé. Le message de fin dit ce qui n'a pas été repris (aucune
  dépense dans la sauvegarde, dates illisibles, modèles, recettes) ; dates
  aussi en JJ/MM/AAAA et JJ.MM.AAAA.
- **Litres au centilitre** à l'affichage (8,51) : « Corriger » n'arrondit
  plus.
- **Photos dans le texte d'une note** : une ligne `[photo N]` (N : photo de
  cette note) montre la photo à cet endroit. Un éditeur (app.js,
  `contenteditable`) montre la photo pendant la saisie et écrit le texte
  dans le textarea caché (sans JavaScript : le textarea tel quel).
  « 📷 Photo ici » envoie la photo (`POST /notes/{id}/photos?place=1` →
  `{"id":N}`) et la pose au curseur ; « ☑️ Case » ; Entrée continue une
  liste ; une image collée devient une photo. Sur une note neuve, la note
  est d'abord enregistrée. Une photo retirée du texte retourne en bas de
  la note (rien n'est effacé sans son ✕) ; « ⤴️ Dans le texte » la replace.
  À l'enregistrement, les repères d'une photo d'une autre note sont
  retirés. Une note avec des cases se lit en entier (texte, photos,
  cases à cocher). Carte de la liste : 📷 à la place du repère.

## 21. Éditeur de texte riche, graphiques interactifs (v0.9.0)
- **Format** (`internal/riche`) : le texte reste du texte, une ligne par
  ligne, avec des signes proches de Markdown : `# / ## / ###` titres,
  `> ` citation, `- ` puce, `1. ` numéro, `[ ] / [x]` case, `---`
  séparation, `[photo N]` photo (notes seulement), `**gras**`,
  `_italique_`, `++souligné++`, `~~barré~~`, `` `code` ``,
  `[texte](https://…)` et adresses http(s) cliquables ; `\` garde un
  signe tel quel. Rendu par le serveur, tout le texte échappé ; liens
  http(s) et mailto seulement (`target=_blank`, `noopener`). Hors des
  notes : cases figées, repères de photo laissés en texte.
- **Éditeur** (app.js, `contenteditable`, sans bibliothèque) : le contenu
  mis en forme vient du serveur (`<template data-editeur-contenu>`) ; à
  chaque frappe, le texte simple est réécrit dans le textarea caché (sans
  JavaScript : le textarea tel quel). Barre d'outils fixe (sous l'en-tête) :
  annuler, refaire, gras, italique, souligné, barré, code, case, puces,
  numéros, titres, citation, lien, photo (notes), séparation. Cases
  dessinées en CSS (rien de figé dans le texte : tout se sélectionne et
  s'efface), cochées d'un toucher à gauche. Lien touché : « Ouvrir le
  lien ».
- **Coller** : une image (capture, « Copier l'image ») est réduite, envoyée
  et posée au curseur (notes) ; un texte copié d'une page garde gras,
  italique, titres, listes, citations et liens (le reste est retiré :
  balises, styles, scripts) ; glisser une image marche aussi.
- **Partout où il y a un texte long** : notes ; description d'un projet ;
  note d'un événement ; notes d'une carte de fidélité, d'un contact (et
  idées de cadeaux), d'un animal, d'une garantie, d'un véhicule, d'un
  abonnement. Pas pour les ingrédients ni l'adresse (lus ligne à ligne).
  vCard : notes sans les signes.
- **Graphiques interactifs** : chaque point et chaque barre porte sa date
  et sa valeur (`data-info`) ; un toucher (ou la souris, ou le doigt qui
  glisse) montre un repère, le point choisi et une bulle. La courbe se
  trace et les barres montent à l'affichage (sauf « réduire les
  animations »). Période des chiffres changée sans recharger la page
  (htmx, l'adresse suit).

## 22. Listes (v0.9.3)
- (v0.9.6 : onglet « ☑️ Listes » et bouton « ＋ Liste » retirés, remplacés
  par les catégories, § 24. Les cases restent dans toutes les notes.)
- Avancement « 2/5 faits » et barre (classes `p0` à `p100` : pas de style
  écrit dans la page).
- Cochés en bas (réglage de la note, migration 0034 `cochees_en_bas`, oui
  par défaut) : à l'affichage seulement, dans chaque suite de cases ; le
  texte ne bouge pas (décocher remet l'élément à sa place).
- « ✏️ Ranger » (ceux qui peuvent modifier la note) : chaque élément devient
  un champ (✓, ↑, ↓, ✕) ; chaque action envoie la ligne telle qu'elle était
  (`avant`) : si la note a changé entre-temps, rien n'est touché. Un
  élément modifié est enregistré en texte simple (signes gardés tels quels).
  Adresses `POST /notes/elements/{id}/{ligne}[/monter|/descendre|/supprimer]`,
  `GET /notes/liste/{id}?ranger=1` (pas `/notes/{id}/…` : conflit avec
  `/notes/photos/{id}`).
- Rappel d'une note (`rappel_le`, `rappel_envoye`) : date et heure (fuseau
  de la famille, 2 ans au plus) ; une tâche de chaque minute
  (`App.AjouterMinute`) envoie la notification une seule fois, même aux
  membres qui ont l'appli ouverte (`App.Rappeler`) ; note privée : son
  auteur seul. Montré sur la carte (⏰ 28/11 18:00).

## 23. Anciens véhicules, import Fuelio de tous les véhicules (v0.9.3)
- **Ancien véhicule** (migration 0035, `vehicules.vendu_le`, date ; vide :
  toujours là) : case « Je ne l'ai plus » et date dans le formulaire.
  Historique complet gardé (pleins, entretien, chiffres). Liste : partie
  « 🏁 Anciens véhicules » en bas, grisée, sans alerte. Fiche : bandeau.
  Plus de rappels ni d'échéances dans « À venir », le calendrier et la
  notification du matin (`NomsVehicules` ne renvoie que les véhicules
  encore là : les échéances des autres sont écartées) ; pas proposé dans
  « Ajouter un plein » (raccourci, autres véhicules) ; pas compté sur
  l'accueil.
- **Import de tous les véhicules** (`POST /vehicules/fuelio`, parents,
  depuis la liste) : chaque véhicule de la sauvegarde va dans celui du
  même nom (accents et majuscules à part), créé s'il manque avec
  l'immatriculation, la marque, le modèle et l'année de Fuelio. Un véhicule
  créé dont le dernier plein a plus d'un an est rangé dans les anciens,
  depuis la date de ce plein. Même reprise que sur une fiche (sans
  doublon, photos, compteur) ; bilan par véhicule.

## 24. Catégories de notes (v0.9.6)
- Table `categories_notes` (migration 0036 : nom, emoji) ; `notes.categorie_id`
  (une seule catégorie par note, vide permis ; catégorie supprimée : ses
  notes restent, sans catégorie). Aucune au départ ; 50 au plus par famille.
- Tout le monde (enfants compris) crée, renomme et supprime les catégories :
  page `/notes/categories` (« ⚙️ Catégories »).
- Emoji choisi dans une grille de 48 (`emojisCategories`, ouverte par le
  bouton du champ, app.js) ; « Autre » pour en écrire un, « Aucun ». Sur
  une catégorie existante, enregistré dès le choix.
- Même grille (gabarits communs `champ-emoji` et `grille-emojis`) pour les
  projets, les abonnements et les récompenses des enfants : 24 emojis
  proposés chacun, « Autre » permis. Lecture commune `appli.LireEmoji`
  (8 caractères au plus, ni lettre, ni chiffre, ni signe ASCII).
- Filtres de la liste : Toutes, chaque catégorie (avec son nombre de notes),
  Sans catégorie, 🔒 Privées. Filtre « Partagées » retiré.
  Adresse : `/notes?cat=ID` ou `/notes?cat=sans`.
- Formulaire de note : choix de la catégorie (préchoisie depuis le filtre) ;
  une catégorie d'une autre famille compte comme « aucune ».
- Carte : étiquette de la catégorie. Export JSON : `notes_categories`.

## 25. Notes « parents seulement » (v0.9.8)
- Migration 0037 : `notes.parents` et `categories_notes.parents` (0/1).
- Une note est cachée aux enfants si elle est « parents seulement » ou si
  sa catégorie l'est ; son auteur la voit toujours (même un enfant). Une
  note privée reste vue par son seul auteur.
- Seul un parent coche « 👥 Parents seulement » (note ou catégorie ; sur la
  page Catégories : pastille « 👥 Parents seulement / 👪 Toute la famille »).
  Un enfant ne voit pas ces catégories, ne peut ni s'en servir, ni les
  renommer, ni les supprimer ; sa note rangée là par un parent y reste.
- Partout : liste, recherche, compte de l'accueil, adresse d'une note ou
  d'une photo (`Module.visible`), annonce « X a écrit la note » et rappel
  (parents et auteur seulement), nombre de notes par catégorie.

## 26. Tâches : rappel et catégories (v0.9.9)
- Migration 0038 : `categories_taches` (comme celles des notes, à part ;
  gérées par tous, grille d'emojis), `taches.categorie_id`, `taches.rappel`
  (« HH:MM », heure de la famille), `taches.date_rappel` (tâche « une
  fois »), `taches_rappels` (un rappel par tâche et par jour au plus).
- Liste : second filtre « catégories » (`?cat=ID` ou `sans`), gardé avec
  celui des membres ; étiquette et « ⏰ 18:00 » sur chaque tâche.
- Rappel (tâche de chaque minute, `envoyerRappels`) : chaque jour où la
  tâche revient, si elle n'est pas faite ; semaine, quinzaine, mois : le
  premier jour de la période ; « une fois » : le jour choisi, sinon chaque
  jour jusqu'à ce qu'elle soit faite. Envoyé à celui qui doit la faire
  (membre choisi, ou celui dont c'est le tour ; tout le monde sinon).
  Rattrapé si le serveur revient moins de 15 min après l'heure ; rien pour
  une tâche créée après l'heure. Traces gardées 60 jours.
- Export JSON : `taches_categories`.

