-- Categories des notes : celles de la famille (nom et emoji), creees par
-- tout le monde. Une note en a une ou aucune ; une categorie supprimee
-- laisse ses notes sans categorie.
CREATE TABLE categories_notes (
  id         INTEGER PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  nom        TEXT    NOT NULL,
  emoji      TEXT    NOT NULL DEFAULT '',
  cree_le    INTEGER NOT NULL
);
CREATE INDEX categories_notes_famille ON categories_notes(famille_id);
ALTER TABLE notes ADD COLUMN categorie_id INTEGER REFERENCES categories_notes(id) ON DELETE SET NULL;
CREATE INDEX notes_categorie ON notes(categorie_id);
