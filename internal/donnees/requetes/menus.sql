-- Menus.

-- name: RepasPeriode :many
SELECT * FROM repas WHERE famille_id = ? AND jour >= sqlc.arg(du) AND jour <= sqlc.arg(au) ORDER BY jour, moment;

-- name: RepasDuCreneau :one
SELECT * FROM repas WHERE famille_id = ? AND jour = ? AND moment = ?;

-- name: EnregistrerRepas :one
INSERT INTO repas (famille_id, jour, moment, nom, ingredients, cree_par, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (famille_id, jour, moment) DO UPDATE SET
  nom = excluded.nom, ingredients = excluded.ingredients,
  dans_courses = CASE WHEN repas.ingredients = excluded.ingredients THEN repas.dans_courses ELSE 0 END
RETURNING *;

-- name: ViderRepas :exec
DELETE FROM repas WHERE famille_id = ? AND jour = ? AND moment = ?;

-- Une seule fois : 0 ligne si le repas est deja aux courses (double envoi).
-- name: MarquerDansCourses :execrows
UPDATE repas SET dans_courses = 1 WHERE id = ? AND famille_id = ? AND dans_courses = 0;

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

-- name: CompterRecettes :one
SELECT count(*) FROM recettes WHERE famille_id = ?;

-- Page Recettes : toutes les recettes retenues.
-- name: ToutesRecettes :many
SELECT * FROM recettes WHERE famille_id = ? ORDER BY nom COLLATE NOCASE;

-- name: ModifierRecette :exec
UPDATE recettes SET nom = ?, ingredients = ? WHERE famille_id = ? AND cle = ?;

-- name: OublierRecette :exec
DELETE FROM recettes WHERE famille_id = ? AND cle = ?;

-- Reprendre la semaine derniere : seulement les creneaux encore vides.
-- name: CopierRepas :execrows
INSERT INTO repas (famille_id, jour, moment, nom, ingredients, cree_par, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (famille_id, jour, moment) DO NOTHING;
