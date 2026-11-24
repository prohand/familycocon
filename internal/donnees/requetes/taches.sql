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
INSERT INTO taches (famille_id, titre, emoji, assigne_a, tour, repetition, categorie_id, rappel, date_rappel, cree_par, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierTache :exec
UPDATE taches SET titre = ?, emoji = ?, assigne_a = ?, tour = ?, repetition = ?, categorie_id = ?, rappel = ?, date_rappel = ?
WHERE id = ? AND famille_id = ?;

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

-- Rappels : les taches qui en ont un (toutes les familles), avec le fuseau
-- de leur famille.
-- name: TachesAvecRappel :many
SELECT t.id, t.famille_id, t.titre, t.emoji, t.assigne_a, t.tour, t.repetition, t.rappel, t.date_rappel, t.cree_le, f.fuseau
FROM taches t JOIN familles f ON f.id = t.famille_id
WHERE t.active = 1 AND t.rappel <> '' ORDER BY t.id LIMIT 5000;

-- 0 ligne : rappel deja envoye ce jour-la.
-- name: MarquerRappelTache :execrows
INSERT INTO taches_rappels (tache_id, jour, envoye_le) VALUES (?, ?, ?)
ON CONFLICT (tache_id, jour) DO NOTHING;

-- name: OublierRappelsTaches :exec
DELETE FROM taches_rappels WHERE envoye_le < ?;

-- Categories des taches de la famille, avec le nombre de taches en cours.
-- name: CategoriesTaches :many
SELECT c.id, c.nom, c.emoji,
  (SELECT count(*) FROM taches t WHERE t.categorie_id = c.id AND t.active = 1) AS nombre
FROM categories_taches c WHERE c.famille_id = ? ORDER BY c.nom COLLATE NOCASE;

-- name: CompterCategoriesTaches :one
SELECT count(*) FROM categories_taches WHERE famille_id = ?;

-- name: CategorieTacheExiste :one
SELECT count(*) FROM categories_taches WHERE id = ? AND famille_id = ?;

-- name: CreerCategorieTache :one
INSERT INTO categories_taches (famille_id, nom, emoji, cree_le) VALUES (?, ?, ?, ?) RETURNING *;

-- name: ModifierCategorieTache :exec
UPDATE categories_taches SET nom = ?, emoji = ? WHERE id = ? AND famille_id = ?;

-- Les taches de la categorie restent, sans categorie (ON DELETE SET NULL).
-- name: SupprimerCategorieTache :exec
DELETE FROM categories_taches WHERE id = ? AND famille_id = ?;
