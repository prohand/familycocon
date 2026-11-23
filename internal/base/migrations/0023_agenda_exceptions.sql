-- Agenda : fois annulees d'un evenement qui revient ("annuler cette fois
-- seulement" : vacances, cours annule). jour : date de la fois (2026-12-01).
CREATE TABLE evenements_exceptions (
  evenement_id INTEGER NOT NULL REFERENCES evenements(id) ON DELETE CASCADE,
  jour         TEXT    NOT NULL,
  PRIMARY KEY (evenement_id, jour)
);
