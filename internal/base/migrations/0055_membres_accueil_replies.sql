-- Blocs de l'accueil replies par le membre (« a-venir,taches » ; vide :
-- tous ouverts). Garde d'un appareil a l'autre.
ALTER TABLE membres ADD COLUMN accueil_replies TEXT NOT NULL DEFAULT '';
