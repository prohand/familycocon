-- Type de carburant d'un plein (SP95, SP95-E10, SP98, Gazole, E85, GPL) ;
-- vide : non precise (lignes d'entretien, anciens pleins).
ALTER TABLE carnet ADD COLUMN carburant TEXT NOT NULL DEFAULT '';
