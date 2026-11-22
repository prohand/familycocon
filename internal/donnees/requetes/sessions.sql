-- Sessions, invitations et appareils de confiance (jetons gardes en empreinte).

-- name: CreerSession :exec
INSERT INTO sessions (empreinte, membre_id, jeton_csrf, cree_le, vue_le, expire_le)
VALUES (?, ?, ?, ?, ?, ?);

-- name: SessionParEmpreinte :one
SELECT sqlc.embed(sessions), sqlc.embed(membres), sqlc.embed(familles)
FROM sessions
JOIN membres ON membres.id = sessions.membre_id
JOIN familles ON familles.id = membres.famille_id
WHERE sessions.empreinte = ? AND sessions.expire_le > ? AND membres.actif = 1;

-- name: ProlongerSession :exec
UPDATE sessions SET vue_le = ?, expire_le = ? WHERE empreinte = ?;

-- name: SupprimerSession :exec
DELETE FROM sessions WHERE empreinte = ?;

-- name: SupprimerSessionsMembre :exec
DELETE FROM sessions WHERE membre_id = ?;

-- name: SupprimerAutresSessions :exec
DELETE FROM sessions WHERE membre_id = ? AND empreinte <> ?;

-- name: PurgerSessions :execrows
DELETE FROM sessions WHERE expire_le <= ?;

-- name: CreerInvitation :exec
INSERT INTO invitations (empreinte, famille_id, role, cree_par, cree_le, expire_le)
VALUES (?, ?, ?, ?, ?, ?);

-- name: InvitationParEmpreinte :one
SELECT invitations.*, familles.nom AS famille_nom
FROM invitations JOIN familles ON familles.id = invitations.famille_id
WHERE invitations.empreinte = ? AND invitations.expire_le > ?;

-- name: SupprimerInvitation :exec
DELETE FROM invitations WHERE empreinte = ?;

-- name: InvitationsEnCours :many
SELECT * FROM invitations WHERE famille_id = ? AND expire_le > ? ORDER BY cree_le DESC;

-- name: AnnulerInvitations :exec
DELETE FROM invitations WHERE famille_id = ?;

-- name: PurgerInvitations :execrows
DELETE FROM invitations WHERE expire_le <= ?;

-- name: CreerAppareil :exec
INSERT INTO appareils (empreinte, famille_id, cree_le, vue_le) VALUES (?, ?, ?, ?);

-- name: AppareilParEmpreinte :one
SELECT * FROM appareils WHERE empreinte = ?;

-- name: VoirAppareil :exec
UPDATE appareils SET vue_le = ? WHERE empreinte = ?;

-- name: PurgerAppareils :execrows
DELETE FROM appareils WHERE vue_le <= ?;
