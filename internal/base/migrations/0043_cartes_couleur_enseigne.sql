-- Cartes de fidelite : 1 = aux couleurs de l'enseigne quand elle est connue
-- (trouvee d'apres son nom, voir internal/fidelite/enseignes.go) ; 0 = la
-- couleur choisie (couleur).
ALTER TABLE cartes ADD COLUMN couleur_enseigne INTEGER NOT NULL DEFAULT 1;
