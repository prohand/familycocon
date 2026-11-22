-- Budget : revenus et dépenses (montants en centimes), par catégorie.
-- « fixe » : revient chaque mois (salaire, loyer…), recopiable d'un clic.

CREATE TABLE operations (
  id         INTEGER PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  sens       TEXT    NOT NULL CHECK (sens IN ('depense', 'revenu')),
  montant    INTEGER NOT NULL CHECK (montant > 0),
  categorie  TEXT    NOT NULL,
  libelle    TEXT    NOT NULL DEFAULT '',
  jour       TEXT    NOT NULL,   -- 2026-11-22
  membre_id  INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  fixe       INTEGER NOT NULL DEFAULT 0,
  cree_par   INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  cree_le    INTEGER NOT NULL
);
CREATE INDEX operations_mois ON operations(famille_id, jour);
