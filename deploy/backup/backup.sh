#!/usr/bin/env bash
# Sauvegarde logique chiffrée de la base. Complément de pgBackRest (WAL + PITR) en production :
# ce dump, portable et hors site, couvre le cas où toute l'infrastructure est perdue.
#
# Usage : DATABASE_URL=... BACKUP_PASSPHRASE=... ./backup.sh <fichier.dump.enc>
set -euo pipefail

: "${DATABASE_URL:?DATABASE_URL est obligatoire}"
: "${BACKUP_PASSPHRASE:?BACKUP_PASSPHRASE est obligatoire}"
output="${1:?Usage : backup.sh <fichier.dump.enc>}"

umask 077
pg_dump --format=custom --no-owner --no-privileges "$DATABASE_URL" |
  openssl enc -aes-256-cbc -pbkdf2 -iter 200000 -salt -pass env:BACKUP_PASSPHRASE -out "$output"

# Un fichier vide ou tronqué ne doit pas passer pour une sauvegarde.
test -s "$output"
echo "Sauvegarde écrite : $output ($(wc -c <"$output") octets)"
