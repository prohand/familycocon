-- Menus de la semaine : un plat par créneau (midi, soir), avec ses
-- ingrédients ; les plats déjà faits sont retenus comme recettes.

CREATE TABLE repas (
  id           INTEGER PRIMARY KEY,
  famille_id   INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  jour         TEXT    NOT NULL,             -- 2026-11-22
  moment       TEXT    NOT NULL CHECK (moment IN ('midi', 'soir')),
  nom          TEXT    NOT NULL,
  ingredients  TEXT    NOT NULL DEFAULT '',  -- un par ligne
  dans_courses INTEGER NOT NULL DEFAULT 0,   -- ingrédients déjà ajoutés aux courses
  cree_par     INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  cree_le      INTEGER NOT NULL,
  UNIQUE (famille_id, jour, moment)
);

CREATE TABLE recettes (
  famille_id  INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  cle         TEXT    NOT NULL,
  nom         TEXT    NOT NULL,
  ingredients TEXT    NOT NULL DEFAULT '',
  compte      INTEGER NOT NULL DEFAULT 1,
  vu_le       INTEGER NOT NULL,
  PRIMARY KEY (famille_id, cle)
);
