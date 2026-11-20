#!/bin/sh
# brainDB MCP smoke test over stdio.
#   sh scripts/mcp-smoke.sh ./bin/braindb mcp --brain /tmp/brain.xml --no-http
#   sh scripts/mcp-smoke.sh docker compose exec -T braindb braindb-entrypoint mcp
#   sh scripts/mcp-smoke.sh ./dist/acme-sales-brain          # a published brain binary
# Sends initialize → tools/list → brain_view → brain_run (SEARCH, SELECT + SHOW, FILL) and checks every reply.
# Works with the default tool set (brain_view + brain_run); published read-only brains included.
set -u
[ "$#" -gt 0 ] || { echo "usage: $0 <command that starts the MCP server>" >&2; exit 64; }

out=$(cat <<'EOF' | "$@" 2>/dev/null
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"brain_view","arguments":{"format":"md"}}}
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"brain_run","arguments":{"script":"SEARCH \"status and next steps\" MODE hybrid LIMIT 3;"}}}
{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"brain_run","arguments":{"script":"SELECT id, kind, text FROM memory ORDER BY weight DESC LIMIT 3 FORMAT md; SHOW stats;"}}}
{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"brain_run","arguments":{"script":"FILL $$\nowner: \"responsible person:name\"\n$$ AS yaml OUTPUT json;"}}}
EOF
)
fails=0
for id in 1 2 3 4 5 6; do
  line=$(printf '%s\n' "$out" | grep "\"id\":$id[,}]" | head -1)
  if [ -z "$line" ] || printf '%s' "$line" | grep -q '"error"\|"isError":true'; then
    fails=$((fails + 1)); printf '  FAIL id=%s %.300s\n' "$id" "$line"
  else
    printf '  ok   id=%s\n' "$id"
  fi
done
printf '%s' "$out" | grep -q '"brain_run"' || { fails=$((fails + 1)); echo '  FAIL tools/list lacks brain_run'; }
echo "== MCP: $((6 - fails)) ok, $fails failed"
exit "$fails"
