#!/bin/sh
# Restauration d'une sauvegarde de FamilyCocon.
#
#   ./scripts/restauration.sh /srv/sauvegardes/familycocon/familycocon-20261122-021500.db
#
# À lancer dans le dossier du docker-compose.yml. Étapes :
#   1. arrêt du conteneur ;
#   2. le binaire vérifie la sauvegarde (intégrité, version) et la met en
#      place ; l'ancienne base est gardée dans le volume
#      (familycocon.db.avant-restauration-…) ;
#   3. redémarrage.
set -eu

FICHIER=${1:?usage : restauration.sh FICHIER.db}
SERVICE=${SERVICE:-familycocon}
[ -f "$FICHIER" ] || { echo "fichier introuvable : $FICHIER" >&2; exit 1; }

# Copie lisible par l'utilisateur du conteneur (uid 65532).
temp=$(mktemp -d)
trap 'rm -rf "$temp"' EXIT
cp "$FICHIER" "$temp/restauration.db"
chmod 755 "$temp"
chmod 644 "$temp/restauration.db"

echo "Arrêt de FamilyCocon…"
docker compose stop "$SERVICE"
echo "Restauration…"
docker compose run --rm --no-deps -v "$temp/restauration.db:/restauration.db:ro" "$SERVICE" -restaurer /restauration.db
echo "Redémarrage…"
docker compose start "$SERVICE"
echo "Fait. Vérifiez : curl -fsS http://127.0.0.1:8080/health"
