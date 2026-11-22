-- Menus.

-- name: RepasPeriode :many
SELECT * FROM repas WHERE famille_id = ? AND jour >= sqlc.arg(du) AND jour <= sqlc.arg(au) ORDER BY jour, moment;

-- name: RepasDuCreneau :one
SELECT * FROM repas WHERE famille_id = ? AND jour = ? AND moment = ?;

-- name: EnregistrerRepas :one
INSERT INTO repas (famille_id, jour, moment, nom, ingredients, cree_par, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (famille_id, jour, moment) DO UPDATE SET
  nom = excluded.nom, ingredients = excluded.ingredients, dans_courses = 0
RETURNING *;

-- name: ViderRepas :exec
DELETE FROM repas WHERE famille_id = ? AND jour = ? AND moment = ?;

-- name: MarquerDansCourses :exec
UPDATE repas SET dans_courses = 1 WHERE id = ?;

-- name: RecetteParCle :one
SELECT * FROM recettes WHERE famille_id = ? AND cle = ?;

-- name: RetenirRecette :exec
INSERT INTO recettes (famille_id, cle, nom, ingredients, compte, vu_le) VALUES (?, ?, ?, ?, 1, ?)
ON CONFLICT (famille_id, cle) DO UPDATE SET
  nom = excluded.nom,
  ingredients = CASE WHEN excluded.ingredients <> '' THEN excluded.ingredients ELSE recettes.ingredients END,
  compte = recettes.compte + 1, vu_le = excluded.vu_le;

-- name: RecettesConnues :many
SELECT nom FROM recettes WHERE famille_id = ? ORDER BY compte DESC, vu_le DESC LIMIT 80;
