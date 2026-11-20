# Installing brainDB in popular agents

brainDB is an MCP server. Any agent that speaks MCP can use it, in one of two ways:

| transport | when | endpoint |
|---|---|---|
| **stdio** | the agent runs on the same machine as brainDB (CLI agents, local gateways) | `braindb mcp --brain …` (or `--data-dir … --db …`) |
| **Streamable HTTP** | the agent runs elsewhere (cloud bots, remote gateways, containers) | `https://<your-host>/mcp` with `Authorization: Bearer <token>` |

By default every agent gets two tools: `brain_view` (the lit context) and `brain_run` (a BDQL script).
Then teach the agent how to use them:
1. install the [`braindb-bdql` skill](../skills/braindb-bdql/SKILL.md) where the agent supports skills;
2. otherwise paste the rules block from [AGENTS.md §4](AGENTS.md#4-rules-for-agents-paste-into-claudemd--agentsmd--the-system-prompt) into the agent's instructions.

Paths below use `/opt/braindb/bin/braindb` and `/srv/brains/sales.xml`. Replace them with yours.
Agent CLIs change quickly, so the exact flags of third-party tools are worth checking with their `--help` (see "Verify" in each section).

---

## Claude Code

```bash
claude mcp add braindb -- /opt/braindb/bin/braindb mcp --brain /srv/brains/sales.xml --actor claude
# remote
claude mcp add --transport http braindb https://brain.example.org/mcp --header "Authorization: Bearer $BRAINDB_TOKEN"
# skill
make install-skill                                   # → ./.claude/skills/braindb-bdql
SKILL_DIR=~/.claude/skills make install-skill        # user-wide
```

Verify: `claude mcp list`, then `/mcp` in a session.

## OpenAI Codex CLI

`~/.codex/config.toml`:

```toml
[mcp_servers.braindb]
command = "/opt/braindb/bin/braindb"
args = ["mcp", "--brain", "/srv/brains/sales.xml", "--actor", "codex"]
```

Skills: Codex reads `AGENTS.md`, so paste the rules block from [AGENTS.md §4](AGENTS.md) and point it at
`skills/braindb-bdql/reference/cheatsheet.md`. Verify: `codex mcp list`.

## Cursor

`.cursor/mcp.json` (project) or `~/.cursor/mcp.json` (global):

```json
{ "mcpServers": {
    "braindb": { "command": "/opt/braindb/bin/braindb", "args": ["mcp", "--brain", "/srv/brains/sales.xml", "--actor", "cursor"] },
    "braindb-remote": { "url": "https://brain.example.org/mcp", "headers": { "Authorization": "Bearer <token>" } }
} }
```

Rules: add the AGENTS.md §4 block to `.cursor/rules/braindb.mdc`.

## Gemini CLI

`~/.gemini/settings.json` (or `.gemini/settings.json` in the project):

```json
{ "mcpServers": { "braindb": { "command": "/opt/braindb/bin/braindb", "args": ["mcp", "--brain", "/srv/brains/sales.xml", "--actor", "gemini"] } } }
```

Rules: add the AGENTS.md §4 block to `GEMINI.md`. Verify: `/mcp` in a session.

## Hermes Agent (Nous Research)

Hermes reads MCP servers from `~/.hermes/config.yaml` under `mcp_servers`.

```yaml
mcp_servers:
  braindb:                                   # local, stdio
    command: "/opt/braindb/bin/braindb"
    args: ["mcp", "--brain", "/srv/brains/sales.xml", "--actor", "hermes"]
    tools:
      include: ["brain_view", "brain_run"]   # optional allowlist
  braindb-remote:                            # remote, HTTP
    url: "https://brain.example.org/mcp"
    headers:
      Authorization: "Bearer <token>"
```

- Stdio servers get only the `env` you configure, plus a safe baseline, not your whole shell environment. Put `BRAINDB_*` variables under `env:` when you need them.
- Reload with `/reload-mcp` in a session, or start a new session.
- Rules and skills: add the AGENTS.md §4 block to the agent's instructions. If you already use the skill with Claude Code, `hermes import-agent claude-code` migrates the MCP servers and skills.
- Verify: `hermes mcp test braindb`. If your version lacks it, check `hermes mcp --help`.

## OpenClaw

MCP servers live under `mcp.servers` in `~/.openclaw/openclaw.json`. You can edit them with `openclaw mcp list|show|set|unset`.

```json
{
  "mcp": {
    "servers": {
      "braindb": {
        "command": "/opt/braindb/bin/braindb",
        "args": ["mcp", "--brain", "/srv/brains/sales.xml", "--actor", "openclaw"],
        "toolFilter": { "include": ["brain_view", "brain_run"] }
      },
      "braindb-remote": {
        "url": "https://brain.example.org/mcp",
        "transport": "streamable-http",
        "headers": { "Authorization": "Bearer ${BRAINDB_TOKEN}" }
      }
    }
  }
}
```

Skill (OpenClaw uses the same `SKILL.md` format):

```bash
openclaw skills install ./skills/braindb-bdql              # into the workspace skills/
openclaw skills install ./skills/braindb-bdql --global     # into ~/.openclaw/skills
```

- Changed servers are picked up on the next turn. Skills are rescanned after install.
- Verify: `openclaw mcp show braindb` and `openclaw mcp probe`.
- Review third-party skills before enabling them. Install `braindb-bdql` from this repository, not from an unverified registry copy.

## Grok Build CLI (xAI)

```bash
grok mcp add braindb -- /opt/braindb/bin/braindb mcp --brain /srv/brains/sales.xml --actor grok
grok mcp add braindb-remote --transport http https://brain.example.org/mcp --header "Authorization: Bearer $BRAINDB_TOKEN"
grok mcp doctor braindb
```

Rules: add the AGENTS.md §4 block to the project's agent instructions.
Verify: `grok mcp list`. The flags above follow xAI's docs at the time of writing; confirm with `grok mcp --help`.

## Grok Bot (xAI, cloud)

Grok Bot runs on a cloud computer, so **stdio and `localhost` will not work**. Expose brainDB over HTTPS (see
*Exposing brainDB remotely* below) and add it as a custom MCP server from the chat:

> Add a custom MCP server called **braindb** at **https://brain.example.org/mcp**. Use the header **Authorization** with the value **Bearer &lt;token&gt;**.

- Say "custom server", otherwise it may install a catalog plugin instead. Confirm with "Add it".
- Attach it in chat with `@braindb`. Settings → Plugins → Yours shows it and whether it is enabled.
- Use a **read-scoped token** limited to one database, or a published `readonly` brain, unless the bot really needs to write.
- Rules: paste the AGENTS.md §4 block into the bot's instructions.

## Any other MCP client

| setting | value |
|---|---|
| stdio command | `/opt/braindb/bin/braindb mcp --brain /srv/brains/sales.xml --actor <name>` |
| HTTP URL | `https://<host>/mcp` (Streamable HTTP) |
| header | `Authorization: Bearer <token>` |
| optional header | `X-BrainDB-Database: <db>` |
| tools | `brain_view`, `brain_run`; `--tools all` adds the per-operation tools |
| no MCP at all | `POST /api/run {"script": "…"}`, or `braindb run -e "…"` |

---

## Exposing brainDB remotely (for cloud agents)

1. **Tokens with scopes.** Never expose an admin token to a third-party bot.

   ```yaml
   # braindb.yaml
   server:
     addr: 0.0.0.0:7791
     tokens:
       - {name: grokbot, env: BRAINDB_TOKEN_GROKBOT, scope: read,  dbs: [sales]}
       - {name: hermes,  env: BRAINDB_TOKEN_HERMES,  scope: write, dbs: [sales, lab]}
       - {name: admin,   env: BRAINDB_TOKEN,         scope: admin}
   ```

   The server refuses to listen on a non-loopback address without tokens. Each token name becomes the actor in the journal and the UI Activity feed.
2. **TLS in front.** Put a reverse proxy (Caddy, nginx, Traefik) or a tunnel (Cloudflare Tunnel, Tailscale Funnel, ngrok) in front of `:7791`.
   - Forward `/mcp` with streaming enabled: no response buffering, long read timeouts for SSE.
   - Example with Caddy: `brain.example.org { reverse_proxy 127.0.0.1:7791 { flush_interval -1 } }`.
3. **Or ship knowledge without write access at all.** `braindb publish --mode readonly` makes a binary of the brain.
   Serve it with `--with-ui serve` behind the proxy, or give partners the binary for stdio.
4. **Docker.** The compose file publishes `127.0.0.1:7791` only. Point the proxy or tunnel at it, and set the token variables in `.env`.

## Sources used for third-party configs

- Hermes Agent MCP docs: <https://hermes-agent.nousresearch.com/docs/user-guide/features/mcp>
- OpenClaw MCP config: <https://docs.openclaw.ai/gateway/config-extensions>
- OpenClaw skills: <https://docs.openclaw.ai/tools/skills>
- xAI Grok Build MCP docs (Grok Build CLI): <https://docs.x.ai/build/features/mcp-servers>
- Grok Bot custom MCP servers (third-party guide): <https://composio.dev/content/how-to-add-mcp-servers-to-grok-bot>
