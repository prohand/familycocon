-- Listes : cases cochees rangees en bas a l'affichage (1 par defaut) ;
-- rappel a une date et heure (unix, 0 : aucun), envoye une seule fois.
ALTER TABLE notes ADD COLUMN cochees_en_bas INTEGER NOT NULL DEFAULT 1;
ALTER TABLE notes ADD COLUMN rappel_le INTEGER NOT NULL DEFAULT 0;
ALTER TABLE notes ADD COLUMN rappel_envoye INTEGER NOT NULL DEFAULT 0;
CREATE INDEX notes_rappel ON notes(rappel_le) WHERE rappel_le > 0 AND rappel_envoye = 0;
