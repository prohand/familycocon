-- Agenda : plus de répétitions (chaque jour, certains jours, toutes les
-- 2 semaines, chaque mois…) et une date de fin de répétition.
-- repetition : '', 'jour', 'jours:1,3' (1 = lundi), 'semaine', '2semaines',
-- 'mois' (même date), 'mois-rang' (ex. 2e mardi), 'an'.
ALTER TABLE evenements ADD COLUMN repetition_fin TEXT NOT NULL DEFAULT '';  -- 2027-06-30 ou ''
