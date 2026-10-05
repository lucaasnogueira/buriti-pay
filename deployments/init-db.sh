#!/bin/sh
set -e

echo "Running database initialization migrations..."
for f in /docker-entrypoint-initdb.d/migrations/*.up.sql; do
    if [ -f "$f" ]; then
        echo "Applying migration: $f"
        psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" -f "$f"
    fi
done
echo "Database migrations applied successfully."