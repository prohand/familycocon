-- sans cles etrangeres
-- (premiere ligne lue par base.appliquer : la migration tourne avec
-- foreign_keys=OFF, sinon supprimer l'ancienne table viderait
-- operations.recurrence_id et les abonnements ; foreign_key_check a la fin.)
--
-- Budget : nouvelle frequence « scolaire » (chaque mois de septembre a
-- juin : cantine, garderie). Le CHECK sur frequence est retire : la liste
-- est verifiee par le programme (budget.FrequenceValide), une prochaine
-- frequence n'aura plus besoin de refaire la table. Colonnes dans le meme
-- ordre.

CREATE TABLE recurrences_nouveau (
  id              INTEGER PRIMARY KEY,
  famille_id      INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  sens            TEXT    NOT NULL CHECK (sens IN ('depense', 'revenu')),
  montant         INTEGER NOT NULL CHECK (montant > 0),
  categorie       TEXT    NOT NULL,
  libelle         TEXT    NOT NULL DEFAULT '',
  membre_id       INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  frequence       TEXT    NOT NULL,
  debut           TEXT    NOT NULL,
  fin             TEXT    NOT NULL DEFAULT '',
  faites          INTEGER NOT NULL DEFAULT 0,
  cree_par        INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  cree_le         INTEGER NOT NULL,
  perso_de        INTEGER NOT NULL DEFAULT 0,
  virement        INTEGER NOT NULL DEFAULT 0,
  nouveau_montant INTEGER NOT NULL DEFAULT 0,
  nouveau_le      TEXT    NOT NULL DEFAULT '',
  a_confirmer     INTEGER NOT NULL DEFAULT 0
);
INSERT INTO recurrences_nouveau (id, famille_id, sens, montant, categorie, libelle, membre_id, frequence, debut, fin, faites,
  cree_par, cree_le, perso_de, virement, nouveau_montant, nouveau_le, a_confirmer)
  SELECT id, famille_id, sens, montant, categorie, libelle, membre_id, frequence, debut, fin, faites,
  cree_par, cree_le, perso_de, virement, nouveau_montant, nouveau_le, a_confirmer FROM recurrences;
DROP TABLE recurrences;
ALTER TABLE recurrences_nouveau RENAME TO recurrences;
CREATE INDEX recurrences_famille ON recurrences(famille_id);
