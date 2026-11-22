-- Tâches ménagères : à qui, quand (répétition), et qui a fait quoi.

CREATE TABLE taches (
  id         INTEGER PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  titre      TEXT    NOT NULL,
  emoji      TEXT    NOT NULL,
  -- NULL : n'importe qui peut la faire.
  assigne_a  INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  -- Règle de répétition (voir internal/repetition).
  repetition TEXT    NOT NULL DEFAULT '',
  -- Supprimée : gardée pour l'historique, plus proposée.
  active     INTEGER NOT NULL DEFAULT 1,
  cree_par   INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  cree_le    INTEGER NOT NULL
);
CREATE INDEX taches_famille ON taches(famille_id, active);

-- Une ligne par tâche faite ; « occurrence » : le jour (2026-11-22), la
-- semaine (s2026-11-16) ou « unique ».
CREATE TABLE taches_faites (
  id          INTEGER PRIMARY KEY,
  tache_id    INTEGER NOT NULL REFERENCES taches(id) ON DELETE CASCADE,
  occurrence  TEXT    NOT NULL,
  fait_par    INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  fait_le     INTEGER NOT NULL,
  UNIQUE (tache_id, occurrence)
);
CREATE INDEX taches_faites_date ON taches_faites(fait_le);
