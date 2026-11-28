-- Pieces jointes : detachees des que leur objet est supprime.
-- Sans cela, une piece restait liee a l'id de l'objet supprime jusqu'au
-- menage de l'heure ; or SQLite redonne l'id de la derniere ligne supprimee
-- (pas d'AUTOINCREMENT) : le nouvel objet (une note d'un enfant, par
-- exemple) recuperait la piece d'une note privee supprimee.
-- objet_id = 0 : aucun objet n'a cet id ; le menage efface ensuite la ligne
-- et le fichier. Les declencheurs servent aussi aux suppressions en cascade
-- (lignes du carnet d'un vehicule supprime...).
CREATE TRIGGER joints_notes AFTER DELETE ON notes BEGIN
  UPDATE pieces_jointes SET objet_id = 0 WHERE famille_id = OLD.famille_id AND module = 'notes' AND objet_id = OLD.id;
END;
CREATE TRIGGER joints_taches AFTER DELETE ON taches BEGIN
  UPDATE pieces_jointes SET objet_id = 0 WHERE famille_id = OLD.famille_id AND module = 'taches' AND objet_id = OLD.id;
END;
CREATE TRIGGER joints_projets AFTER DELETE ON projets BEGIN
  UPDATE pieces_jointes SET objet_id = 0 WHERE famille_id = OLD.famille_id AND module = 'projets' AND objet_id = OLD.id;
END;
CREATE TRIGGER joints_carnet AFTER DELETE ON carnet BEGIN
  UPDATE pieces_jointes SET objet_id = 0 WHERE famille_id = OLD.famille_id AND module = 'carnet' AND objet_id = OLD.id;
END;
CREATE TRIGGER joints_vehicules AFTER DELETE ON vehicules BEGIN
  UPDATE pieces_jointes SET objet_id = 0 WHERE famille_id = OLD.famille_id AND module = 'vehicules' AND objet_id = OLD.id;
END;
CREATE TRIGGER joints_animaux AFTER DELETE ON animaux BEGIN
  UPDATE pieces_jointes SET objet_id = 0 WHERE famille_id = OLD.famille_id AND module = 'animaux' AND objet_id = OLD.id;
END;
CREATE TRIGGER joints_garanties AFTER DELETE ON garanties BEGIN
  UPDATE pieces_jointes SET objet_id = 0 WHERE famille_id = OLD.famille_id AND module = 'garanties' AND objet_id = OLD.id;
END;
-- Pieces deja orphelines (objet supprime avant cette version) : detachees aussi.
UPDATE pieces_jointes SET objet_id = 0
WHERE (module = 'notes' AND NOT EXISTS (SELECT 1 FROM notes n WHERE n.id = pieces_jointes.objet_id AND n.famille_id = pieces_jointes.famille_id))
   OR (module = 'taches' AND NOT EXISTS (SELECT 1 FROM taches t WHERE t.id = pieces_jointes.objet_id AND t.famille_id = pieces_jointes.famille_id))
   OR (module = 'projets' AND NOT EXISTS (SELECT 1 FROM projets p WHERE p.id = pieces_jointes.objet_id AND p.famille_id = pieces_jointes.famille_id))
   OR (module = 'carnet' AND NOT EXISTS (SELECT 1 FROM carnet c WHERE c.id = pieces_jointes.objet_id AND c.famille_id = pieces_jointes.famille_id))
   OR (module = 'vehicules' AND NOT EXISTS (SELECT 1 FROM vehicules v WHERE v.id = pieces_jointes.objet_id AND v.famille_id = pieces_jointes.famille_id))
   OR (module = 'animaux' AND NOT EXISTS (SELECT 1 FROM animaux a WHERE a.id = pieces_jointes.objet_id AND a.famille_id = pieces_jointes.famille_id))
   OR (module = 'garanties' AND NOT EXISTS (SELECT 1 FROM garanties g WHERE g.id = pieces_jointes.objet_id AND g.famille_id = pieces_jointes.famille_id));
