#!/usr/bin/env bash
# Runs once, on the compose Postgres's first start: provisions the oagent roles with the dev-only passwords from compose.yaml.
set -euo pipefail

ADMIN_URL="postgresql://${POSTGRES_USER}@/?host=/var/run/postgresql" \
  DB_NAME="${POSTGRES_DB}" PREFIX=oagent SCHEMA=public \
  MIG_PW="${OAGENT_DB_MIGRATOR_PASSWORD:?set OAGENT_DB_MIGRATOR_PASSWORD}" \
  SVC_PW="${OAGENT_DB_SERVICE_PASSWORD:?set OAGENT_DB_SERVICE_PASSWORD}" \
  /oagent/scripts/db/provision.sh
