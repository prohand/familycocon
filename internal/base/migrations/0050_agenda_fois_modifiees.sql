-- Agenda : "modifier cette fois seulement" d'un evenement qui revient.
-- Une ligne de evenements_exceptions est soit une fois annulee
-- (modifiee = 0, comme avant), soit une fois modifiee (modifiee = 1) :
-- jour reste la date d'origine de la fois ; la fois a lieu a nouveau_debut
-- (fin : nouveau_fin), avec nouveau_titre ('' : celui de l'evenement) et
-- nouveau_lieu (NULL : celui de l'evenement).
ALTER TABLE evenements_exceptions ADD COLUMN modifiee INTEGER NOT NULL DEFAULT 0;
ALTER TABLE evenements_exceptions ADD COLUMN nouveau_debut INTEGER NOT NULL DEFAULT 0;
ALTER TABLE evenements_exceptions ADD COLUMN nouveau_fin INTEGER NOT NULL DEFAULT 0;
ALTER TABLE evenements_exceptions ADD COLUMN nouvelle_journee INTEGER NOT NULL DEFAULT 0;
ALTER TABLE evenements_exceptions ADD COLUMN nouveau_titre TEXT NOT NULL DEFAULT '';
ALTER TABLE evenements_exceptions ADD COLUMN nouveau_lieu TEXT;
CREATE INDEX evenements_exceptions_modifiees ON evenements_exceptions(nouveau_debut) WHERE modifiee = 1;
