-- Vehicule : ou vont ses depenses du carnet (pleins, entretiens) dans le budget.
--   budget = '' (nulle part), 'commun' (compte commun) ou 'perso' (budget
--   perso du parent budget_perso_de). Le reglage vaut aussi pour les lignes
--   deja notees et pour la reprise Fuelio.
ALTER TABLE vehicules ADD COLUMN budget TEXT NOT NULL DEFAULT '';
ALTER TABLE vehicules ADD COLUMN budget_perso_de INTEGER NOT NULL DEFAULT 0;
