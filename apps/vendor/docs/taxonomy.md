# Catalogue classification

Open Catalogue > food item, Shop product or Shop variant > Classification.

The workspace loads the current published taxonomy, facets, nodes and source classification through the existing vendor taxonomy API. Categories can be searched within a facet; node detail shows the server description, parent grouping and vocabulary terms when returned. Food and Shop remain distinct namespaces.

Editing replaces the selected facet only. Choose at most one primary category, plus secondary and discovery mappings. An empty reviewed selection explicitly clears that facet. Untouched candidates preserve their confidence basis, score and metadata. Changed roles become manual declarations. Active mappings that cannot be resolved against the current publication block editing rather than silently disappearing.

PUT /api/v1/vendor/taxonomy/namespaces/{namespaceCode}/mappings sends only facet_code, source_type, source_ref and mappings. Each candidate uses node_code, mapping_type and confidence_basis, preserving optional confidence_score/metadata for untouched existing candidates. There is no If-Match contract. The app compares a fresh publication/classification read before submission; the public API still has a read-to-write race. The server resolves business/branch ownership and permission scope from the catalogue resource. No backend changes were made.

A journal scoped by mode, vendor, staff and source preserves the exact request and Idempotency-Key before submission. Uncertain outcomes block new edits and offer an identical saved-request retry. A successful replay confirms acceptance; the app reloads the latest classification, which may include later updates. Storage failures block unsafe recovery. Denied or disabled Live requests remain explicit; Mock is never substituted for Live.

Assigned tags and dietary claims show actual names and states, including verification policy and dietary risk. The vendor surface exposes these as reads only: no assignment, removal, evidence upload or verification action is invented. Dietary definitions can also be viewed. Platform taxonomy publication and vocabulary administration remain excluded.

Mock embeds the existing server Food/Shop/semantic seed vocabulary, binds sources to actual Mock catalogue records and records per-facet replacement/replay. Tags are illustrative fixtures. Vocabulary terms are empty in Mock; Live uses the node-detail response. Mock does not certify real taxonomy publication, branch permissions or database transactions. Live requires enabled taxonomy reads/writes and the corresponding vendor.taxonomy.read or vendor.taxonomy.mapping.manage permission. Deployed integration remains pending.
