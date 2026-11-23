-- Ancien vehicule (vendu, donne...) : date AAAA-MM-JJ depuis laquelle il
-- n'est plus a la famille ; vide : toujours la. Son historique reste ; plus
-- de rappels pour lui.
ALTER TABLE vehicules ADD COLUMN vendu_le TEXT NOT NULL DEFAULT '';
