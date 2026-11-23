-- Espace enfants : etoiles donnees (bonus) ou retirees par un parent, avec
-- un motif (« a aide sa soeur », « gros mot »). etoiles : de -20 a 20, jamais 0.
CREATE TABLE etoiles_bonus (
  id         INTEGER PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  enfant_id  INTEGER NOT NULL REFERENCES membres(id) ON DELETE CASCADE,
  etoiles    INTEGER NOT NULL CHECK (etoiles BETWEEN -20 AND 20 AND etoiles <> 0),
  motif      TEXT    NOT NULL,
  par        INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  le         INTEGER NOT NULL
);
CREATE INDEX etoiles_bonus_enfant ON etoiles_bonus(enfant_id, le);
