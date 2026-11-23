-- Membres deja retires de la famille (actif = 0) : leurs taches reviennent a
-- tout le monde, et leurs notes privees (que personne d'autre ne peut voir
-- ni supprimer) sont effacees. Le retrait d'un membre le fait desormais
-- lui-meme ; cette migration rattrape les retraits passes.

UPDATE taches SET assigne_a = NULL
WHERE assigne_a IN (SELECT id FROM membres WHERE actif = 0);

DELETE FROM notes
WHERE privee = 1 AND auteur_id IN (SELECT id FROM membres WHERE actif = 0);
