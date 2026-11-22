-- Comptes : familles, membres, sessions, invitations, appareils de confiance.
-- Dates : secondes Unix (UTC). Empreintes : SHA-256 des jetons (le jeton
-- lui-même n'est jamais gardé en base).

CREATE TABLE familles (
  id       INTEGER PRIMARY KEY,
  nom      TEXT    NOT NULL,
  fuseau   TEXT    NOT NULL DEFAULT 'Europe/Paris',
  devise   TEXT    NOT NULL DEFAULT 'EUR',
  -- Modules activés, séparés par des virgules (réglages de la famille).
  modules  TEXT    NOT NULL DEFAULT 'courses,agenda,taches,menus,budget,enfants',
  cree_le  INTEGER NOT NULL
);

CREATE TABLE membres (
  id          INTEGER PRIMARY KEY,
  famille_id  INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  nom         TEXT    NOT NULL,
  role        TEXT    NOT NULL CHECK (role IN ('parent', 'enfant')),
  couleur     INTEGER NOT NULL DEFAULT 1 CHECK (couleur BETWEEN 1 AND 8),
  -- Connexion par identifiant + mot de passe (parents, ados) ;
  -- NULL pour un enfant qui se connecte seulement par code PIN.
  identifiant TEXT    UNIQUE COLLATE NOCASE,
  mdp         TEXT,
  -- Empreinte du code PIN (enfants).
  pin         TEXT,
  -- Raccourcis de la barre du bas (4 cases autour du « + »).
  raccourcis  TEXT    NOT NULL DEFAULT 'accueil,agenda,courses,moi',
  actif       INTEGER NOT NULL DEFAULT 1,
  cree_le     INTEGER NOT NULL
);
CREATE INDEX membres_famille ON membres(famille_id);

CREATE TABLE sessions (
  empreinte  BLOB    PRIMARY KEY,
  membre_id  INTEGER NOT NULL REFERENCES membres(id) ON DELETE CASCADE,
  jeton_csrf TEXT    NOT NULL,
  cree_le    INTEGER NOT NULL,
  vue_le     INTEGER NOT NULL,
  expire_le  INTEGER NOT NULL
) WITHOUT ROWID;
CREATE INDEX sessions_membre ON sessions(membre_id);

-- Lien d'invitation : à usage unique, valable 7 jours.
CREATE TABLE invitations (
  empreinte  BLOB    PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  role       TEXT    NOT NULL CHECK (role IN ('parent', 'enfant')),
  cree_par   INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  cree_le    INTEGER NOT NULL,
  expire_le  INTEGER NOT NULL
) WITHOUT ROWID;

-- Appareil de confiance : sur cet appareil, les enfants de la famille
-- se connectent en touchant leur avatar puis en tapant leur code PIN.
CREATE TABLE appareils (
  empreinte  BLOB    PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  cree_le    INTEGER NOT NULL,
  vue_le     INTEGER NOT NULL
) WITHOUT ROWID;
