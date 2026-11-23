-- Pieces jointes du carnet (ticket, facture) : photos gardees sur le disque,
-- dans le dossier des donnees (pieces/<famille>/<fichier>.jpg). La base ne
-- garde que leur nom. Supprimees avec la ligne du carnet.
CREATE TABLE pieces (
  id         INTEGER PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  carnet_id  INTEGER NOT NULL REFERENCES carnet(id) ON DELETE CASCADE,
  fichier    TEXT    NOT NULL UNIQUE,
  taille     INTEGER NOT NULL,
  cree_par   INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  cree_le    INTEGER NOT NULL
);
CREATE INDEX pieces_carnet ON pieces(carnet_id);
CREATE INDEX pieces_famille ON pieces(famille_id);
