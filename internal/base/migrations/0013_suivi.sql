-- Suivi commun aux modules Véhicules, Animaux et Prestataires :
--   - échéances (contrôle technique, vaccin, ramonage…), qui peuvent revenir
--     tous les N mois ;
--   - carnet : ce qui a été fait (date, coût, kilométrage, note).

CREATE TABLE echeances (
  id              INTEGER PRIMARY KEY,
  famille_id      INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  module          TEXT    NOT NULL,
  objet_id        INTEGER NOT NULL,
  libelle         TEXT    NOT NULL,
  jour            TEXT    NOT NULL,            -- 2027-03-15
  repetition_mois INTEGER NOT NULL DEFAULT 0,  -- 0 : une fois
  cree_le         INTEGER NOT NULL
);
CREATE INDEX echeances_objet ON echeances(famille_id, module, objet_id);
CREATE INDEX echeances_jour ON echeances(famille_id, jour);

CREATE TABLE carnet (
  id         INTEGER PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  module     TEXT    NOT NULL,
  objet_id   INTEGER NOT NULL,
  jour       TEXT    NOT NULL,
  libelle    TEXT    NOT NULL,
  montant    INTEGER NOT NULL DEFAULT 0,  -- centimes
  km         INTEGER NOT NULL DEFAULT 0,
  note       TEXT    NOT NULL DEFAULT '',
  cree_par   INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  cree_le    INTEGER NOT NULL
);
CREATE INDEX carnet_objet ON carnet(famille_id, module, objet_id, jour);

CREATE TABLE vehicules (
  id              INTEGER PRIMARY KEY,
  famille_id      INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  nom             TEXT    NOT NULL,
  genre           TEXT    NOT NULL DEFAULT 'voiture',  -- voiture, moto, velo, campingcar, autre
  immatriculation TEXT    NOT NULL DEFAULT '',
  modele          TEXT    NOT NULL DEFAULT '',
  kilometrage     INTEGER NOT NULL DEFAULT 0,
  notes           TEXT    NOT NULL DEFAULT '',
  cree_le         INTEGER NOT NULL
);

CREATE TABLE animaux (
  id          INTEGER PRIMARY KEY,
  famille_id  INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  nom         TEXT    NOT NULL,
  espece      TEXT    NOT NULL DEFAULT 'chien',
  race        TEXT    NOT NULL DEFAULT '',
  naissance   TEXT    NOT NULL DEFAULT '',
  puce        TEXT    NOT NULL DEFAULT '',
  veterinaire TEXT    NOT NULL DEFAULT '',
  notes       TEXT    NOT NULL DEFAULT '',
  cree_le     INTEGER NOT NULL
);

CREATE TABLE prestataires (
  id         INTEGER PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  nom        TEXT    NOT NULL,
  metier     TEXT    NOT NULL DEFAULT '',
  telephone  TEXT    NOT NULL DEFAULT '',
  email      TEXT    NOT NULL DEFAULT '',
  adresse    TEXT    NOT NULL DEFAULT '',
  site       TEXT    NOT NULL DEFAULT '',
  notes      TEXT    NOT NULL DEFAULT '',
  cree_le    INTEGER NOT NULL
);

UPDATE familles SET modules = CASE WHEN modules = '' THEN 'vehicules,animaux,prestataires'
  ELSE modules || ',vehicules,animaux,prestataires' END;
