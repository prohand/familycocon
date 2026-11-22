-- Cartes de fidélité : numéro et code-barres à montrer en caisse.

CREATE TABLE cartes (
  id          INTEGER PRIMARY KEY,
  famille_id  INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  enseigne    TEXT    NOT NULL,
  numero      TEXT    NOT NULL,
  format      TEXT    NOT NULL DEFAULT 'auto',  -- auto, ean13, code128, qr, aucun
  couleur     INTEGER NOT NULL DEFAULT 1 CHECK (couleur BETWEEN 1 AND 8),
  titulaire   TEXT    NOT NULL DEFAULT '',
  notes       TEXT    NOT NULL DEFAULT '',
  cree_le     INTEGER NOT NULL,
  utilisee_le INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX cartes_famille ON cartes(famille_id);

UPDATE familles SET modules = CASE WHEN modules = '' THEN 'fidelite' ELSE modules || ',fidelite' END;
