-- Familles et membres.

-- name: CompterFamilles :one
SELECT count(*) FROM familles;

-- name: CreerFamille :one
INSERT INTO familles (nom, fuseau, devise, modules, cree_le) VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: ToutesFamilles :many
SELECT * FROM familles ORDER BY id;

-- name: FamilleParID :one
SELECT * FROM familles WHERE id = ?;

-- name: ModifierFamille :exec
UPDATE familles SET nom = ?, fuseau = ?, devise = ? WHERE id = ?;

-- name: ModifierModules :exec
UPDATE familles SET modules = ? WHERE id = ?;

-- name: CreerMembre :one
INSERT INTO membres (famille_id, nom, role, couleur, identifiant, mdp, pin, push_modules, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
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

-- Parents d'une famille (rappels d'une note cachee aux enfants).
-- name: ParentsDeFamille :many
SELECT id FROM membres WHERE famille_id = ? AND actif = 1 AND role = 'parent' ORDER BY id;

-- name: CompterParents :one
SELECT count(*) FROM membres WHERE famille_id = ? AND actif = 1 AND role = 'parent';

-- name: ModifierNaissance :exec
UPDATE membres SET naissance = ? WHERE id = ? AND famille_id = ?;

-- name: ModifierProfil :exec
UPDATE membres SET nom = ?, couleur = ?, avatar = ? WHERE id = ? AND famille_id = ?;

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

-- Membre retire : ses liens d'invitation ne servent plus (sinon il
-- reviendrait par l'un d'eux).
-- name: SupprimerInvitationsDe :exec
DELETE FROM invitations WHERE famille_id = ? AND cree_par = ?;

-- name: ModifierAccueil :exec
UPDATE membres SET accueil = ? WHERE id = ? AND famille_id = ?;

-- name: ModifierAccueilReglages :exec
UPDATE membres SET accueil_jour_haut = ?, accueil_compact = ? WHERE id = ? AND famille_id = ?;

-- name: ModifierBlocsReplies :exec
UPDATE membres SET accueil_replies = ? WHERE id = ? AND famille_id = ?;

-- name: ModifierHeuresCalmes :exec
UPDATE membres SET calme_debut = ?, calme_fin = ? WHERE id = ? AND famille_id = ?;
