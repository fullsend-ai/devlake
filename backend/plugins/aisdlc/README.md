# AISDLC plugin

The `aisdlc` data-source plugin collects station results and AI usage summaries
published in GitLab results repositories. It preserves the evidence behind each
record and optionally links work-item references to issues already in DevLake.

## Supported data

| Selection | Files | Stored information |
| --- | --- | --- |
| RFE assessment (`rfe_assess`) | `*.result.md` with an RHAIRFE key in the filename | One assessment per file: subject, score/scale, verdict, criteria, title, evidence |
| Strategy pipeline (`strat_pipeline`) | `pipeline-data.json` | One processing result per Strategy: reported status/recommendation, scores, source RFE links, batch execution |
| Strategy fallback | `strat-tasks/*.md` when the batch has no `pipeline-data.json` | Front-matter identifiers, status, title, source RFE links; no inferred execution |
| Usage summaries (`usageMode: summary`) | `<run-directory>/telemetry/usage.json` | Run-level and per-model cost/tokens, activity counts, producer CI identity, timestamps, raw-artifact manifest |

Usage supports the common `agentic-telemetry/v1` contract produced by RFE
Autofixer, Epic Decomposer, and Strategy pipelines using Claude or Fullsend.
Usage can be collected alongside a result parser or on its own.

Epic reports, Autofixer YAML reports, Test Plan results, reviews, originals,
`scores.csv`, and `summary.json` do not currently have result parsers here.
Compressed raw telemetry is referenced by its manifest but is not downloaded.
The plugin does not receive live OTLP or download GitHub Actions artifacts.

## How collection works

1. `collectFiles` resolves the configured branch to a commit and reads that
   commit's tree. Tree listing and downloads use the same revision. It skips
   symlinks and any path segment named `current` or `latest`.
2. Selected artifact bodies are stored in `_tool_aisdlc_raw_files`. Blobs already
   downloaded are reused; their last-seen commit and `collected_at` are updated.
   Downloads are limited to 8 MiB per file.
3. `extractObservations` parses station results.
4. `extractUsage` parses summaries when enabled, retains report revisions, and
   selects one current report per producer run.
5. `linkWorkItems` links a work-item key only when exactly one existing domain
   `issues` row matches. It never creates issues or issue relationships.

This is **branch-tip collection, not Git history traversal**. Historical run
folders still present in the tree are collected. Files already collected remain
available if subsequently removed from Git, with their previous last-seen time.
The collector does not apply a blueprint date window to the repository tree;
use `pathPrefix` to restrict a pilot or backfill.

## Configuration

### Prerequisites

- Enable/build the `aisdlc` plugin in DevLake and apply pending migrations.
- Configure a GitLab connection with access to the results repositories. The
  token must support the GitLab user, project, repository tree, and raw file
  APIs; `read_api` is the usual read-only token scope.
- Keep the GitLab connection available: AISDLC stores only its connection ID
  and reuses its endpoint and encrypted token. It does not store another token.

### Config UI

1. Add an **AISDLC** connection and select the existing GitLab connection.
2. Test and save the connection.
3. In a DevLake project, select the results repositories as AISDLC scopes.
4. Configure each scope's **Station** section:
   - **Tenant**: stable organization/tenant identifier.
   - **Source**: stable identifier for this artifact source. It participates in
     record identity; avoid changing it after ingestion.
   - **Station**: your reporting category, such as `rfe-creator`,
     `epic-decomposer`, or `strategy-creator`. It is explicitly configured,
     never inferred from a repository name.
   - **Parser**: RFE assessment, Strategy pipeline, or **Usage summaries only**.
   - **Usage telemetry**: **Usage summaries** to enable collection.
   - **Branch**: empty uses the repository's default branch.
   - **Path prefix**: optional directory or exact artifact path.
5. Save the scope configuration and run/schedule the project's blueprint.

An empty result parser requires `usageMode: "summary"`. Existing configurations
with missing/empty `usageMode`, or `"off"`, keep usage collection disabled.
There is one scope per results repository per AISDLC connection; enable both
result and usage collection on that scope rather than adding duplicate scopes.
Repository discovery lists projects visible through the GitLab membership API.
The API can register a known repository path directly if discovery omits it.

### Suggested producer configurations

Repository paths below share `redhat/rhel-ai/agentic-ci/`.

| Results repository | Result parser | Usage mode | Example path prefix |
| --- | --- | --- | --- |
| `rfe-assess-data` | `rfe_assess` | `off` | `RHAIRFE` |
| `rfe-autofixer-results` | empty | `summary` | `initiative-rhaistrat/20261118-143311` for a pilot; empty for all subtrees |
| `epic-decomposer-results` | empty | `summary` | empty |
| `strat-pipeline-data` | `strat_pipeline` | `summary` | `RHAISTRAT` |

Use the directories actually published by each producer; prefixes are not
hardcoded. RFE and Initiative runs can share a repository. A scope has one
configured station; if those runs need separate station attribution, choose an
appropriate scoped collection rather than deriving it from a job name.

### API examples

Requests use your normal DevLake authentication. Paths below are relative to
the DevLake API root (typically `/api` through Config UI).

Create a connection with `POST /plugins/aisdlc/connections`:

```json
{
  "name": "AISDLC station artifacts",
  "gitlabConnectionId": 1
}
```

Create a scope configuration with
`POST /plugins/aisdlc/connections/<aisdlc-connection-id>/scope-configs`:

```json
{
  "name": "RFE initiative usage pilot",
  "entities": ["CODE"],
  "tenantId": "rhai",
  "sourceId": "rfe-autofixer-results",
  "stationId": "rfe-creator",
  "displayName": "RFE Creator",
  "parser": "",
  "usageMode": "summary",
  "branch": "main",
  "pathPrefix": "initiative-rhaistrat/20261118-143311"
}
```

Register/associate the scope with
`PUT /plugins/aisdlc/connections/<aisdlc-connection-id>/scopes`.
Replace `42` with the returned scope-configuration ID:

```json
{
  "data": [
    {
      "fullName": "redhat/rhel-ai/agentic-ci/rfe-autofixer-results",
      "name": "RFE Autofixer results",
      "scopeConfigId": 42
    }
  ]
}
```

Attach that scope to a project blueprint through Config UI. The existing
`CODE` entity selection enables the plugin's subtasks, including `extractUsage`.
For an existing configuration, use `PATCH` on its scope-config endpoint to set
`usageMode: "summary"`.

## Usage contract and interpretation

The reference contract is documented in
[`schema/agentic-telemetry-v1.schema.json`](schema/agentic-telemetry-v1.schema.json).
It describes expected producer data. The ingestion parser is deliberately more
permissive at the **field** level so one invalid field does not reject its valid
siblings. Unknown additional fields are ignored.

- Preserve `null` as SQL `NULL`; missing or invalid measurements also become
  `NULL`, with diagnostics. Never replace them with zero.
- `totals` can itself be null. Model measurements can still be retained.
  `has_totals` indicates that the producer supplied a totals object, not that
  every measurement is known.
- Cost uses `DECIMAL(24,6)`, without float conversion or inferred pricing.
  Values requiring more than six decimal places or exceeding the column's
  range are rejected rather than silently rounded.
- Counts must be nonnegative integers within signed 64-bit range. Numeric JSON
  values such as `42.0` are accepted when integral; numeric strings are not.
- Each model has cost, token counts, and API requests. Tool calls, turns, and
  user prompts exist only at run level.
- Claude summaries report API requests and user prompts; reasoning tokens and
  turns are unknown. Fullsend reports turns and may report reasoning tokens;
  total API requests, user prompts, and timestamps are generally unknown.
- The producer selects Claude usage when both input sources exist. DevLake
  ingests the selected summary without combining raw sources.
- Invalid Claude model names may already have become `unknown`; invalid
  Fullsend model entries may already have been omitted by the producer.
  Totals and model breakdowns therefore need not match.
  Known mismatches are logged without changing either value; cost comparisons
  allow for independent six-decimal rounding.
- Producer `ci.project` is the executing project, not the results repository.
  CI IDs remain strings. A job name containing `dry` is not a structured
  dry-run indicator.
- Valid timezone-aware timestamps are normalized to UTC and retain their
  original text. Valid timezone-free timestamps retain their text only.
  Invalid calendar values are dropped. An inverted interval keeps its valid
  endpoints, logs an error, and has no derived `duration_ms`.
- The interval is the producer's recorded telemetry interval, not necessarily
  CI wall time. Folder timestamps are not normalized as UTC.
- The raw manifest's SHA-256 describes cleaned **decompressed** bytes.
  `removed_attributes` is optional and may include dropped structural fields
  or lines. Raw-file availability is not required for summary ingestion.

Some producer versions turn missing Claude measurements into zero. DevLake
cannot recover that distinction from the summary and preserves the reported
zero. Raw request/spans/metrics describe overlapping activity and must not be
added to summary totals.

## Storage, identity, and replay

All AISDLC data is currently in plugin-owned tool tables:

| Table suffix (prefix `_tool_aisdlc_`) | Purpose |
| --- | --- |
| `connections`, `scopes`, `scope_configs` | Connection and scope configuration |
| `raw_files` | Collected artifact bodies, path, blob, commit, last-seen time |
| `checkpoints` | Examined revision, selected file count, collection/extraction diagnostics |
| `observations`, `executions` | Work-item station results and source-identified batches |
| `work_items`, `item_links` | Source-qualified item references and evidenced input/output links |
| `evidence`, `unknowns`, `times` | Supporting artifact links, missing-value explanations, source timestamp precision |
| `usage_reports` | Versioned summaries, typed totals, CI metadata, manifest, validation state |
| `usage_models` | Model breakdowns belonging to a report revision |
| `usage_runs` | One authoritative report pointer per source-qualified producer execution |

Station observations retain the existing identity of source, blob, path, field
selector, and observation kind. Usage report identity includes tenant, source,
results repository, path, and blob; rows are also connection-qualified.

For usage:

1. For each artifact path, the most recently collected blob is current.
2. When CI project and job ID are available, execution identity uses those
   values plus the producer run ID, tenant, source, and results repository.
   Otherwise, it falls back to the source-qualified artifact run directory.
3. Identical normalized copies of one execution contribute once. Conflicting
   copies set `usage_runs.conflicted`; their evidence remains available, but
   aggregate queries must exclude them.
4. Re-ingesting an unchanged blob updates evidence without adding usage.
   Corrections keep the old report revision and move the current pointer.
5. An unreadable newer revision gets a provenance-only report; the collector
   does not silently resurrect an older revision's cost.
6. A distinct CI retry published in a separate run directory remains distinct.
   If a producer overwrites one directory, it is treated as a revision of that
   artifact; retained report history is not a complete CI attempt inventory.

Report/model writes and current-pointer replacement are transactional. All
scope-owned rows carry DevLake scope-cleanup metadata. Shared work-item rows
remain connection-owned, as before. Disabling collection does not erase
previously collected data; use the normal scope-data deletion flow for that.

## Error handling and troubleshooting

Producer data problems are logged at **error** level and ingestion continues:

- Missing fields, incorrect types, negative/fractional counts, invalid names,
  invalid timestamps, and malformed manifest fields: keep valid siblings.
- Bare Python JSON constants `NaN`, `Infinity`, and `-Infinity`: replace only
  those unquoted values with null before parsing and log the repair.
- Other malformed JSON, an unavailable body, or an unsupported schema version:
  keep provenance and diagnostics, without guessing measurements.
- Per-file download errors: log and record the observed revision with an empty
  body; continue other files and retry that body on the next collection.
  Empty RFE result bodies are file errors, not scoreless assessments.
- When every selected download fails, or every selected result or usage summary fails,
  the final `checkDownloads` subtask fails the pipeline **after** valid files
  have been extracted and linked. Mixed outcomes remain successful with
  checkpoint diagnostics and error logs.
- Malformed station-result files: log and continue other valid observations.

Authentication, branch/tree discovery, cancellation, and database failures still
fail collection when reliable progress cannot be made. Failed database writes
are rolled back. Checkpoint messages contain a bounded diagnostic count and
sample; complete per-file download/parse errors remain in logs, and the current
usage report's
`validation_state`/`diagnostics` explain partial ingestion even when a subtask
succeeds. Historical bad revisions retain their diagnostics but are not logged
again once superseded.

Common checks:

- **No summaries:** verify `usageMode`, prefix, branch, and repository access.
  Producers may skip publication for idle/failed runs or telemetry failures.
- **Wrong station:** check the explicit scope configuration; no station is
  inferred from producer names.
- **Null metrics:** inspect diagnostics, the original summary, and its `source`.
  Unknown fields are normal for some execution paths.
- **Unlinked Jira keys:** collect the relevant Jira issues and check for
  ambiguous duplicate keys; the plugin links only a unique exact match.
- **Totals too large:** ensure queries use `usage_runs`, exclude conflicts,
  and do not multiply totals through joins to models or work items.

## Example queries (MySQL and PostgreSQL)

Reported cost and measurement coverage by station:

```sql
SELECT r.tenant_id, r.station_id,
       COUNT(*) AS summary_runs,
       COUNT(r.cost_usd) AS runs_with_reported_cost,
       SUM(r.cost_usd) AS reported_cost_usd,
       SUM(r.input_tokens) AS input_tokens,
       SUM(r.output_tokens) AS output_tokens,
       SUM(r.cache_read_tokens) AS cache_read_tokens,
       SUM(r.cache_creation_tokens) AS cache_creation_tokens
FROM _tool_aisdlc_usage_runs u
JOIN _tool_aisdlc_usage_reports r
  ON r.connection_id = u.connection_id AND r.id = u.report_id
WHERE u.conflicted = false
GROUP BY r.tenant_id, r.station_id;
```

Model breakdown, independently of totals:

```sql
SELECT r.tenant_id, r.station_id, m.model,
       SUM(m.cost_usd) AS reported_cost_usd,
       SUM(m.output_tokens) AS output_tokens
FROM _tool_aisdlc_usage_runs u
JOIN _tool_aisdlc_usage_reports r
  ON r.connection_id = u.connection_id AND r.id = u.report_id
JOIN _tool_aisdlc_usage_models m
  ON m.connection_id = r.connection_id AND m.report_id = r.id
WHERE u.conflicted = false
GROUP BY r.tenant_id, r.station_id, m.model;
```

Apply tenant/project/connection and time filters appropriate to your dashboard.
Fullsend summaries may have no event timestamps; `collected_at` measures
collection freshness, not execution time. Summary counts do not measure all
CI runs or failure rates. Use CI records for those metrics. A run may cover
multiple work items; do not allocate its entire cost to each item.

## Development and verification

From `backend/`:

```sh
go test ./plugins/aisdlc/...
go vet ./plugins/aisdlc/...
golangci-lint run ./plugins/aisdlc/...
```

Database lifecycle tests run when `AISDLC_TEST_MYSQL_DSN` and/or
`AISDLC_TEST_POSTGRES_DSN` are set to **disposable databases**. They apply the
plugin's real migrations and verify decimal/null persistence, replay,
corrections, model replacement, and scope cleanup:

```sh
AISDLC_TEST_MYSQL_DSN='root:password@tcp(127.0.0.1:23306)/aisdlc_test?parseTime=true' \
go test ./plugins/aisdlc/tasks -run TestUsageDatabaseLifecycle -v
```

Source layout and contributor conventions are in [AGENTS.md](AGENTS.md).
