#!/bin/sh
# brainDB container entrypoint.
# The image serves a data directory (one subdirectory per database). On an empty
# volume the server creates an empty database named "default". Bundled examples
# are installed as databases, not baked into the image start:
#   braindb example install sales-company --data-dir /data
#   curl -X POST localhost:7791/api/dbs -H 'Content-Type: application/json' -d '{"example":"sales-company"}'
# BRAINDB_INSTALL (comma-separated, optional "example:dbname") installs before serve.
# BRAINDB_BRAIN selects the old single-file mode, and only when BRAINDB_DATA_DIR is unset.
set -eu

DATA="${BRAINDB_DATA_DIR:-/data}"
EXAMPLES="${BRAINDB_EXAMPLES:-/opt/braindb/examples}"
ADDR="${BRAINDB_ADDR:-0.0.0.0:7791}"
DB="${BRAINDB_DB:-default}"

# Older images wrote one brain at /data/brain.xml. Keep it as the default database.
if [ -z "${BRAINDB_BRAIN:-}" ] || [ -n "${BRAINDB_DATA_DIR:-}" ]; then
  if [ -f "$DATA/brain.xml" ] && [ ! -e "$DATA/default/brain.xml" ]; then
    echo "braindb: moving /data/brain.xml into $DATA/default" >&2
    mkdir -p "$DATA/default"
    mv "$DATA/brain.xml" "$DATA/default/brain.xml"
    [ -f "$DATA/braindb.yaml" ] && mv "$DATA/braindb.yaml" "$DATA/default/braindb.yaml"
    [ -f "$DATA/brain.journal.jsonl" ] && mv "$DATA/brain.journal.jsonl" "$DATA/default/brain.journal.jsonl"
    [ -d "$DATA/templates" ] && mv "$DATA/templates" "$DATA/default/templates"
    [ -d "$DATA/embeddings.cache" ] && mv "$DATA/embeddings.cache" "$DATA/default/embeddings.cache"
  fi
fi

install_requested() {
  [ -n "${BRAINDB_INSTALL:-}" ] || return 0
  list=$(printf '%s' "$BRAINDB_INSTALL" | tr ',' ' ')
  for spec in $list; do
    [ -n "$spec" ] || continue
    ex=$spec
    name=$spec
    case $spec in
      *:*)
        ex=${spec%%:*}
        name=${spec#*:}
        ;;
    esac
    if [ ! -f "$DATA/$name/brain.xml" ]; then
      echo "braindb: installing example $ex as $name" >&2
      braindb example install "$ex" --as "$name" --data-dir "$DATA" --examples "$EXAMPLES"
    fi
  done
}

brain_path() {
  if [ -n "${BRAINDB_BRAIN:-}" ] && [ -z "${BRAINDB_DATA_DIR:-}" ]; then
    printf '%s' "$BRAINDB_BRAIN"
    return
  fi
  printf '%s/%s/brain.xml' "$DATA" "$DB"
}

cmd="${1:-serve}"
[ "$#" -gt 0 ] && shift
case "$cmd" in
  serve)
    install_requested
    if [ -n "${BRAINDB_BRAIN:-}" ] && [ -z "${BRAINDB_DATA_DIR:-}" ]; then
      exec braindb serve --brain "$BRAINDB_BRAIN" --addr "$ADDR" "$@"
    fi
    exec braindb serve --data-dir "$DATA" --addr "$ADDR" "$@"
    ;;
  mcp)
    if [ -n "${BRAINDB_BRAIN:-}" ] && [ -z "${BRAINDB_DATA_DIR:-}" ]; then
      exec braindb mcp --brain "$BRAINDB_BRAIN" --no-http "$@"
    fi
    exec braindb mcp --data-dir "$DATA" --db "$DB" --no-http "$@"
    ;;
  query|fill|context|ingest|state)
    exec braindb "$cmd" --brain "$(brain_path)" "$@"
    ;;
  *)
    exec braindb "$cmd" "$@"
    ;;
esac
