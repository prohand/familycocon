-- Budget : operation qui revient a date variable (cantine, facture,
-- remboursement...) : a_confirmer = 1, elle n'est pas ecrite toute seule.
-- Chaque fois, elle attend (comptee dans le reste a vivre du mois) qu'on
-- dise « C'est passe » (vraie date, vrai montant) ou « Pas cette fois ».
ALTER TABLE recurrences ADD COLUMN a_confirmer INTEGER NOT NULL DEFAULT 0;
