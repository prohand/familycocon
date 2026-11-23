-- Budget perso : chaque parent a son budget, prive (seul lui le voit), a
-- cote du compte commun. perso_de : le membre proprietaire ; 0 : compte
-- commun (toutes les operations d'avant y restent).
ALTER TABLE operations ADD COLUMN perso_de INTEGER NOT NULL DEFAULT 0;
ALTER TABLE recurrences ADD COLUMN perso_de INTEGER NOT NULL DEFAULT 0;
DROP INDEX operations_mois;
CREATE INDEX operations_mois ON operations(famille_id, perso_de, jour);

-- Virement d'un budget perso vers le compte commun : deux operations
-- liees (depense cote perso, revenu cote commun), supprimees ensemble.
ALTER TABLE operations ADD COLUMN virement_id INTEGER REFERENCES operations(id) ON DELETE SET NULL;
CREATE INDEX operations_virement ON operations(virement_id);
-- Virement qui revient (chaque mois...) : chaque occurrence ecrit les deux.
ALTER TABLE recurrences ADD COLUMN virement INTEGER NOT NULL DEFAULT 0;

-- Plafonds : par compte aussi.
CREATE TABLE budget_plafonds_nouveau (
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  perso_de   INTEGER NOT NULL DEFAULT 0,
  categorie  TEXT    NOT NULL,
  montant    INTEGER NOT NULL, -- en centimes, par mois
  PRIMARY KEY (famille_id, perso_de, categorie)
);
INSERT INTO budget_plafonds_nouveau (famille_id, perso_de, categorie, montant)
  SELECT famille_id, 0, categorie, montant FROM budget_plafonds;
DROP TABLE budget_plafonds;
ALTER TABLE budget_plafonds_nouveau RENAME TO budget_plafonds;

-- Abonnements et projets prives : ceux d'un budget perso (recurrences.perso_de)
-- ou marques prives (projets.perso_de), vus par leur proprietaire seul.
ALTER TABLE projets ADD COLUMN perso_de INTEGER NOT NULL DEFAULT 0;
