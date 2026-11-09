#!/usr/bin/env bash
# Restaure une sauvegarde chiffrée dans une base VIDE. Refuse d'écraser une base contenant des données.
#
# Usage : DATABASE_URL=... BACKUP_PASSPHRASE=... ./restore.sh <fichier.dump.enc>
set -euo pipefail

: "${DATABASE_URL:?DATABASE_URL est obligatoire}"
: "${BACKUP_PASSPHRASE:?BACKUP_PASSPHRASE est obligatoire}"
input="${1:?Usage : restore.sh <fichier.dump.enc>}"

tables=$(psql "$DATABASE_URL" -Atc "SELECT count(*) FROM pg_tables WHERE schemaname = 'public'")
if [ "$tables" != "0" ]; then
  echo "Refus : la base cible contient déjà $tables tables. Restaurez dans une base vide." >&2
  exit 1
fi

openssl enc -d -aes-256-cbc -pbkdf2 -iter 200000 -pass env:BACKUP_PASSPHRASE -in "$input" |
  pg_restore --no-owner --exit-on-error --dbname="$DATABASE_URL"

echo "Restauration terminée."
