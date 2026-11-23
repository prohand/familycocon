-- Index des nouveaux liens (sinon, supprimer une operation ou un contact
-- parcourt toute la table) :
--   - carnet.operation_id (depense du carnet ajoutee au budget) ;
--   - garanties.magasin_id (magasin choisi dans les contacts) ;
--   - agenda_ical.famille_id (famille supprimee).
CREATE INDEX carnet_operation ON carnet(operation_id);
CREATE INDEX garanties_magasin ON garanties(magasin_id);
CREATE INDEX agenda_ical_famille ON agenda_ical(famille_id);
