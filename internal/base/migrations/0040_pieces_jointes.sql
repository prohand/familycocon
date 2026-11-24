-- Pieces jointes (documents) des notes, taches, projets et lignes du carnet.
-- Le fichier est garde sur le disque (pieces/<famille>/<nom aleatoire>) ; la
-- base garde son nom d'origine, son type (choisi par le serveur d'apres
-- l'extension) et sa taille. objet_id : la note, la tache, le projet ou la
-- ligne du carnet (module). Une piece dont l'objet n'existe plus est
-- effacee par le menage de chaque heure.
CREATE TABLE pieces_jointes (
  id         INTEGER PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  module     TEXT    NOT NULL,
  objet_id   INTEGER NOT NULL,
  nom        TEXT    NOT NULL,
  type       TEXT    NOT NULL,
  taille     INTEGER NOT NULL,
  fichier    TEXT    NOT NULL,
  cree_par   INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  cree_le    INTEGER NOT NULL
);
CREATE INDEX pieces_jointes_objet ON pieces_jointes(famille_id, module, objet_id);
