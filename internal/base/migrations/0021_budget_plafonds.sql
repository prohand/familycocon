-- Budget : plafond mensuel par categorie de depense (enveloppe). La jauge
-- de la categorie se compare a ce plafond ; une alerte s'affiche a 90 %.
CREATE TABLE budget_plafonds (
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  categorie  TEXT    NOT NULL,
  montant    INTEGER NOT NULL, -- en centimes, par mois
  PRIMARY KEY (famille_id, categorie)
);
