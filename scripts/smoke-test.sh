#!/bin/sh
# brainDB end-to-end smoke test against running services (HTTP API).
#   TARGETS="http://127.0.0.1:7791 http://127.0.0.1:7792" sh scripts/smoke-test.sh
# Needs: curl. Honors BRAINDB_TOKEN. Exit code = number of failed checks.
set -u
TARGETS="${TARGETS:-http://127.0.0.1:7791}"
AUTH=""
[ -n "${BRAINDB_TOKEN:-}" ] && AUTH="Authorization: Bearer ${BRAINDB_TOKEN}"
fails=0; passes=0

req() { # method url [json-body] → prints "<status>\n<body>"
  if [ -n "${3:-}" ]; then
    curl -s -o /tmp/body -w '%{http_code}' -X "$1" ${AUTH:+-H "$AUTH"} -H 'Content-Type: application/json' --data "$3" "$2"
  else
    curl -s -o /tmp/body -w '%{http_code}' -X "$1" ${AUTH:+-H "$AUTH"} "$2"
  fi
}
check() { # name expected-status expected-substring method url [body]
  name="$1"; want="$2"; needle="$3"; shift 3
  code=$(req "$@"); body=$(cat /tmp/body 2>/dev/null || true)
  if [ "$code" = "$want" ] && printf '%s' "$body" | grep -q -- "$needle"; then
    passes=$((passes + 1)); printf '  ok   %s\n' "$name"
  else
    fails=$((fails + 1)); printf '  FAIL %s (status %s, want %s, missing %s)\n       %.300s\n' "$name" "$code" "$want" "$needle" "$body"
  fi
}

for T in $TARGETS; do
  echo "== $T"
  check "health"                  200 '"status":"ok"'   GET  "$T/api/health"
  check "ui index"                200 '<'               GET  "$T/"
  check "state has nodes"         200 '"nodes":\['      GET  "$T/api/state"
  check "context markdown"        200 '# '              GET  "$T/api/context.md"
  check "context xml"             200 '<context'        GET  "$T/api/context.xml"
  check "brain xml"               200 '<brain'          GET  "$T/api/brain.xml"
  check "search hybrid"           200 '"rows":\['       POST "$T/api/search" '{"query":"status and next steps","mode":"hybrid","k":5}'
  check "search vector"           200 '"rows":\['       POST "$T/api/search" '{"query":"status and next steps","mode":"vector","k":5}'
  check "search tree"             200 '"rows":\['       POST "$T/api/search" '{"query":"status and next steps","mode":"tree"}'
  check "search graph"            200 '"rows":\['       POST "$T/api/search" '{"query":"status and next steps","mode":"graph"}'
  check "raw query (compact)"     200 '"rows":\['       POST "$T/api/query" '{"query":"sort:-weight limit:5","format":"md"}'
  check "context query"           200 '"'               POST "$T/api/context/query" '{"query":"risks","depth":2,"format":"json"}'
  check "templates list"          200 '\['              GET  "$T/api/templates"
  check "template fill"           200 '"output"'        POST "$T/api/fill" '{"template":"summary: \"current status:text\"\nowner: \"responsible person:name\"\n","templateFormat":"yaml","output":"json"}'
  check "inspect discover"        200 '"discover"'      GET  "$T/api/inspect?what=discover"
  check "layout tree"             200 '"positions"'     GET  "$T/api/layout?view=tree"
  check "exec remember"           200 '"changes"'       POST "$T/api/exec" '{"ops":[{"op":"remember","kind":"note","text":"smoke test note","concepts":["smoke test"]}]}'
  check "exec atomic failure"     400 'error'           POST "$T/api/exec" '{"ops":[{"op":"boost","id":"does-not-exist"}]}'
  check "journal"                 200 '\['              GET  "$T/api/journal"
done

echo "== $passes passed, $fails failed"
exit "$fails"
