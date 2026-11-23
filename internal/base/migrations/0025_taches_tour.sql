-- Taches a tour de role : les membres qui se relaient (identifiants separes
-- par des virgules, dans l'ordre). '' : pas de tour de role (assigne_a).
ALTER TABLE taches ADD COLUMN tour TEXT NOT NULL DEFAULT '';
