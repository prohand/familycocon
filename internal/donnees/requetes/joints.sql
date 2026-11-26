-- Pieces jointes (documents) : voir internal/appli/joints.go.

-- name: JointsObjet :many
SELECT * FROM pieces_jointes WHERE famille_id = ? AND module = ? AND objet_id = ? ORDER BY id;

-- Toutes celles d'un module (liste des taches : une seule requete).
-- name: JointsModule :many
SELECT * FROM pieces_jointes WHERE famille_id = ? AND module = ? ORDER BY objet_id, id;

-- name: JointParID :one
SELECT * FROM pieces_jointes WHERE id = ? AND famille_id = ?;

-- name: CompterJointsObjet :one
SELECT count(*) FROM pieces_jointes WHERE famille_id = ? AND module = ? AND objet_id = ?;

-- Place prise par la famille (octets).
-- name: TailleJointsFamille :one
SELECT CAST(coalesce(sum(taille), 0) AS INTEGER) FROM pieces_jointes WHERE famille_id = ?;

-- name: AjouterJoint :one
INSERT INTO pieces_jointes (famille_id, module, objet_id, nom, type, taille, fichier, cree_par, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: SupprimerJoint :exec
DELETE FROM pieces_jointes WHERE id = ? AND famille_id = ?;

-- Pieces dont l'objet n'existe plus (note ou projet supprime, tache retiree,
-- ligne du carnet supprimee avec son vehicule...).
-- name: JointsOrphelins :many
SELECT j.* FROM pieces_jointes j
WHERE (j.module = 'notes' AND NOT EXISTS (SELECT 1 FROM notes n WHERE n.id = j.objet_id AND n.famille_id = j.famille_id))
   OR (j.module = 'taches' AND NOT EXISTS (SELECT 1 FROM taches t WHERE t.id = j.objet_id AND t.famille_id = j.famille_id AND t.active = 1))
   OR (j.module = 'projets' AND NOT EXISTS (SELECT 1 FROM projets p WHERE p.id = j.objet_id AND p.famille_id = j.famille_id))
   OR (j.module = 'carnet' AND NOT EXISTS (SELECT 1 FROM carnet c WHERE c.id = j.objet_id AND c.famille_id = j.famille_id))
   OR (j.module = 'vehicules' AND NOT EXISTS (SELECT 1 FROM vehicules v WHERE v.id = j.objet_id AND v.famille_id = j.famille_id))
   OR (j.module = 'animaux' AND NOT EXISTS (SELECT 1 FROM animaux a WHERE a.id = j.objet_id AND a.famille_id = j.famille_id))
   OR (j.module = 'garanties' AND NOT EXISTS (SELECT 1 FROM garanties g WHERE g.id = j.objet_id AND g.famille_id = j.famille_id))
LIMIT 500;

-- Export : sans les pieces des notes, taches et projets prives des autres membres
-- (ni celles du carnet de leurs projets prives).
-- name: ExportJoints :many
SELECT j.id, j.module, j.objet_id, j.nom, j.type, j.taille, j.cree_par, j.cree_le FROM pieces_jointes j
WHERE j.famille_id = sqlc.arg(famille_id)
  AND NOT (j.module = 'notes' AND EXISTS (SELECT 1 FROM notes n WHERE n.id = j.objet_id AND n.privee = 1 AND n.auteur_id IS NOT sqlc.arg(membre)))
  AND NOT (j.module = 'taches' AND EXISTS (SELECT 1 FROM taches t WHERE t.id = j.objet_id AND t.privee = 1 AND t.cree_par IS NOT sqlc.arg(membre)))
  AND NOT (j.module = 'projets' AND EXISTS (SELECT 1 FROM projets p WHERE p.id = j.objet_id AND p.perso_de <> 0 AND p.perso_de <> sqlc.arg(membre)))
  AND NOT (j.module = 'carnet' AND EXISTS (SELECT 1 FROM carnet c JOIN projets p ON p.id = c.objet_id AND c.module = 'projets'
    WHERE c.id = j.objet_id AND p.perso_de <> 0 AND p.perso_de <> sqlc.arg(membre)))
ORDER BY j.id;
