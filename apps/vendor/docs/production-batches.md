# Production policies and batches

Open More > Production batches. Stock policies are selected through an actual recipe, preserving its branch and typed catalogue target. Batches require an actual active recipe version. Draft recipe activation remains in More > Recipes.

## Implemented journeys

- Batch list, status filters, server pagination and detail with ingredient/packaging snapshots.
- Target-scoped stock policy read/write: ON_DEMAND or BATCH_STOCK, prepared inventory reference and integer output-conversion ratio. A missing saved policy uses the backend's explicit default, including version zero. Policy writes use If-Match: 0 for initial creation or the returned version for replacement.
- Creation with actual branch, recipe and active version; optional existing production-job reference. No synthetic job is submitted.
- Draft requirement preview and explicit planning; planned batch start.
- Actual consumption replacement, additive waste with reason code, and additional controlled inventory reservations while planned or running.
- Completion with actual output. Required controlled-material consumption must be recorded first; prepared output must convert to whole inventory units. The server remains authoritative for stock, optional materials, packaging consumption point, shortage, cancellation safety and output posting.
- Reservation evidence and prepared-output read; cancellation with an explicit reason from supported nonterminal states.

Writes persist the exact method, path, query, body, If-Match and idempotency key before transmission. Unconfirmed outcomes pause other production writes and can be retried with the original request after reopening. Bodyless preview/plan/start calls omit their body. Mutation buttons observe known denied routes and only the selected action displays its busy state.

## Mock and Live boundaries

Mock shares real Mock recipes, materials, mappings and generic inventory balances. It supplies a prepared production-component fixture, supports lifecycle and integer-stock reservation/output exercises, and replays accepted commands before version checking. It does not reproduce database transactions, 64-bit overflow limits, all packaging consumption policies, concurrency or external production job events. No active recipe or draft batch is fabricated when the selected recipe lacks an active version.

Prepared inventory choices come from actual generic branch balances. Their public projection lacks item-type/status metadata, so Live validates active PRODUCTION_COMPONENT ownership on the server. Live requires a genuinely configured item; the frontend does not create one through an invented endpoint. Recipe/material discovery and optional job reference discovery inherit the integration boundaries documented in recipes.md and production-materials.md.

No backend endpoint was added. One focused controlled-HTTP check covers completed prepared output after accepted-response loss and reopening without duplicate posting. It is not certification of deployed stock transactions, permissions, job integration or all batch states. The broad regression pass and database-backed Live verification remain pending.
