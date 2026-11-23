-- Agenda : un evenement peut etre rappele (bloc « A venir » de l'accueil et
-- notification du matin, le jour meme et 7 jours avant). 0 : pas de rappel.
ALTER TABLE evenements ADD COLUMN rappel INTEGER NOT NULL DEFAULT 0;
