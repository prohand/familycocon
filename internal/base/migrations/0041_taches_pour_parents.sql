-- Tache confiee aux parents (l'un ou l'autre la fait) : 1. assigne_a reste
-- vide et tour aussi.
ALTER TABLE taches ADD COLUMN pour_parents INTEGER NOT NULL DEFAULT 0;
