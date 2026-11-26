-- Preparation facultative d'une recette (texte mis en forme, voir internal/riche).
ALTER TABLE recettes ADD COLUMN preparation TEXT NOT NULL DEFAULT '';
