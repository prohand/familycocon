-- Contacts de la famille (modules Adresses et Anniversaires), date de
-- naissance des membres, notifications de rappel.

CREATE TABLE contacts (
  id         INTEGER PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  nom        TEXT    NOT NULL,
  relation   TEXT    NOT NULL DEFAULT 'autre',  -- famille, ami, voisin, ecole, travail, autre
  -- Date de naissance : 2018-03-05, ou 0000-03-05 si l'année est inconnue ;
  -- '' : pas d'anniversaire.
  naissance  TEXT    NOT NULL DEFAULT '',
  telephone  TEXT    NOT NULL DEFAULT '',
  email      TEXT    NOT NULL DEFAULT '',
  adresse    TEXT    NOT NULL DEFAULT '',
  notes      TEXT    NOT NULL DEFAULT '',
  idees      TEXT    NOT NULL DEFAULT '',     -- idées de cadeaux
  cree_le    INTEGER NOT NULL,
  modifie_le INTEGER NOT NULL
);
CREATE INDEX contacts_famille ON contacts(famille_id);

ALTER TABLE membres ADD COLUMN naissance TEXT NOT NULL DEFAULT '';

-- Rappels (anniversaires, échéances) : notifiés par défaut.
UPDATE membres SET push_modules = CASE WHEN push_modules = '' THEN 'rappels' ELSE push_modules || ',rappels' END;

UPDATE familles SET modules = CASE WHEN modules = '' THEN 'anniversaires,adresses' ELSE modules || ',anniversaires,adresses' END;
