-- Mission " Pas besoin de verifier " (reglee par un parent) : cochee par
-- l'enfant, elle est validee tout de suite (etoiles gagnees sans passer par
-- " A valider ").
ALTER TABLE missions ADD COLUMN sans_verification INTEGER NOT NULL DEFAULT 0;
