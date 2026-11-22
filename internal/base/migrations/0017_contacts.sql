-- Module Contacts : le carnet central de la famille. Adresses (contacts avec
-- une adresse) et Anniversaires (contacts avec une date) en sont des vues ;
-- Prestataires aussi (contacts « pro »). Animaux, Véhicules et Agenda
-- peuvent citer un contact.

ALTER TABLE contacts ADD COLUMN tel_fixe    TEXT    NOT NULL DEFAULT '';
ALTER TABLE contacts ADD COLUMN tel_travail TEXT    NOT NULL DEFAULT '';
ALTER TABLE contacts ADD COLUMN societe     TEXT    NOT NULL DEFAULT '';
ALTER TABLE contacts ADD COLUMN metier      TEXT    NOT NULL DEFAULT '';
ALTER TABLE contacts ADD COLUMN site        TEXT    NOT NULL DEFAULT '';
ALTER TABLE contacts ADD COLUMN favori      INTEGER NOT NULL DEFAULT 0;
ALTER TABLE contacts ADD COLUMN pro         INTEGER NOT NULL DEFAULT 0;  -- prestataire, professionnel
ALTER TABLE contacts ADD COLUMN groupes     TEXT    NOT NULL DEFAULT ''; -- étiquettes libres, séparées par des virgules

-- Les prestataires deviennent des contacts « pro ». Leurs échéances et leur
-- carnet (module 'prestataires') suivent le nouvel identifiant.
ALTER TABLE contacts ADD COLUMN ancien_prestataire INTEGER;
INSERT INTO contacts (famille_id, nom, relation, telephone, email, adresse, notes, metier, site, pro, cree_le, modifie_le, ancien_prestataire)
SELECT famille_id, nom, 'pro', telephone, email, adresse, notes, metier, site, 1, cree_le, cree_le, id FROM prestataires;
UPDATE echeances SET objet_id = (SELECT c.id FROM contacts c WHERE c.ancien_prestataire = echeances.objet_id AND c.famille_id = echeances.famille_id)
WHERE module = 'prestataires';
UPDATE carnet SET objet_id = (SELECT c.id FROM contacts c WHERE c.ancien_prestataire = carnet.objet_id AND c.famille_id = carnet.famille_id)
WHERE module = 'prestataires';
ALTER TABLE contacts DROP COLUMN ancien_prestataire;
DROP TABLE prestataires;

-- Contacts cités par les autres modules (le texte libre reste possible).
ALTER TABLE animaux   ADD COLUMN veterinaire_id INTEGER REFERENCES contacts(id) ON DELETE SET NULL;
ALTER TABLE vehicules ADD COLUMN garage_id      INTEGER REFERENCES contacts(id) ON DELETE SET NULL;
ALTER TABLE vehicules ADD COLUMN assureur_id    INTEGER REFERENCES contacts(id) ON DELETE SET NULL;

CREATE TABLE evenements_contacts (
  evenement_id INTEGER NOT NULL REFERENCES evenements(id) ON DELETE CASCADE,
  contact_id   INTEGER NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
  PRIMARY KEY (evenement_id, contact_id)
);

UPDATE familles SET modules = CASE WHEN modules = '' THEN 'contacts' ELSE modules || ',contacts' END;
