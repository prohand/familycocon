-- Agenda partagé : événements, avec les membres concernés (couleur par
-- personne). Aucun membre : l'événement concerne toute la famille.

CREATE TABLE evenements (
  id         INTEGER PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  titre      TEXT    NOT NULL,
  lieu       TEXT    NOT NULL DEFAULT '',
  note       TEXT    NOT NULL DEFAULT '',
  debut      INTEGER NOT NULL,
  fin        INTEGER NOT NULL,
  journee    INTEGER NOT NULL DEFAULT 0,
  -- '' (une fois), 'semaine' (activité), 'an' (anniversaire)
  repetition TEXT    NOT NULL DEFAULT '',
  cree_par   INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  cree_le    INTEGER NOT NULL
);
CREATE INDEX evenements_famille ON evenements(famille_id, debut);

CREATE TABLE evenements_membres (
  evenement_id INTEGER NOT NULL REFERENCES evenements(id) ON DELETE CASCADE,
  membre_id    INTEGER NOT NULL REFERENCES membres(id) ON DELETE CASCADE,
  PRIMARY KEY (evenement_id, membre_id)
);
