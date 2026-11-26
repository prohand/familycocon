-- Agenda : rappel juste avant un evenement, en plus du rappel du matin.
-- rappel_avant : '' (aucun), '15min', '1h', 'veille' (la veille a 18 h,
-- heure de la famille). Journee entiere : 'veille' seulement.
ALTER TABLE evenements ADD COLUMN rappel_avant TEXT NOT NULL DEFAULT '';
CREATE INDEX evenements_rappel_avant ON evenements(famille_id) WHERE rappel_avant <> '';

-- Rappels "juste avant" deja envoyes : un par fois de l'evenement (debut :
-- heure de debut de cette fois, en secondes). Garde en base : jamais deux
-- fois, meme apres un redemarrage du serveur.
CREATE TABLE evenements_rappels (
  evenement_id INTEGER NOT NULL REFERENCES evenements(id) ON DELETE CASCADE,
  debut        INTEGER NOT NULL,
  envoye_le    INTEGER NOT NULL,
  PRIMARY KEY (evenement_id, debut)
);
