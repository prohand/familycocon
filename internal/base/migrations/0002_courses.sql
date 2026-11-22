-- Courses : listes partagées, produits rangés par rayon, produits connus
-- de la famille (rayon retenu, suggestions).

CREATE TABLE listes (
  id         INTEGER PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  nom        TEXT    NOT NULL,
  cree_le    INTEGER NOT NULL
);
CREATE INDEX listes_famille ON listes(famille_id);

CREATE TABLE produits (
  id         INTEGER PRIMARY KEY,
  liste_id   INTEGER NOT NULL REFERENCES listes(id) ON DELETE CASCADE,
  nom        TEXT    NOT NULL,
  quantite   TEXT    NOT NULL DEFAULT '',
  rayon      TEXT    NOT NULL,
  emoji      TEXT    NOT NULL,
  pris       INTEGER NOT NULL DEFAULT 0,
  ajoute_par INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  pris_par   INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  cree_le    INTEGER NOT NULL,
  pris_le    INTEGER
);
CREATE INDEX produits_liste ON produits(liste_id, pris);

-- Ce que la famille achète souvent : le rayon choisi est retenu.
CREATE TABLE produits_connus (
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  cle        TEXT    NOT NULL,
  nom        TEXT    NOT NULL,
  rayon      TEXT    NOT NULL,
  emoji      TEXT    NOT NULL,
  compte     INTEGER NOT NULL DEFAULT 1,
  vu_le      INTEGER NOT NULL,
  PRIMARY KEY (famille_id, cle)
);
