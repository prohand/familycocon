-- Espace enfants.

-- name: MissionsActives :many
SELECT * FROM missions WHERE famille_id = ? AND active = 1 ORDER BY enfant_id, id;

-- name: MissionParID :one
SELECT * FROM missions WHERE id = ? AND famille_id = ? AND active = 1;

-- name: CreerMission :one
INSERT INTO missions (famille_id, enfant_id, titre, emoji, etoiles, categorie, repetition, cree_par, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierMission :exec
UPDATE missions SET enfant_id = ?, titre = ?, emoji = ?, etoiles = ?, categorie = ?, repetition = ?
WHERE id = ? AND famille_id = ?;

-- name: RetirerMission :exec
UPDATE missions SET active = 0 WHERE id = ? AND famille_id = ?;

-- name: MissionsFaitesOccurrences :many
SELECT missions_faites.* FROM missions_faites JOIN missions ON missions.id = missions_faites.mission_id
WHERE missions.famille_id = ? AND missions.active = 1 AND missions_faites.occurrence IN (sqlc.slice('occurrences'));

-- Une mission refusee peut etre recochee ; une mission validee ne bouge plus.
-- La mission doit etre a la famille ; 0 ligne si elle est deja cochee.
-- name: CocherMission :execrows
INSERT INTO missions_faites (mission_id, occurrence, etat, etoiles, coche_le)
SELECT m.id, sqlc.arg(occurrence), 'attente', sqlc.arg(etoiles), sqlc.arg(coche_le)
FROM missions m WHERE m.id = sqlc.arg(mission_id) AND m.famille_id = sqlc.arg(famille_id)
ON CONFLICT (mission_id, occurrence) DO UPDATE SET
  etat = 'attente', etoiles = excluded.etoiles, coche_le = excluded.coche_le, decide_par = NULL, decide_le = NULL
WHERE missions_faites.etat = 'refusee';

-- name: AnnulerCoche :exec
DELETE FROM missions_faites WHERE mission_id = ? AND occurrence = ? AND etat = 'attente'
  AND mission_id IN (SELECT m.id FROM missions m WHERE m.famille_id = ?);

-- name: FaiteMissionParID :one
SELECT missions_faites.*, missions.titre, missions.emoji, missions.enfant_id
FROM missions_faites JOIN missions ON missions.id = missions_faites.mission_id
WHERE missions_faites.id = ? AND missions.famille_id = ?;

-- name: DeciderMission :execrows
UPDATE missions_faites SET etat = ?, decide_par = ?, decide_le = ? WHERE missions_faites.id = ? AND missions_faites.etat = 'attente'
  AND mission_id IN (SELECT m.id FROM missions m WHERE m.famille_id = ?);

-- name: MissionsEnAttente :many
SELECT missions_faites.id, missions_faites.etoiles, missions_faites.coche_le, missions.titre, missions.emoji, missions.enfant_id
FROM missions_faites JOIN missions ON missions.id = missions_faites.mission_id
WHERE missions.famille_id = ? AND missions_faites.etat = 'attente'
ORDER BY missions_faites.coche_le;

-- name: EtoilesGagnees :one
SELECT CAST(coalesce(sum(missions_faites.etoiles), 0) AS INTEGER)
FROM missions_faites JOIN missions ON missions.id = missions_faites.mission_id
WHERE missions.enfant_id = ? AND missions_faites.etat = 'validee';

-- name: EtoilesEchangees :one
SELECT
  CAST(coalesce(sum(CASE WHEN etat = 'confirme' THEN prix ELSE 0 END), 0) AS INTEGER) AS depensees,
  CAST(coalesce(sum(CASE WHEN etat = 'attente' THEN prix ELSE 0 END), 0) AS INTEGER) AS reservees
FROM echanges WHERE enfant_id = ?;

-- name: MissionsValideesDepuis :many
SELECT missions_faites.etoiles, missions_faites.decide_le, missions.titre, missions.emoji
FROM missions_faites JOIN missions ON missions.id = missions_faites.mission_id
WHERE missions.enfant_id = ? AND missions_faites.etat = 'validee' AND missions_faites.decide_le >= ?
ORDER BY missions_faites.decide_le DESC;

-- name: RecompensesActives :many
SELECT * FROM recompenses WHERE famille_id = ? AND active = 1 ORDER BY prix, id;

-- name: RecompenseParID :one
SELECT * FROM recompenses WHERE id = ? AND famille_id = ? AND active = 1;

-- name: CreerRecompense :one
INSERT INTO recompenses (famille_id, titre, emoji, prix, cree_le) VALUES (?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierRecompense :exec
UPDATE recompenses SET titre = ?, emoji = ?, prix = ? WHERE id = ? AND famille_id = ?;

-- name: RetirerRecompense :exec
UPDATE recompenses SET active = 0 WHERE id = ? AND famille_id = ?;

-- name: CreerEchange :one
INSERT INTO echanges (recompense_id, enfant_id, prix, etat, demande_le) VALUES (?, ?, ?, 'attente', ?) RETURNING *;

-- Une demande deja en attente pour cette recompense (double envoi).
-- name: EchangeEnAttente :one
SELECT count(*) FROM echanges WHERE recompense_id = ? AND enfant_id = ? AND etat = 'attente';

-- name: EchangeParID :one
SELECT echanges.*, recompenses.titre, recompenses.emoji
FROM echanges JOIN recompenses ON recompenses.id = echanges.recompense_id
WHERE echanges.id = ? AND recompenses.famille_id = ?;

-- name: DeciderEchange :execrows
UPDATE echanges SET etat = ?, decide_par = ?, decide_le = ? WHERE echanges.id = ? AND echanges.etat = 'attente'
  AND recompense_id IN (SELECT r.id FROM recompenses r WHERE r.famille_id = ?);

-- name: EchangesEnAttente :many
SELECT echanges.*, recompenses.titre, recompenses.emoji
FROM echanges JOIN recompenses ON recompenses.id = echanges.recompense_id
WHERE recompenses.famille_id = ? AND echanges.etat = 'attente' ORDER BY echanges.demande_le;

-- name: EchangesEnfant :many
SELECT echanges.*, recompenses.titre, recompenses.emoji
FROM echanges JOIN recompenses ON recompenses.id = echanges.recompense_id
WHERE echanges.enfant_id = ? ORDER BY echanges.demande_le DESC LIMIT 20;

-- Etoiles donnees ou retirees par un parent.

-- name: AjouterBonus :exec
INSERT INTO etoiles_bonus (famille_id, enfant_id, etoiles, motif, par, le) VALUES (?, ?, ?, ?, ?, ?);

-- name: EtoilesBonus :one
SELECT CAST(coalesce(sum(etoiles), 0) AS INTEGER) FROM etoiles_bonus WHERE enfant_id = ?;

-- name: BonusRecents :many
SELECT etoiles, motif, le FROM etoiles_bonus WHERE enfant_id = ? ORDER BY le DESC LIMIT 8;

-- Bonus d'un instant (double envoi) : meme enfant, meme motif, 10 dernieres secondes.
-- name: BonusIdentiqueRecent :one
SELECT count(*) FROM etoiles_bonus WHERE enfant_id = ? AND etoiles = ? AND motif = ? AND le >= ?;
