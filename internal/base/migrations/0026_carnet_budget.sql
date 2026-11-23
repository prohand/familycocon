-- Carnet (vehicules, animaux, prestataires, projets) :
--   - operation_id : la depense ajoutee aussi au budget (supprimees ensemble) ;
--   - litres : carburant d'un plein, en centilitres (consommation du vehicule).
ALTER TABLE carnet ADD COLUMN operation_id INTEGER REFERENCES operations(id) ON DELETE SET NULL;
ALTER TABLE carnet ADD COLUMN litres INTEGER NOT NULL DEFAULT 0;
