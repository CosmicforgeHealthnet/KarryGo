# Settings templates (no real values)

Each `*.env.example` lists the settings one service needs, with comments. On the
server, the real file lives next to the service's release record, for example
`/opt/karrygo/customer-service/customer.env`, and must be **mode 600** (owner
only). Real files are never committed: `.gitignore` blocks them.

How to fill one in (full walkthrough: `docs/deployment/VPS_DEPLOYMENT_GUIDE.md`, section G):

| Placeholder | What to put |
|---|---|
| `GENERATE` | a new random value: `openssl rand -hex 32` |
| `MUST MATCH ...` | copy the exact value from the named setting of the named service |
| `YOUR ...` | your own account value (for example a Google API key) |

`db.env.example` is the template for each new service's `db.env` (its own
database and Redis passwords). The same database password also appears inside
that service's `*_DATABASE_URL`.
