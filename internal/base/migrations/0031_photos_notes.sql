-- Photos des notes : gardees sur le disque comme les pieces du carnet
-- (pieces/<famille>/<fichier>.jpg). La base ne garde que leur nom.
-- Supprimees avec la note.
CREATE TABLE photos_notes (
  id         INTEGER PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  note_id    INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
  fichier    TEXT    NOT NULL UNIQUE,
  taille     INTEGER NOT NULL,
  cree_par   INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  cree_le    INTEGER NOT NULL
);
CREATE INDEX photos_notes_note ON photos_notes(note_id);
CREATE INDEX photos_notes_famille ON photos_notes(famille_id);
