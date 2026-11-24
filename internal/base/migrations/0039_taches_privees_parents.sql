-- Taches privees (vues par leur createur seul) et reservees aux parents
-- (la tache ou sa categorie : cachees aux enfants, sauf a qui l'a creee ou
-- doit la faire).
ALTER TABLE taches ADD COLUMN privee INTEGER NOT NULL DEFAULT 0;
ALTER TABLE taches ADD COLUMN parents INTEGER NOT NULL DEFAULT 0;
ALTER TABLE categories_taches ADD COLUMN parents INTEGER NOT NULL DEFAULT 0;
