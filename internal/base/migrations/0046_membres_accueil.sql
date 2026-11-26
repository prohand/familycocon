-- Accueil de chaque membre : l'ordre des tuiles et les modules masques
-- (« agenda,courses,-animaux » ; « - » : masque ; vide : ordre par defaut).
ALTER TABLE membres ADD COLUMN accueil TEXT NOT NULL DEFAULT '';
