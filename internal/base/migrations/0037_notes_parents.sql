-- Notes et categories reservees aux parents : cachees aux enfants (sauf a
-- l'auteur de la note).
ALTER TABLE notes ADD COLUMN parents INTEGER NOT NULL DEFAULT 0;
ALTER TABLE categories_notes ADD COLUMN parents INTEGER NOT NULL DEFAULT 0;
