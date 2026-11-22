-- Projets de la famille (vacances, travaux, déménagement…) : étapes à
-- cocher, date d'objectif, budget prévu. Les dépenses vont dans le carnet
-- commun (module 'projets').

CREATE TABLE projets (
  id          INTEGER PRIMARY KEY,
  famille_id  INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  nom         TEXT    NOT NULL,
  emoji       TEXT    NOT NULL DEFAULT '🎯',
  couleur     INTEGER NOT NULL DEFAULT 1,
  description TEXT    NOT NULL DEFAULT '',
  objectif    TEXT    NOT NULL DEFAULT '',  -- date visée (2027-07-01) ou ''
  budget      INTEGER NOT NULL DEFAULT 0,   -- centimes
  termine_le  INTEGER NOT NULL DEFAULT 0,   -- 0 : en cours
  cree_le     INTEGER NOT NULL
);
CREATE INDEX projets_famille ON projets(famille_id);

CREATE TABLE projet_etapes (
  id         INTEGER PRIMARY KEY,
  projet_id  INTEGER NOT NULL REFERENCES projets(id) ON DELETE CASCADE,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  texte      TEXT    NOT NULL,
  jour       TEXT    NOT NULL DEFAULT '',   -- à faire pour le…
  membre_id  INTEGER REFERENCES membres(id) ON DELETE SET NULL,  -- qui s'en occupe
  faite_le   INTEGER NOT NULL DEFAULT 0,
  faite_par  INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  cree_le    INTEGER NOT NULL
);
CREATE INDEX projet_etapes_projet ON projet_etapes(projet_id);
CREATE INDEX projet_etapes_jour ON projet_etapes(famille_id, jour);

UPDATE familles SET modules = CASE WHEN modules = '' THEN 'projets' ELSE modules || ',projets' END;
