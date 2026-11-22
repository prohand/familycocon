-- Familles et membres.

-- name: CompterFamilles :one
SELECT count(*) FROM familles;

-- name: CreerFamille :one
INSERT INTO familles (nom, fuseau, devise, cree_le) VALUES (?, ?, ?, ?)
RETURNING *;

-- name: FamilleParID :one
SELECT * FROM familles WHERE id = ?;

-- name: ModifierFamille :exec
UPDATE familles SET nom = ?, fuseau = ?, devise = ? WHERE id = ?;

-- name: ModifierModules :exec
UPDATE familles SET modules = ? WHERE id = ?;

-- name: CreerMembre :one
INSERT INTO membres (famille_id, nom, role, couleur, identifiant, mdp, pin, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: MembreParIdentifiant :one
SELECT * FROM membres WHERE identifiant = ? AND actif = 1;

-- name: MembreParID :one
SELECT * FROM membres WHERE id = ? AND famille_id = ? AND actif = 1;

-- name: MembresDeFamille :many
SELECT * FROM membres WHERE famille_id = ? AND actif = 1
ORDER BY CASE role WHEN 'parent' THEN 0 ELSE 1 END, id;

-- name: EnfantsAvecPin :many
SELECT * FROM membres WHERE famille_id = ? AND actif = 1 AND role = 'enfant' AND pin IS NOT NULL
ORDER BY id;

-- name: CompterParents :one
SELECT count(*) FROM membres WHERE famille_id = ? AND actif = 1 AND role = 'parent';

-- name: ModifierProfil :exec
UPDATE membres SET nom = ?, couleur = ? WHERE id = ? AND famille_id = ?;

-- name: ModifierMdp :exec
UPDATE membres SET mdp = ? WHERE id = ?;

-- name: ModifierPin :exec
UPDATE membres SET pin = ? WHERE id = ? AND famille_id = ?;

-- name: ModifierRaccourcis :exec
UPDATE membres SET raccourcis = ? WHERE id = ?;

-- Un membre retire n'est pas efface (l'historique " qui a fait quoi " reste
-- lisible) : il est desactive, perd ses identifiants et ses sessions.
-- name: RetirerMembre :exec
UPDATE membres SET actif = 0, identifiant = NULL, mdp = NULL, pin = NULL, totp = '', totp_dernier = 0
WHERE id = ? AND famille_id = ?;
