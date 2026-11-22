-- Budget : opérations qui reviennent (salaire, loyer, abonnements…), créées
-- toutes seules à leur date. Une occurrence n°k tombe à debut + k périodes
-- (même jour du mois, ou le dernier jour s'il n'existe pas) ; « faites »
-- compte les occurrences déjà écrites dans operations.
-- Remplace les opérations « fixes » (recopiées à la main chaque mois).

CREATE TABLE recurrences (
  id         INTEGER PRIMARY KEY,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  sens       TEXT    NOT NULL CHECK (sens IN ('depense', 'revenu')),
  montant    INTEGER NOT NULL CHECK (montant > 0),
  categorie  TEXT    NOT NULL,
  libelle    TEXT    NOT NULL DEFAULT '',
  membre_id  INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  frequence  TEXT    NOT NULL CHECK (frequence IN ('semaine', 'mois', 'trimestre', 'annee')),
  debut      TEXT    NOT NULL,             -- première occurrence
  fin        TEXT    NOT NULL DEFAULT '',  -- dernière date possible, '' : sans fin
  faites     INTEGER NOT NULL DEFAULT 0,
  cree_par   INTEGER REFERENCES membres(id) ON DELETE SET NULL,
  cree_le    INTEGER NOT NULL
);
CREATE INDEX recurrences_famille ON recurrences(famille_id);

ALTER TABLE operations ADD COLUMN recurrence_id INTEGER REFERENCES recurrences(id) ON DELETE SET NULL;

-- Les anciennes opérations fixes deviennent des récurrences mensuelles qui
-- partent de leur dernière occurrence (déjà écrite : faites = 1).
INSERT INTO recurrences (famille_id, sens, montant, categorie, libelle, membre_id, frequence, debut, faites, cree_par, cree_le)
SELECT famille_id, sens, montant, categorie, libelle, membre_id, 'mois', MAX(jour), 1, cree_par, cree_le
FROM operations WHERE fixe = 1 GROUP BY famille_id, categorie, libelle;

UPDATE operations SET recurrence_id = (
  SELECT r.id FROM recurrences r
  WHERE r.famille_id = operations.famille_id AND r.categorie = operations.categorie AND r.libelle = operations.libelle
) WHERE fixe = 1;
UPDATE operations SET fixe = 0;

-- Abonnements (Netflix, téléphone, box, assurance…) : une récurrence du
-- budget et quelques informations en plus. Réservé aux parents.
CREATE TABLE abonnements (
  id             INTEGER PRIMARY KEY,
  famille_id     INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  recurrence_id  INTEGER NOT NULL UNIQUE REFERENCES recurrences(id) ON DELETE CASCADE,
  emoji          TEXT    NOT NULL DEFAULT '📺',
  engagement_fin TEXT    NOT NULL DEFAULT '',  -- fin d'engagement (2027-06-30) ou ''
  site           TEXT    NOT NULL DEFAULT '',  -- page de gestion ou de résiliation
  notes          TEXT    NOT NULL DEFAULT '',
  cree_le        INTEGER NOT NULL
);

UPDATE familles SET modules = CASE WHEN modules = '' THEN 'abonnements' ELSE modules || ',abonnements' END;
