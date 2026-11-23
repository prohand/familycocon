-- Index manquants releves par la revue du 23/11/2026 (aucune donnee changee).

-- Espace enfants : etoiles gagnees et progres d'un enfant.
CREATE INDEX missions_enfant ON missions(enfant_id);
-- Missions cochees qui attendent un parent (peu de lignes : index partiel).
CREATE INDEX missions_faites_attente ON missions_faites(mission_id) WHERE etat = 'attente';
-- Demandes de recompense d'une famille, et suppression d'une recompense.
CREATE INDEX echanges_recompense ON echanges(recompense_id);
CREATE INDEX recompenses_famille ON recompenses(famille_id);

-- Budget : operations d'une recurrence (suppression, ON DELETE SET NULL).
CREATE INDEX operations_recurrence ON operations(recurrence_id);
CREATE INDEX abonnements_famille ON abonnements(famille_id);

-- Agenda : evenements qui citent un contact (jointure, contact supprime).
CREATE INDEX evenements_contacts_contact ON evenements_contacts(contact_id);
