-- Double authentification (TOTP, codes de secours) et passkeys.

-- name: MembreParIDSeul :one
SELECT * FROM membres WHERE id = ? AND actif = 1;

-- name: ActiverTOTP :exec
UPDATE membres SET totp = ?, totp_dernier = ? WHERE id = ?;

-- name: DesactiverTOTP :exec
UPDATE membres SET totp = '', totp_dernier = 0 WHERE id = ?;

-- Anti-rejeu : le pas ne peut que monter (deux requetes simultanees avec
-- le meme code : une seule passe).
-- name: NoterPasTOTP :execrows
UPDATE membres SET totp_dernier = sqlc.arg(pas) WHERE id = sqlc.arg(id) AND totp_dernier < sqlc.arg(pas);

-- name: EffacerCodesSecours :exec
DELETE FROM codes_secours WHERE membre_id = ?;

-- name: AjouterCodeSecours :exec
INSERT OR IGNORE INTO codes_secours (membre_id, empreinte, cree_le) VALUES (?, ?, ?);

-- name: UtiliserCodeSecours :execrows
UPDATE codes_secours SET utilise_le = ? WHERE membre_id = ? AND empreinte = ? AND utilise_le = 0;

-- name: CodesSecoursRestants :one
SELECT count(*) FROM codes_secours WHERE membre_id = ? AND utilise_le = 0;

-- name: PasskeysMembre :many
SELECT * FROM passkeys WHERE membre_id = ? ORDER BY id;

-- name: PasskeyParIdentifiant :one
SELECT * FROM passkeys WHERE identifiant = ?;

-- name: CompterPasskeys :one
SELECT count(*) FROM passkeys WHERE membre_id = ?;

-- name: AjouterPasskey :exec
INSERT INTO passkeys (membre_id, identifiant, cle_publique, algorithme, compteur, domaine, nom, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: NoterPasskey :exec
UPDATE passkeys SET compteur = ?, utilise_le = ? WHERE id = ?;

-- name: SupprimerPasskey :execrows
DELETE FROM passkeys WHERE id = ? AND membre_id = ?;

-- name: SupprimerPasskeysMembre :exec
DELETE FROM passkeys WHERE membre_id = ?;
