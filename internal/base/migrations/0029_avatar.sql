-- Avatar d'un membre : un emoji choisi dans une liste (animal, visage...).
-- '' : l'initiale du prenom, comme avant. Aucune photo n'est envoyee.
ALTER TABLE membres ADD COLUMN avatar TEXT NOT NULL DEFAULT '';
