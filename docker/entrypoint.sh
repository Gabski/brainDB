#!/bin/sh
# brainDB container entrypoint.
# - Seeds /data/brain.xml from a bundled example on first start when BRAINDB_EXAMPLE is set
#   (sales-company | research-lab), including the example's docs/ and templates/.
# - Then runs `braindb <args>` with the brain path and listen address from the environment.
set -eu

BRAIN="${BRAINDB_BRAIN:-/data/brain.xml}"
ADDR="${BRAINDB_ADDR:-0.0.0.0:7791}"

if [ ! -f "$BRAIN" ] && [ -n "${BRAINDB_EXAMPLE:-}" ]; then
  EX="/opt/braindb/examples/${BRAINDB_EXAMPLE}"
  if [ ! -f "$EX/seed.json" ]; then
    echo "braindb: unknown example '${BRAINDB_EXAMPLE}' (have: $(ls /opt/braindb/examples | tr '\n' ' '))" >&2
    exit 64
  fi
  echo "braindb: seeding $BRAIN from example ${BRAINDB_EXAMPLE}" >&2
  if [ -d "$EX/docs" ]; then
    braindb seed "$EX/seed.json" -o "$BRAIN" --ingest "$EX/docs"
  else
    braindb seed "$EX/seed.json" -o "$BRAIN"
  fi
  if [ -d "$EX/templates" ] && [ ! -d "$(dirname "$BRAIN")/templates" ]; then
    cp -r "$EX/templates" "$(dirname "$BRAIN")/templates"
  fi
fi

cmd="${1:-serve}"
[ "$#" -gt 0 ] && shift
case "$cmd" in
  serve)        exec braindb serve --brain "$BRAIN" --addr "$ADDR" "$@" ;;
  mcp)          exec braindb mcp --brain "$BRAIN" --no-http "$@" ;;   # use with: docker compose exec -T <svc> braindb-entrypoint mcp
  query|fill|context|ingest|state)
                exec braindb "$cmd" --brain "$BRAIN" "$@" ;;
  *)            exec braindb "$cmd" "$@" ;;
esac
