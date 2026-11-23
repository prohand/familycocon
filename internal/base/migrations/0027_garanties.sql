-- Module Garanties : les appareils achetes (lave-linge, telephone...), leur
-- garantie et ou est rangee la facture. La fin de garantie devient une
-- echeance (table echeances, module "garanties") : rappels compris.
CREATE TABLE garanties (
  id            INTEGER PRIMARY KEY,
  famille_id    INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  nom           TEXT    NOT NULL,
  marque        TEXT    NOT NULL DEFAULT '',
  achat         TEXT    NOT NULL DEFAULT '',   -- date d'achat (2026-11-23) ou ''
  prix          INTEGER NOT NULL DEFAULT 0,    -- centimes
  garantie_mois INTEGER NOT NULL DEFAULT 24,
  magasin       TEXT    NOT NULL DEFAULT '',
  magasin_id    INTEGER REFERENCES contacts(id) ON DELETE SET NULL,
  facture       TEXT    NOT NULL DEFAULT '',   -- ou est rangee la facture
  notes         TEXT    NOT NULL DEFAULT '',
  cree_le       INTEGER NOT NULL
);
CREATE INDEX garanties_famille ON garanties(famille_id);

-- Les familles existantes ont le nouveau module (un parent peut l'enlever).
UPDATE familles SET modules = modules || ',garanties' WHERE modules <> '' AND instr(modules, 'garanties') = 0;
