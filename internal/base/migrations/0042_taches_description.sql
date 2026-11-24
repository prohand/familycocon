-- Description facultative d'une tache (texte mis en forme, voir internal/riche).
ALTER TABLE taches ADD COLUMN description TEXT NOT NULL DEFAULT '';
