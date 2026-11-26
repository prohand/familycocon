-- Taches a tour de role : " Passer au suivant ". Chaque ligne decale le tour
-- d'un cran a partir de l'occurrence numero (Regle.Numero) : la tache de ce
-- jour-la revient au membre suivant, et la suite continue a partir de lui.
-- Le tour d'un jour vaut (numero + nombre de lignes de numero <= au sien)
-- modulo le nombre de membres : les jours passes gardent leur membre.
-- de_membre : celui dont c'etait le tour ; a_membre : celui a qui il passe.
CREATE TABLE taches_tours_passes (
  id         INTEGER PRIMARY KEY,
  tache_id   INTEGER NOT NULL REFERENCES taches(id) ON DELETE CASCADE,
  numero     INTEGER NOT NULL,
  de_membre  INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  a_membre   INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  passe_par  INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  passe_le   INTEGER NOT NULL
);
CREATE INDEX taches_tours_passes_tache ON taches_tours_passes(tache_id, numero);
CREATE INDEX taches_tours_passes_date ON taches_tours_passes(passe_le);
