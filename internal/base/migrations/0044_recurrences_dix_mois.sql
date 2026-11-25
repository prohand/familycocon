-- sans cles etrangeres
-- (premiere ligne lue par base.appliquer : la migration tourne avec
-- foreign_keys=OFF, sinon supprimer l'ancienne table viderait
-- operations.recurrence_id et les abonnements ; foreign_key_check a la fin.)
--
-- Budget : nouvelle frequence « mois10 », chaque mois de janvier a octobre
-- (10 fois par an, comme la taxe fonciere mensualisee). SQLite ne sait pas
-- changer un CHECK : la table est refaite, colonnes dans le meme ordre.

CREATE TABLE recurrences_nouveau (
  id         INTEGER PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  sens       TEXT    NOT NULL CHECK (sens IN ('depense', 'revenu')),
  montant    INTEGER NOT NULL CHECK (montant > 0),
  categorie  TEXT    NOT NULL,
  libelle    TEXT    NOT NULL DEFAULT '',
  membre_id  INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  frequence  TEXT    NOT NULL CHECK (frequence IN ('semaine', 'mois', 'mois10', '2mois', 'trimestre', 'semestre', 'annee')),
  debut      TEXT    NOT NULL,
  fin        TEXT    NOT NULL DEFAULT '',
  faites     INTEGER NOT NULL DEFAULT 0,
  cree_par   INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  cree_le    INTEGER NOT NULL,
  perso_de   INTEGER NOT NULL DEFAULT 0,
  virement   INTEGER NOT NULL DEFAULT 0
);
INSERT INTO recurrences_nouveau (id, famille_id, sens, montant, categorie, libelle, membre_id, frequence, debut, fin, faites, cree_par, cree_le, perso_de, virement)
  SELECT id, famille_id, sens, montant, categorie, libelle, membre_id, frequence, debut, fin, faites, cree_par, cree_le, perso_de, virement FROM recurrences;
DROP TABLE recurrences;
ALTER TABLE recurrences_nouveau RENAME TO recurrences;
CREATE INDEX recurrences_famille ON recurrences(famille_id);
