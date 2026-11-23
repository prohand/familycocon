-- Taches menageres.

-- Une tache " une fois " faite avant aujourd'hui (depuis = minuit, heure de
-- la famille) n'est plus chargee : elle n'est plus montree nulle part.
-- name: TachesActives :many
SELECT * FROM taches
WHERE taches.famille_id = sqlc.arg(famille_id) AND taches.active = 1
  AND NOT (taches.repetition = '' AND EXISTS (
    SELECT 1 FROM taches_faites f
    WHERE f.tache_id = taches.id AND f.occurrence = 'unique' AND f.fait_le < sqlc.arg(depuis)))
ORDER BY taches.id;

-- name: CompterTachesActives :one
SELECT count(*) FROM taches
WHERE taches.famille_id = sqlc.arg(famille_id) AND taches.active = 1
  AND NOT (taches.repetition = '' AND EXISTS (
    SELECT 1 FROM taches_faites f
    WHERE f.tache_id = taches.id AND f.occurrence = 'unique' AND f.fait_le < sqlc.arg(depuis)));

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
WHERE taches.famille_id = sqlc.arg(famille_id) AND taches.active = 1
  AND taches_faites.occurrence IN (sqlc.slice('occurrences'))
  AND (taches_faites.occurrence <> 'unique' OR taches_faites.fait_le >= sqlc.arg(depuis));

-- 0 ligne si elle est deja cochee (deux membres en meme temps).
-- name: MarquerFaite :execrows
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

-- Scores de la semaine : comptes en SQL (pas de plafond de lignes).
-- name: ScoresTaches :many
SELECT taches_faites.fait_par, count(*) AS nombre
FROM taches_faites JOIN taches ON taches.id = taches_faites.tache_id
WHERE taches.famille_id = ? AND taches_faites.fait_le >= ? AND taches_faites.fait_par IS NOT NULL
GROUP BY taches_faites.fait_par;

-- Membre retire : ses taches reviennent a tout le monde.
-- name: LibererTachesMembre :exec
UPDATE taches SET assigne_a = NULL WHERE famille_id = ? AND assigne_a = ?;
