-- Projets et leurs etapes.

-- Projets vus par un membre : les partages, et ses projets prives.
-- name: ProjetsFamille :many
SELECT p.id, p.nom, p.emoji, p.couleur, p.objectif, p.budget, p.termine_le, p.perso_de,
  (SELECT COUNT(*) FROM projet_etapes e WHERE e.projet_id = p.id) AS nb_etapes,
  (SELECT COUNT(*) FROM projet_etapes e WHERE e.projet_id = p.id AND e.faite_le > 0) AS nb_faites,
  CAST((SELECT coalesce(sum(k.montant), 0) FROM carnet k WHERE k.famille_id = p.famille_id AND k.module = 'projets' AND k.objet_id = p.id) AS INTEGER) AS depense
FROM projets p WHERE p.famille_id = ? AND p.perso_de IN (0, sqlc.arg(membre))
ORDER BY p.termine_le > 0, p.termine_le DESC, p.objectif = '', p.objectif, p.nom COLLATE NOCASE;

-- name: ProjetParID :one
SELECT * FROM projets WHERE id = ? AND famille_id = ?;

-- name: CreerProjet :one
INSERT INTO projets (famille_id, perso_de, nom, emoji, couleur, description, objectif, budget, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierProjet :exec
UPDATE projets SET nom = ?, emoji = ?, couleur = ?, description = ?, objectif = ?, budget = ?, perso_de = ? WHERE id = ? AND famille_id = ?;

-- name: TerminerProjet :exec
UPDATE projets SET termine_le = ? WHERE id = ? AND famille_id = ?;

-- name: SupprimerProjet :exec
DELETE FROM projets WHERE id = ? AND famille_id = ?;

-- name: EtapesProjet :many
SELECT * FROM projet_etapes WHERE projet_id = ? AND famille_id = ?
ORDER BY faite_le > 0, jour = '', jour, id;

-- name: EtapeParID :one
SELECT * FROM projet_etapes WHERE id = ? AND famille_id = ?;

-- name: AjouterEtape :one
INSERT INTO projet_etapes (projet_id, famille_id, texte, jour, membre_id, cree_le)
VALUES (?, ?, ?, ?, ?, ?) RETURNING *;

-- name: CocherEtape :exec
UPDATE projet_etapes SET faite_le = ?, faite_par = ? WHERE id = ? AND famille_id = ?;

-- name: SupprimerEtape :exec
DELETE FROM projet_etapes WHERE id = ? AND famille_id = ?;

-- name: EtapesPeriode :many
SELECT e.id, e.texte, e.jour, e.projet_id, p.nom AS projet, p.emoji, p.perso_de
FROM projet_etapes e JOIN projets p ON p.id = e.projet_id
WHERE e.famille_id = sqlc.arg(famille_id) AND e.faite_le = 0 AND p.termine_le = 0
  AND e.jour >= sqlc.arg(du) AND e.jour < sqlc.arg(au)
ORDER BY e.jour;

-- name: ObjectifsPeriode :many
SELECT id, nom, emoji, objectif, perso_de FROM projets
WHERE famille_id = sqlc.arg(famille_id) AND termine_le = 0
  AND objectif >= sqlc.arg(du) AND objectif < sqlc.arg(au);

-- name: CompterProjets :one
SELECT count(*) FROM projets WHERE famille_id = ?;

-- name: CompterProjetsEnCours :one
SELECT count(*) FROM projets WHERE famille_id = ? AND termine_le = 0 AND perso_de IN (0, sqlc.arg(membre));

-- Membre retire : ses projets prives, que personne d'autre ne voit (le
-- carnet et les photos d'abord).
-- name: ProjetsPrivesMembre :many
SELECT id FROM projets WHERE famille_id = ? AND perso_de = ?;

-- name: CompterEtapesProjet :one
SELECT count(*) FROM projet_etapes WHERE projet_id = ? AND famille_id = ?;

-- Modifier une etape (texte, date, qui).
-- name: ModifierEtape :exec
UPDATE projet_etapes SET texte = sqlc.arg(texte), jour = sqlc.arg(jour), membre_id = sqlc.arg(membre_id)
WHERE id = sqlc.arg(id) AND famille_id = sqlc.arg(famille_id);
