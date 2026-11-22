-- Notes de la famille : partagées (toute la famille) ou privées (leur
-- auteur seulement). Le module est activé pour les familles existantes.

CREATE TABLE notes (
  id         INTEGER PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  auteur_id  INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  titre      TEXT    NOT NULL DEFAULT '',
  texte      TEXT    NOT NULL DEFAULT '',
  couleur    INTEGER NOT NULL DEFAULT 1 CHECK (couleur BETWEEN 1 AND 6),
  epinglee   INTEGER NOT NULL DEFAULT 0,
  privee     INTEGER NOT NULL DEFAULT 0,
  cree_le    INTEGER NOT NULL,
  modifie_le INTEGER NOT NULL
);
CREATE INDEX notes_famille ON notes(famille_id, privee);

UPDATE familles SET modules = CASE WHEN modules = '' THEN 'notes' ELSE modules || ',notes' END;
