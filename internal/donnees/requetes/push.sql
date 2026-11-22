-- Notifications Web Push.

-- name: LireServeur :one
SELECT valeur FROM serveur WHERE nom = ?;

-- name: EcrireServeur :exec
INSERT INTO serveur (nom, valeur) VALUES (?, ?) ON CONFLICT (nom) DO NOTHING;

-- name: Abonner :exec
INSERT INTO abonnements_push (membre_id, adresse, p256dh, auth, cree_le) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (adresse) DO UPDATE SET membre_id = excluded.membre_id, p256dh = excluded.p256dh, auth = excluded.auth;

-- name: Desabonner :exec
DELETE FROM abonnements_push WHERE adresse = ? AND membre_id = ?;

-- name: SupprimerAbonnement :exec
DELETE FROM abonnements_push WHERE id = ?;

-- name: CompterAbonnements :one
SELECT count(*) FROM abonnements_push WHERE membre_id = ?;

-- name: AbonnementsFamille :many
SELECT abonnements_push.id, abonnements_push.adresse, abonnements_push.p256dh, abonnements_push.auth,
       membres.id AS membre_id, membres.push_modules
FROM abonnements_push JOIN membres ON membres.id = abonnements_push.membre_id
WHERE membres.famille_id = ? AND membres.actif = 1 AND membres.id <> sqlc.arg(auteur);

-- name: ModifierPushModules :exec
UPDATE membres SET push_modules = ? WHERE id = ?;
