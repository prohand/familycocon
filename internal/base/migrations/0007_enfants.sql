-- Espace enfants : missions (tâches à étoiles), validation par un parent,
-- récompenses échangées contre des étoiles.
-- Solde d'un enfant = étoiles des missions validées − prix des échanges confirmés.

CREATE TABLE missions (
  id         INTEGER PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  enfant_id  INTEGER NOT NULL REFERENCES membres(id) ON DELETE CASCADE,
  titre      TEXT    NOT NULL,
  emoji      TEXT    NOT NULL,
  etoiles    INTEGER NOT NULL CHECK (etoiles BETWEEN 1 AND 5),
  categorie  TEXT    NOT NULL DEFAULT 'menage',  -- menage | devoirs | routine | autre
  repetition TEXT    NOT NULL DEFAULT '',
  active     INTEGER NOT NULL DEFAULT 1,
  cree_par   INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  cree_le    INTEGER NOT NULL
);
CREATE INDEX missions_famille ON missions(famille_id, enfant_id, active);

CREATE TABLE missions_faites (
  id         INTEGER PRIMARY KEY,
  mission_id INTEGER NOT NULL REFERENCES missions(id) ON DELETE CASCADE,
  occurrence TEXT    NOT NULL,
  etat       TEXT    NOT NULL CHECK (etat IN ('attente', 'validee', 'refusee')),
  etoiles    INTEGER NOT NULL,           -- étoiles gagnées (copie : la mission peut changer)
  coche_le   INTEGER NOT NULL,
  decide_par INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  decide_le  INTEGER,
  UNIQUE (mission_id, occurrence)
);

CREATE TABLE recompenses (
  id         INTEGER PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  titre      TEXT    NOT NULL,
  emoji      TEXT    NOT NULL,
  prix       INTEGER NOT NULL CHECK (prix > 0),
  active     INTEGER NOT NULL DEFAULT 1,
  cree_le    INTEGER NOT NULL
);

CREATE TABLE echanges (
  id            INTEGER PRIMARY KEY,
  recompense_id INTEGER NOT NULL REFERENCES recompenses(id) ON DELETE CASCADE,
  enfant_id     INTEGER NOT NULL REFERENCES membres(id) ON DELETE CASCADE,
  prix          INTEGER NOT NULL,
  etat          TEXT    NOT NULL CHECK (etat IN ('attente', 'confirme', 'refuse')),
  demande_le    INTEGER NOT NULL,
  decide_par    INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  decide_le     INTEGER
);
CREATE INDEX echanges_enfant ON echanges(enfant_id, etat);
