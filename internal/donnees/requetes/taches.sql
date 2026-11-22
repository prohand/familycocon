-- Taches menageres.

-- name: TachesActives :many
SELECT * FROM taches WHERE famille_id = ? AND active = 1 ORDER BY id;

-- name: TacheParID :one
SELECT * FROM taches WHERE id = ? AND famille_id = ? AND active = 1;

-- name: CreerTache :one
INSERT INTO taches (famille_id, titre, emoji, assigne_a, repetition, cree_par, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierTache :exec
UPDATE taches SET titre = ?, emoji = ?, assigne_a = ?, repetition = ? WHERE id = ? AND famille_id = ?;

-- name: RetirerTache :exec
UPDATE taches SET active = 0 WHERE id = ? AND famille_id = ?;

-- name: TachesFaitesOccurrences :many
SELECT taches_faites.* FROM taches_faites JOIN taches ON taches.id = taches_faites.tache_id
WHERE taches.famille_id = ? AND taches.active = 1 AND taches_faites.occurrence IN (sqlc.slice('occurrences'));

-- name: MarquerFaite :exec
INSERT INTO taches_faites (tache_id, occurrence, fait_par, fait_le) VALUES (?, ?, ?, ?)
ON CONFLICT (tache_id, occurrence) DO NOTHING;

-- name: FaiteParOccurrence :one
SELECT * FROM taches_faites WHERE tache_id = ? AND occurrence = ?;

-- name: AnnulerFaite :exec
DELETE FROM taches_faites WHERE tache_id = ? AND occurrence = ?;

-- name: HistoriqueTaches :many
SELECT taches.titre, taches.emoji, taches_faites.fait_le, taches_faites.fait_par
FROM taches_faites JOIN taches ON taches.id = taches_faites.tache_id
WHERE taches.famille_id = ? AND taches_faites.fait_le >= ?
ORDER BY taches_faites.fait_le DESC LIMIT 100;
