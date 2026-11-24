-- Taches : categories (comme celles des notes, propres aux taches) et rappel.
CREATE TABLE categories_taches (
  id         INTEGER PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  nom        TEXT    NOT NULL,
  emoji      TEXT    NOT NULL DEFAULT '',
  cree_le    INTEGER NOT NULL
);
CREATE INDEX categories_taches_famille ON categories_taches(famille_id);
ALTER TABLE taches ADD COLUMN categorie_id INTEGER REFERENCES categories_taches(id) ON DELETE SET NULL;
CREATE INDEX taches_categorie ON taches(categorie_id);

-- Rappel : heure (HH:MM, heure de la famille) ; '' : aucun. Envoye chaque
-- jour ou la tache revient, si elle n'est pas faite. date_rappel : pour une
-- tache " une fois ", le jour (AAAA-MM-JJ) ; '' : chaque jour jusqu'a ce
-- qu'elle soit faite.
ALTER TABLE taches ADD COLUMN rappel TEXT NOT NULL DEFAULT '';
ALTER TABLE taches ADD COLUMN date_rappel TEXT NOT NULL DEFAULT '';
CREATE INDEX taches_rappel ON taches(rappel) WHERE rappel <> '';

-- Rappels deja envoyes (un par tache et par jour : jamais deux fois).
CREATE TABLE taches_rappels (
  tache_id  INTEGER NOT NULL REFERENCES taches(id) ON DELETE CASCADE,
  jour      TEXT    NOT NULL,
  envoye_le INTEGER NOT NULL,
  PRIMARY KEY (tache_id, jour)
);
