-- Reglages de chaque membre :
--   - accueil_jour_haut : les blocs du jour (Aujourd'hui, taches, A venir)
--     au-dessus des tuiles de l'accueil ;
--   - accueil_compact : tuiles plus petites (3 par ligne, sans resume) ;
--   - calme_debut, calme_fin : heures calmes (HH:MM, vides : aucune), sans
--     notification d'activite (les rappels a heure choisie partent quand meme).
ALTER TABLE membres ADD COLUMN accueil_jour_haut INTEGER NOT NULL DEFAULT 0;
ALTER TABLE membres ADD COLUMN accueil_compact INTEGER NOT NULL DEFAULT 0;
ALTER TABLE membres ADD COLUMN calme_debut TEXT NOT NULL DEFAULT '';
ALTER TABLE membres ADD COLUMN calme_fin TEXT NOT NULL DEFAULT '';
