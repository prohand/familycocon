-- Agenda.

-- name: EvenementsPeriode :many
SELECT * FROM evenements
WHERE famille_id = ? AND debut < sqlc.arg(fin_periode) AND (fin >= sqlc.arg(debut_periode) OR repetition <> '')
ORDER BY debut;

-- name: EvenementParID :one
SELECT * FROM evenements WHERE id = ? AND famille_id = ?;

-- name: CreerEvenement :one
INSERT INTO evenements (famille_id, titre, lieu, note, debut, fin, journee, repetition, cree_par, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierEvenement :exec
UPDATE evenements SET titre = ?, lieu = ?, note = ?, debut = ?, fin = ?, journee = ?, repetition = ?
WHERE id = ? AND famille_id = ?;

-- name: SupprimerEvenement :exec
DELETE FROM evenements WHERE id = ? AND famille_id = ?;

-- name: ParticipantsDeFamille :many
SELECT evenements_membres.* FROM evenements_membres
JOIN evenements ON evenements.id = evenements_membres.evenement_id
WHERE evenements.famille_id = ?;

-- name: AjouterParticipant :exec
INSERT OR IGNORE INTO evenements_membres (evenement_id, membre_id) VALUES (?, ?);

-- name: ViderParticipants :exec
DELETE FROM evenements_membres WHERE evenement_id = ?;
