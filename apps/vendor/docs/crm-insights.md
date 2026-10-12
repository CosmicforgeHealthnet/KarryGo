# Customer insights

Open More > Customers & relationships > Customer insights. The overview shows the backend RFM_V1 and CHURN_V1 models, freshness, scored/unavailable coverage and source/computation times. Business-level staff can choose an actual branch or whole-business results. Current branch scope is preserved; server permission and branch denial remain authoritative.

Distribution screens show combined RFM, recency, frequency and monetary buckets, or churn risk bands, with animated proportional bars and actual counts. Unavailable reasons have their own distribution. Missing data is not rendered as zero risk. Failed/stale/queued/running/current states have distinct semantic colors; unavailable states are neutral. Loading uses skeletons, and failed reloads suppress stale scope data.

Refresh insights opens a confirmation sheet then POSTs model_key, scope_type and optional actual branch_ref to the existing intelligence/rebuild endpoint. No score editor, prediction probability or client-controlled model definition exists. QUEUED/RUNNING disables another rebuild. Manual refresh checks overview status; acceptance does not imply worker completion. Durable CRM recovery reuses the original request and key after interrupted responses and directs unrelated pending edits back to their original screen.

Mock fixtures have no commercial source projection, so initial models are NOT_AVAILABLE. Rebuilds progress on reads to a completed evaluation with unavailable relationships and SOURCE_NOT_AVAILABLE reasons. Histograms remain empty instead of inventing scores. This is a local fixture lifecycle, not a live worker simulation.

Live uses only existing vendor intelligence endpoints. CRM/intelligence flags and the corresponding read/rebuild permissions must be enabled, and the intelligence worker must be deployed/configured to process rebuilds. Writes must be enabled for rebuild requests. No backend changes were needed for these screens.

Focused checks cover lost-response rebuild recovery, unchanged generation on retry, unavailable-score preservation, branch denial, and 360 x 800 layouts for overview/RFM/churn in light and dark themes. Final physical-device, deployed worker and real permission/privacy behavior remain release checks.
