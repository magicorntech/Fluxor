#!/usr/bin/env bash
# Runs fluxor-server for local development against a local MongoDB (mongodb://localhost:27017).
#
# The token signing key is created on first use and kept in .dev-secret (git-ignored), so that signed-in
# sessions survive a restart. Override the database with FLUXOR_MONGO_URI / FLUXOR_MONGO_DB.
# On an empty database the first administrator is created with a generated password that is printed
# once in the log: copy it from there, you will be asked to change it at the first login.
set -euo pipefail
cd "$(dirname "$0")/.."
export PATH=/opt/homebrew/bin:$PATH

secret_file=.dev-secret
if [ ! -s "$secret_file" ]; then
  (umask 077 && openssl rand -hex 32 > "$secret_file")
  echo "created $secret_file"
fi

export FLUXOR_JWT_SECRET="$(cat "$secret_file")"
export FLUXOR_MONGO_URI="${FLUXOR_MONGO_URI:-mongodb://localhost:27017}"
export FLUXOR_MONGO_DB="${FLUXOR_MONGO_DB:-fluxor}"
exec go run ./cmd/fluxor-server
