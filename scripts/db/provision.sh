#!/usr/bin/env bash
# Creates the oagent database and its roles on any Postgres; safe to re-run, and a re-run sets the passwords it is given.
#
# Roles: <PREFIX>_owner (NOLOGIN BYPASSRLS, owns the schema, tables and definer functions), <PREFIX>_migrator
# (LOGIN, runs migrations as the owner), <PREFIX>_service (LOGIN NOBYPASSRLS, reads and writes rows for the
# running app, subject to row-level security). verify.template.sql checks the result and fails the run on drift.
#
# Env (asked for interactively when missing):
#   ADMIN_URL         admin connection URL without /<db>, e.g. postgres://postgres:pw@localhost:5432?sslmode=disable
#   DB_NAME           application database (default oagent)
#   PREFIX            role prefix (default oagent)
#   SCHEMA            schema oagent uses, postgres.schema (default public)
#   TABLE_PREFIX      table prefix oagent uses, postgres.tablePrefix (default empty)
#   ADMIN_DB          database used for CREATE DATABASE (default postgres; neondb on Neon)
#   MIG_PW, SVC_PW    migrator and service passwords (16+ characters, no ' or \); "gen" generates one
#   CREDENTIALS_FILE  where generated passwords are written, mode 0600 (default ./oagent-db-credentials.env)
#   PSQL              psql binary (default psql)
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PSQL="${PSQL:-psql}"

is_tty() { [[ -t 0 && -t 2 ]]; }

# with_db "postgres://host:5432/neondb?sslmode=disable" oagent → postgres://host:5432/oagent?sslmode=disable
with_db() {
  local url="$1" db="$2" base query=""
  base="${url%%\?*}"
  if [[ "$url" == *"?"* ]]; then
    query="?${url#*\?}"
  fi
  base="${base%/}"
  if [[ "${base#*://}" == */* ]]; then
    base="${base%/*}"
  fi
  printf '%s/%s%s' "$base" "$db" "$query"
}

# redact hides the password in a postgres URL for display.
redact() { printf '%s' "${1%%\?*}" | sed -E 's#(://[^:/@]+):[^@]*@#\1:****@#'; }

ask_line() {
  local label="$1" default="${2-}" varname="$3" val
  while true; do
    if [[ -n "$default" ]]; then
      read -e -r -p "$label [$default]: " val || true
      val="${val:-$default}"
    else
      read -e -r -p "$label: " val || true
    fi
    [[ -n "$val" ]] && break
    echo "  required" >&2
  done
  printf -v "$varname" '%s' "$val"
}

ask_secret() {
  local label="$1" varname="$2" val
  printf '%s (16+ characters, or "gen" to generate): ' "$label" >&2
  IFS= read -s -r val || true
  printf '\n' >&2
  printf -v "$varname" '%s' "$val"
}

generated=()
resolve_password() {
  local name="$1" value="${!1}"
  if [[ "$value" == "gen" ]]; then
    value="$(openssl rand -base64 48 | tr -dc 'A-Za-z0-9' | head -c 40)"
    generated+=("$name")
  fi
  if [[ ${#value} -lt 16 ]]; then
    echo "$name must be at least 16 characters" >&2
    exit 1
  fi
  if [[ "$value" == *"'"* || "$value" == *\\* ]]; then
    echo "$name must not contain ' or \\" >&2
    exit 1
  fi
  printf -v "$name" '%s' "$value"
}

ADMIN_URL="${ADMIN_URL-}"
DB_NAME="${DB_NAME-}"
PREFIX="${PREFIX-}"
SCHEMA="${SCHEMA-}"
TABLE_PREFIX="${TABLE_PREFIX-}"
ADMIN_DB="${ADMIN_DB-}"
MIG_PW="${MIG_PW-}"
SVC_PW="${SVC_PW-}"
CREDENTIALS_FILE="${CREDENTIALS_FILE:-./oagent-db-credentials.env}"

interactive=false
if [[ -z "$ADMIN_URL" || -z "$MIG_PW" || -z "$SVC_PW" ]]; then
  if ! is_tty; then
    echo "Set ADMIN_URL, MIG_PW and SVC_PW, or run this in a terminal to be asked." >&2
    exit 1
  fi
  interactive=true
  echo "oagent database provisioning"
fi

if [[ -z "$ADMIN_URL" ]]; then
  ask_line "ADMIN_URL (admin user, no /<db>)" "" ADMIN_URL
fi
if $interactive; then
  [[ -n "$DB_NAME" ]] || ask_line "DB_NAME" oagent DB_NAME
  [[ -n "$PREFIX" ]] || ask_line "PREFIX (role prefix)" oagent PREFIX
  [[ -n "$SCHEMA" ]] || ask_line "SCHEMA (postgres.schema)" public SCHEMA
fi
DB_NAME="${DB_NAME:-oagent}"
PREFIX="${PREFIX:-oagent}"
SCHEMA="${SCHEMA:-public}"
if [[ -z "$ADMIN_DB" ]]; then
  ADMIN_DB=postgres
  [[ "$ADMIN_URL" == *neon.tech* ]] && ADMIN_DB=neondb
fi
[[ -n "$MIG_PW" ]] || ask_secret "MIG_PW (${PREFIX}_migrator password)" MIG_PW
[[ -n "$SVC_PW" ]] || ask_secret "SVC_PW (${PREFIX}_service password)" SVC_PW
resolve_password MIG_PW
resolve_password SVC_PW

for pair in "DB_NAME:$DB_NAME" "SCHEMA:$SCHEMA"; do
  if [[ ! "${pair#*:}" =~ ^[a-z_][a-z0-9_]{0,62}$ ]]; then
    echo "${pair%%:*} must match [a-z_][a-z0-9_]{0,62} (got ${pair#*:})" >&2
    exit 1
  fi
done
if [[ ! "$TABLE_PREFIX" =~ ^[a-z0-9_]{0,30}$ ]]; then
  echo "TABLE_PREFIX must match [a-z0-9_]{0,30} (got $TABLE_PREFIX)" >&2
  exit 1
fi
if [[ ! "$PREFIX" =~ ^[a-z][a-z0-9_]{0,40}$ ]]; then
  echo "PREFIX must match [a-z][a-z0-9_]{0,40} (got $PREFIX)" >&2
  exit 1
fi
if (( ${#generated[@]} > 0 )) && [[ -e "$CREDENTIALS_FILE" ]]; then
  echo "$CREDENTIALS_FILE already exists; move it or set CREDENTIALS_FILE" >&2
  exit 1
fi

case "$ADMIN_URL" in
  *-pooler.*|*pooler.supabase.com*|*.proxy-*.rds.amazonaws.com*|*:6432*)
    echo "WARNING: ADMIN_URL looks like a connection pooler; role and ownership changes need a direct connection." >&2
    if $interactive; then
      read -e -r -p "Continue anyway? [y/N] " ans || ans=""
      [[ "$ans" =~ ^(y|Y|yes)$ ]] || { echo "Aborted."; exit 130; }
    fi
    ;;
esac

if $interactive; then
  cat <<EOF

About to provision:
  server    $(redact "$ADMIN_URL")
  database  $DB_NAME (created if missing, via $ADMIN_DB)
  schema    $SCHEMA, owned by ${PREFIX}_owner (table prefix '${TABLE_PREFIX}')
  roles     ${PREFIX}_owner, ${PREFIX}_migrator, ${PREFIX}_service

EOF
  read -e -r -p "Proceed? [y/N] " ans || ans=""
  [[ "$ans" =~ ^(y|Y|yes)$ ]] || { echo "Aborted."; exit 130; }
fi

render() { sed -e "s/@@PREFIX@@/$PREFIX/g" -e "s/@@SCHEMA@@/$SCHEMA/g" -e "s/@@TABLE_PREFIX@@/$TABLE_PREFIX/g" "$1"; }

echo "==> [1/3] database $DB_NAME"
"$PSQL" -qX -v ON_ERROR_STOP=1 "$(with_db "$ADMIN_URL" "$ADMIN_DB")" <<SQL
SELECT format('CREATE DATABASE %I', '$DB_NAME')
 WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '$DB_NAME')\gexec
SQL

echo "==> [2/3] roles, schema and privileges (one transaction)"
{
  # passwords travel on stdin, never on the command line
  printf "\\\\set migrator_password '%s'\n" "$MIG_PW"
  printf "\\\\set service_password '%s'\n" "$SVC_PW"
  echo "BEGIN;"
  render "$script_dir/bootstrap.template.sql"
  echo "COMMIT;"
} | "$PSQL" -qX -v ON_ERROR_STOP=1 "$(with_db "$ADMIN_URL" "$DB_NAME")"

echo "==> [3/3] verify"
render "$script_dir/verify.template.sql" | "$PSQL" -qX -v ON_ERROR_STOP=1 "$(with_db "$ADMIN_URL" "$DB_NAME")"

if (( ${#generated[@]} > 0 )); then
  (
    umask 077
    for name in "${generated[@]}"; do
      printf '%s=%s\n' "$name" "${!name}"
    done > "$CREDENTIALS_FILE"
  )
  echo "==> generated passwords written to $CREDENTIALS_FILE (mode 0600); move them to your secret store and delete the file"
fi

host="$(redact "$ADMIN_URL" | sed -E 's#^[a-z]+://([^@]*@)?##; s#/$##')"
[[ -n "$host" ]] || host="<host>:5432"
cat <<EOF
==> done

Point oagent at it (passwords are the ones you set):
  postgres.dsn:            postgres://${PREFIX}_service:<SVC_PW>@${host}/${DB_NAME}
  postgres.migrator.dsn:   postgres://${PREFIX}_migrator:<MIG_PW>@${host}/${DB_NAME}
  postgres.migrator.role:  ${PREFIX}_owner
  postgres.schema:         ${SCHEMA}
  postgres.tablePrefix:    ${TABLE_PREFIX}
Optionally set postgres.role: ${PREFIX}_service, so oagent refuses to start when the DSN logs in as another role.
Re-check the roles and privileges at any time, for example after migrations:
  sed -e 's/@@PREFIX@@/${PREFIX}/g' -e 's/@@SCHEMA@@/${SCHEMA}/g' -e 's/@@TABLE_PREFIX@@/${TABLE_PREFIX}/g' \\
    scripts/db/verify.template.sql | psql "<ADMIN_URL with /${DB_NAME}>"
Keep the URL's query string (sslmode and so on). oagent migrates on start (db.autoMigrate).
EOF
