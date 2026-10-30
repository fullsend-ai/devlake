# Upstream Divergence Tracking

This file tracks modifications to files originating from [apache/incubator-devlake](https://github.com/apache/devlake)
that must be maintained during upstream syncs.

Owned plugins (`aireview`, `codecov`, `testregistry`, `agentready`, `langfuse`, `jira_snowflake`) are additions,
not modifications, and are not tracked here.

`backend/pkg/gcshelper` is an owned addition, not a modification, and is not tracked here.
`oidchelper` now lives in upstream `backend/helpers/oidchelper`; see the auth section.
`jira_snowflake/tasks/convert_*.go` are adapted copies of `jira/tasks/` convertors — see the
[jira_snowflake AGENTS.md](../backend/plugins/jira_snowflake/AGENTS.md) for the diff details.

## gitextractor: ForceFullClone / FORCE_FULL_GIT_HISTORY

**Files:**
- `backend/plugins/gitextractor/impl/impl.go`
- `backend/plugins/gitextractor/parser/clone_gitcli.go`
- `backend/plugins/gitextractor/parser/taskdata.go`
- `env.example`

**Reason:** Upstream gitextractor incremental syncs miss commits via `--shallow-since`.
Adds a separate `ForceFullClone` strategy that bypasses shallow cloning entirely,
controlled by the `FORCE_FULL_GIT_HISTORY` environment variable. Also fixes a temp
directory leak in `doubleClone()`.

**Upstream status:** Pending
**Upstream PR:** none yet
**Owner:** @kpiwko

**Rebase notes:** Touches clone strategy selection in `clone_gitcli.go`.
Watch for upstream changes to `CloneRepo()`, `shallowClone()`, or `doubleClone()`.

## gitlab: Map WorkInProgress to IsDraft in MR converter

**Files:**
- `backend/plugins/gitlab/tasks/mr_convertor.go`

**Reason:** `WorkInProgress` (from the GitLab API `work_in_progress` field) was extracted and
stored in `_tool_gitlab_merge_requests` but never forwarded to `code.PullRequest.IsDraft`
in the converter. This meant draft/WIP MRs were indistinguishable from non-draft MRs at the
domain layer. One-liner fix: `IsDraft: gitlabMr.WorkInProgress`.

**Upstream status:** Merged in apache/devlake#8982. No outstanding fork delta.
**Upstream PR:** https://github.com/apache/devlake/pull/8982
**Owner:** @fmuntean

**Rebase notes:** Change is isolated to the struct literal in `mr_convertor.go`'s `Convert` func.
No conflicts expected unless upstream touches the same field mapping block.

## server/api/auth: OIDC authentication

**Files:**
- `backend/server/api/auth/auth.go`
- `backend/server/api/auth/middleware.go`
- `backend/server/api/auth/handlers_test.go`
- `backend/server/api/auth/store.go`
- `backend/server/api/auth/cleanup.go`
- `backend/server/api/auth/revocation_cache.go`
- `backend/server/api/auth/revocation_cache_test.go`
- `backend/server/api/auth/auth_test.go`
- `config-ui/src/utils/request.ts` (401 interceptor; skip plugin `/test` so remote credential failures do not redirect to `/login`)
- `env.example` (OIDC/auth env var documentation)

**Reason:** Upstream now includes OIDC login. The remaining fork behavior is that auth is
enabled by default; `AUTH_ENABLED=false` is required to opt out (e.g. in local dev or CI).
The config-ui 401 interceptor still skips plugin `/test` so a remote credential failure
does not redirect to `/login`.

The supporting library now lives in upstream `backend/helpers/oidchelper/` (allowlists included). This fork keeps `AUTH_ENABLED` defaulting to true unless it is explicitly false; that default was re-applied on top of upstream's opt-in default. Do not move the package back to `backend/pkg/oidchelper/`.

**Upstream status:** Auth itself is upstream (`#8854`, `#8984`, `#9147`). Outstanding delta is the default-on `AUTH_ENABLED` and the plugin `/test` 401 exception.
**Upstream PR:** https://github.com/apache/devlake/pull/8854
**Owner:** @fmuntean

**Rebase notes:**
- `auth.go` / `middleware.go` wire into `backend/server/api/api.go` via `auth.Init()` and three
  `router.Use()` calls — watch for upstream changes to `api.go`'s middleware chain.
- `store.go` introduces the `auth_sessions` table; no upstream equivalent.
- `env.example` additions are at the end of the file and unlikely to conflict.

## jira: Scope collectParentIssues to current board

**Files:**
- `backend/plugins/jira/tasks/parent_issue_collector.go`
- `backend/plugins/jira/impl/impl.go`

**Reason:** collectParentIssues queries all issues on the Jira connection for epic keys
(filtering by connection_id only). Scoped the epic key query to the current board via board_id filter.

**Upstream status:** N/A — collectParentIssues is Konflux-specific (commit f1c634d), not present in upstream Apache DevLake.
**Upstream PR:** none — not applicable
**Owner:** @cmulliga

**Rebase notes:** `parent_issue_collector.go` is Konflux-only, no upstream conflicts expected.
`impl.go` has a Konflux addition (`CollectParentIssuesMeta` in `SubTaskMetas()`) — watch for upstream changes to the subtask registration list.
## archived/base.go: inline Unsigned constraint

**Files:**
- `backend/core/models/migrationscripts/archived/base.go`

**Reason:** `golang.org/x/exp/constraints` was imported only for `constraints.Unsigned` in
`GenericModel`. Recent versions of `golang.org/x/exp` require Go 1.23+ (they import the
standard `cmp` package added in Go 1.21). The CI environment runs an older Go, so the
transitive dependency chain through `core/runner` → `archived/base.go` caused a `typecheck`
failure in golangci-lint for any PR that introduces a new plugin main package.

Replaced `constraints.Unsigned` with a locally-defined `unsignedInteger` interface that has
identical semantics, eliminating the `golang.org/x/exp` import entirely.

**Upstream status:** Merged in apache/devlake#9032. No outstanding fork delta.
**Upstream PR:** https://github.com/apache/devlake/pull/9032
**Owner:** @fmuntean

**Rebase notes:** If upstream changes `GenericModel`, check whether they still reference
`golang.org/x/exp/constraints` and reapply the inline if needed.

## docker-compose-dev: local UBI image builds

**Files:**
- `docker-compose-dev.yml`
- `Makefile` (root `build-config-ui-image`)
- `backend/Makefile` (`build-server-image`)
- `backend/Dockerfile`, `config-ui/Dockerfile` (DEPRECATED comments only)

**Reason:** Points `devlake` and `config-ui` builds at UBI Containerfiles
(`dockerfile: Containerfile`) and tags images as `localhost/devlake-*:local`
instead of personal Quay tags / Apache Scarf images. Makefile image targets use
`Containerfile`. Debian Dockerfiles are marked DEPRECATED pending removal.
Konflux Tekton builds `backend/Containerfile` and `config-ui/Containerfile`
(fork-only `backend/Dockerfile.konflux` removed).

**Upstream status:** N/A — Konflux local-dev / image-build customization.
**Upstream PR:** none — not applicable
**Owner:** @fmuntean

**Rebase notes:** If upstream regenerates `docker-compose-dev.yml`, re-add under
`devlake` / `config-ui`: `dockerfile: Containerfile` and
`image: localhost/devlake-backend:local` / `localhost/devlake-frontend:local`.
If upstream changes Makefile image targets or Dockerfiles, re-point builds at
`Containerfile` and keep DEPRECATED headers until the Debian files are deleted.

(`backend/Containerfile` and `config-ui/Containerfile` are fork additions, not
tracked here.) `backend/Containerfile` must build libgit2 1.5.0. git2go v34
rejects 1.3.2, which breaks the coverage image when it compiles gitextractor.

## Hadolint shell quoting

**Files:**
- `backend/Dockerfile.local`

**Reason:** The Dockerfile lint job fails at warning level. `make -j$(nproc)`
trips shellcheck SC2046.

**Upstream status:** Pending.
**Upstream PR:** none
**Owner:** @fmuntean

**Rebase notes:** Keep `make -j"$(nproc)"`. Do not drop the quotes.

## Framework migration: idempotent cicd_pipeline_commits rename

**Files:**
- `backend/core/models/migrationscripts/20220915_rename_pipeline_commits.go`

**Reason:** Plugin e2e tests AutoMigrate domain tables on the shared e2e database
without writing `_devlake_migration_history`. When testregistry starts the API
server, pending migrations try to `RENAME cicd_pipeline_repos TO
cicd_pipeline_commits` even though `cicd_pipeline_commits` already exists.

**Upstream status:** Pending — safe idempotent guard; worth proposing upstream.
**Upstream PR:** none
**Owner:** @fmuntean

**Rebase notes:** Keep the `HasTable` / `HasColumn` guards. Do not restore an
unconditional `RenameTable`.

## Framework migration: skip cicd_pipeline_commits repo rename when applied

**Files:**
- `backend/core/models/migrationscripts/20230406_rename_cicd_pipeline_commits_repo_url.go`

**Reason:** Domain AutoMigrate on the shared e2e database creates
`cicd_pipeline_commits` with `repo_url` before this migration runs, so
`RENAME repo TO repo_url` fails on Postgres when `repo` is absent.

**Upstream status:** Pending — idempotent guard.
**Upstream PR:** none
**Owner:** @fmuntean

**Rebase notes:** Rename only when column `repo` still exists.

## Framework migration: keep cicd_scope_id indexable on shared e2e DB

**Files:**
- `backend/core/models/migrationscripts/20221207_add_cicd_scope_drop_builds_jobs.go`

**Reason:** When `cicd_tasks` already exists from plugin e2e AutoMigrate, this
script's partial AutoMigrate used an untyped `CicdScopeId`, which GORM widened
to `longtext` and broke the indexed column on MySQL (Error 1170).

**Upstream status:** Pending — migration snapshot should match domain types.
**Upstream PR:** none
**Owner:** @fmuntean

**Rebase notes:** Keep `gorm:"index;type:varchar(255)"` on `CicdScopeId` in
the 20221107 snapshot structs.

## Framework migration: skip project_pr_metrics deployment rename when applied

**Files:**
- `backend/core/models/migrationscripts/20230416_rename_project_pr_metrict_deployment_id.go`

**Reason:** Plugin e2e AutoMigrate creates `project_pr_metrics` with
`deployment_commit_id` before framework migrations run. Replaying
`20221111` can add legacy `deployment_id`; renaming then fails on Postgres
because the target column already exists.

**Upstream status:** Pending — idempotent rename / drop duplicate column.
**Upstream PR:** none
**Owner:** @fmuntean

**Rebase notes:** Rename only when `deployment_id` exists and
`deployment_commit_id` does not; drop `deployment_id` when both exist.

## Framework migration: skip finished_commits_diffs rename when applied

**Files:**
- `backend/core/models/migrationscripts/20230516_rename_finished_commits_diffs.go`

**Reason:** Refdiff e2e and the refdiff tool model AutoMigrate
`_tool_refdiff_finished_commits_diffs` on the shared e2e database before this
framework rename runs.

**Upstream status:** Pending — idempotent guard.
**Upstream PR:** none
**Owner:** @fmuntean

**Rebase notes:** Rename only when `finished_commits_diffs` exists and
`_tool_refdiff_finished_commits_diffs` does not.

## jira: cleanup stale board associations after tickets leave/change board

**Files:**
- `backend/plugins/jira/impl/impl.go`
- `backend/plugins/jira/tasks/stale_board_issue_cleaner.go`
- `backend/plugins/jira/tasks/stale_board_issue_cleaner_test.go`

**Reason:** Incremental Jira collection only adds board associations; it never removes
issues that left the board or moved to a different team. Stale rows in
`_tool_jira_board_issues` / domain `board_issues` keep those tickets on the old
board in metrics. Adds `cleanupStaleBoardIssues`, which batch-checks membership
via `agile/1.0/board/:id/issue` JQL (`issue IN (...)`, 100 per request),
re-fetches current issue state, and deletes the stale associations.

**Upstream status:** N/A — Konflux-specific subtask, not present in upstream Apache DevLake. Also depends on `collectAndExtractSingleIssue` in `parent_issue_collector.go` (Konflux-only).
**Upstream PR:** none — not applicable
**Owner:** @rsoaresd

**Rebase notes:** `stale_board_issue_cleaner.go` specific use case; no upstream equivalent.
`impl.go` registers `CleanupStaleBoardIssuesMeta` in `SubTaskMetas()` after
`ExtractEpicsMeta` — same conflict hotspot as `CollectParentIssuesMeta`.
Cleanup reuses `collectAndExtractSingleIssue` from `parent_issue_collector.go`
(also Konflux-only).

## core: IsBot field on domain accounts model

**Files:**
- `backend/core/models/domainlayer/crossdomain/account.go`
- `backend/core/models/migrationscripts/20260715_add_is_bot_to_accounts.go` (new file)
- `backend/core/models/migrationscripts/register.go` (append-only)

**Reason:** Added `IsBot bool` to the domain `Account` struct so GitHub/GitLab
account convertors can flag bot identity once, instead of every downstream
consumer (n8n workflows, Grafana dashboards) re-deriving it via provider-specific
`author_name LIKE '%[bot]%'`-style patterns. See
`docs/research/bot-commit-identification.md` for the full analysis. Tracked in
DPROD-1342.

**Upstream status:** Pending — not proposed upstream. This is a Konflux-specific
domain-model addition; upstream has no equivalent field.
**Upstream PR:** none
**Owner:** @kpiwko

**Rebase notes:** Pure field addition, low conflict risk. Migration file is new
(no conflict); `register.go` only needs an append at the end of the `All()` slice.

## github: IsBot field in account convertor

**Files:**
- `backend/plugins/github/tasks/account_convertor.go`

**Reason:** Populate the new `IsBot` field via `isBotAccount()` (API
`Type == "Bot"`, `[bot]`/`-bot`/`-robot` suffixes, or exact logins
`copilot`/`dependabot`/`github-actions`/`codecov-commenter`).

**History:** this fork previously carried a much larger divergence here — a
two-pass design (`ConvertAccounts` + a separate `convertOrphanedRepoAccounts`
pass) introduced by `cd9928159` ("fix: populate bot account identity...",
2026-05-13, DPROD-1259) to handle accounts referenced only via
`_tool_github_repo_accounts` with no matching `_tool_github_accounts` row.
That divergence was never logged here. While implementing `IsBot`, we found
upstream had independently fixed the exact same problem
([apache/devlake#8894](https://github.com/apache/devlake/pull/8894), fixes
[#8886](https://github.com/apache/devlake/issues/8886), merged 2026-06-12) via
a more complete, unified-query design (also fixing `_raw_data` provenance,
which our two-pass patch had broken). We adopted upstream's version verbatim
instead of extending our own — `account_convertor.go` now matches upstream's
`ConvertAccounts` exactly except for `IsBot` population, and the fork's
`convertOrphanedRepoAccounts`/`buildOrphanDomainAccount` are gone.

**Upstream status:** Pending — `IsBot` itself is not proposed upstream (the
underlying query/struct now match upstream exactly, via #8894).
**Upstream PR:** none yet (fork proposal: konflux-ci/devlake#118)
**Owner:** @kpiwko

**Rebase notes:** Low conflict risk — the query/struct match upstream's #8894
version exactly (`Type` field + its `COALESCE(ga.type, '')` select and the
`IsBot` line in `buildDomainAccount` are the only additions). Future upstream
changes to `ConvertAccounts` should apply cleanly except around those two
insertion points.

## github: zero-ID guard and merged_by emission — adopted from upstream, no outstanding divergence

**Files:**
- `backend/plugins/github/tasks/pr_convertor.go`
- `backend/plugins/github/tasks/pr_extractor.go`

**Reason:** Both files previously diverged from upstream (missing zero-ID guard on
`AuthorId`/`MergedById`; missing `merged_by` → `_tool_github_repo_accounts`
emission), both fixed by
[apache/devlake#8894](https://github.com/apache/devlake/pull/8894).

- `pr_extractor.go` is byte-identical to upstream (verified via diff against
  `apache/incubator-devlake`) — no outstanding divergence.
- `pr_convertor.go` is behavior-identical (same zero-ID guard semantics) but
  not byte-identical: we extracted the check into a named `hasValidAccountId()`
  helper for unit-testability, which upstream doesn't have. This is a
  deliberate, cosmetic divergence, not a functional one — worth a quick glance
  on future upstream syncs to `pr_convertor.go`, but not a rebase risk.

No entry needed going forward for either file — listed here only for
traceability since the fix landed alongside the `IsBot` work above.

**Upstream status:** N/A — functionally matches upstream, zero behavioral divergence.
**Owner:** @kpiwko

## gitlab: Bot identity (IsBot) in account convertor

**Files:**
- `backend/plugins/gitlab/tasks/account_convertor.go`

**Reason:** Populate `IsBot` via `isGitlabBotAccount()`. GitLab accounts have no
API-reported type field (unlike GitHub), so detection is username/name-pattern
only: `project_<id>_bot...`/`group_<id>_bot...` access-token prefixes,
`-bot`/`-robot` suffixes, and "Service Account" in the display name.

**Upstream status:** Pending — not proposed upstream.
**Upstream PR:** none yet (fork proposal: konflux-ci/devlake#118)
**Owner:** @kpiwko

**Rebase notes:** Upstream's `account_convertor.go` for GitLab is currently
unchanged from the version this fork started from (single `Convert` closure,
no orphan handling) — low conflict risk, isolated to the `Convert` closure body.

## core: ai_commits domain table

**Files:**
- `backend/core/models/domainlayer/code/ai_commit.go` (new file)
- `backend/core/models/domainlayer/domaininfo/domaininfo.go` (append)
- `backend/core/models/migrationscripts/20260826_add_ai_commits_domain.go` (new file)
- `backend/core/models/migrationscripts/register.go` (append-only)

**Reason:** Project-scoped domain table for commits classified as AI-assisted by
the aireview plugin. Grafana LEFT JOINs `ai_commits` instead of evaluating
Co-Authored-By / Assisted-by / Made-with trailers at query time. Follows the
same pattern as `ai_reviews` (tool table in aireview, domain table in core).

**Upstream status:** N/A — Konflux aireview addition; upstream Apache DevLake
has no equivalent table.
**Upstream PR:** none — not applicable
**Owner:** @fmuntean

**Rebase notes:** New files plus an append in `domaininfo.go` and
`register.go:All()`. Low conflict risk unless upstream adds adjacent domain
tables in the same slice.

## jira: OAuth 2.0 client-credentials (service account / 2LO)

**Files:**
- `backend/plugins/jira/models/connection.go`
- `backend/plugins/jira/models/oauth.go`
- `backend/plugins/jira/models/connection_test.go`
- `backend/plugins/jira/models/oauth_test.go`
- `backend/plugins/jira/models/migrationscripts/20260907_add_oauth2_credentials.go`
- `backend/plugins/jira/models/migrationscripts/register.go`
- `backend/plugins/jira/api/connection_api.go`
- `backend/plugins/jira/api/connection_api_test.go`
- `backend/plugins/jira/tasks/api_client.go`
- `backend/plugins/jira/token/token_provider.go`
- `backend/plugins/jira/token/round_tripper.go`
- `backend/plugins/jira/token/token_provider_test.go`
- `config-ui/src/plugins/register/jira/connection-fields/auth.tsx`
- `config-ui/src/plugins/components/connection-form/index.tsx`
- `config-ui/src/types/connection.ts`
- `config-ui/src/api/connection/index.ts`

**Reason:** Jira Cloud service accounts authenticate with OAuth 2.0 client
credentials (`grant_type=client_credentials`), not Basic Auth / PAT. Access
tokens last 60 minutes and API calls must go through
`https://api.atlassian.com/ex/jira/{cloudId}/rest/`. Adds `authMethod=OAuth2`
handling inside the Jira plugin (no core MultiAuth change), in-memory token
minting, and a refresh round tripper for long collections.

**Upstream status:** Merged in apache/devlake#9159. This tree matches that version; no outstanding fork delta.
**Upstream PR:** https://github.com/apache/devlake/pull/9159
**Owner:** @fmuntean

**Rebase notes:** Watch upstream changes to `JiraConn` / `_tool_jira_connections`,
`NewJiraApiClient`, `testConnection` validation, and Config UI Cloud auth fields.
`register.go` append is low risk. Shared Config UI connection form/types/API pick
lists gained `cloudId` / `clientId` / `clientSecret`.
## graphql collector: include data-error message and variables in logs

**Files:**
- `backend/helpers/pluginhelper/api/graphql_collector.go`
- `backend/helpers/pluginhelper/api/graphql_collector_test.go`

**Reason:** `errors.Default.Wrap(dataError, "graphql query got error")` uses
cockroachdb `WithDetail`. `Error()` then `%+v`s that wrapper, so pipeline
messages look like `Wraps: (2) graphql query got error Error types: (1)
*hintdetail.withDetail (2) *errors.errorString` and drop the GraphQL
`Message`, locations, and request variables. Store/log a flattened string
instead (`errors.Default.New`) so subtask `message` and worker logs include
the real GitHub error and variables.

**Upstream status:** Pending — should be contributed upstream as a diagnostics
fix.
**Upstream PR:** none yet
**Owner:** @rsoaresd

**Rebase notes:** Touches `fetchAsync` data-error / transport-error handling
and `Execute` worker-error logging. Watch for upstream changes around
`IgnoreQueryErrors` / `isIgnorableGraphqlQueryError`.

## github/github_graphql: graphql client token refresh (cherry-pick)

**Files:**
- `backend/helpers/pluginhelper/api/graphql_async_client.go`
- `backend/plugins/github/tasks/api_client.go`
- `backend/plugins/github/tasks/http_client.go` (new)
- `backend/plugins/github/token/round_tripper.go`
- `backend/plugins/github_graphql/impl/impl.go`
- `backend/plugins/github_graphql/tasks/graphql_client.go` (new)
- `env.example`

**Reason:** `updateRateRemaining` in `graphql_async_client.go` called `panic(err)`
when the GitHub rate-limit API returned an error (e.g. 401 from an expired token).
Because the panic fired inside a background goroutine, Go killed the entire process,
causing ~80 container restarts in 4 days on the prod instance. The upstream fix
replaces the panic with a logged fallback + retry, adds `StaticRoundTripper` for
PAT token rotation, shares the REST client's authenticated HTTP transport with the
GraphQL client (enabling GitHub App token auto-refresh), and introduces a
hierarchical rate-limit fallback (`GRAPHQL_RATE_LIMIT` env → `WithFallbackRateLimit`
→ default 1000).

**Upstream status:** Merged, and present in this tree via the upstream history. No outstanding fork delta.
**Upstream PR:** https://github.com/apache/devlake/pull/8791
**Upstream commit:** `61d66a1339d35709868105cd85153c60b824f290`
**Cherry-picked as:** konflux-ci/devlake#162
**Owner:** @psturc

**Rebase notes:** The cherry-pick arrived with upstream. This entry is kept for history only.

## ci: unit-test Codecov upload on a separate Ubuntu job

**Files:**
- `.github/workflows/test.yml`

**Reason:** Fork-only Codecov upload for owned plugins. `codecov-action` needs
`gpg`, which is not in `mericodev/lake-builder` (Debian 11 / Bullseye). Installing
it via `apt-get` in that container fails because `bullseye-security` InRelease
expired after Debian 11 LTS EOL (2026-08-31). Tests stay in `lake-builder`;
coverage artifacts are uploaded from a follow-on `ubuntu-latest` job with OIDC.

**Upstream status:** N/A — upstream `test.yml` has no Codecov steps.
**Upstream PR:** none — not applicable
**Owner:** @fmuntean

**Rebase notes:** Keep the `test` job aligned with upstream (container, unit
tests). Re-apply the `Custom plugins coverage` / artifact upload steps and the
`upload-coverage` job after upstream workflow changes.

## config-ui: connection fields for owned plugins

**Files:**
- `config-ui/src/plugins/components/connection-form/index.tsx`
- `config-ui/src/api/connection/index.ts`
- `config-ui/src/types/connection.ts`

**Reason:** The shared connection form whitelists which values reach the backend,
so each owned-plugin field must be listed in `IConnectionAPI`, the `test` /
`testOld` payload `Pick<>`s, and both test-payload builders. Fields added so far:
`service` (codecov); `ciTool`, `quayOrganization`, `githubOrganization`,
`githubToken`, `project`, `junitRegex` (testregistry). Missing a builder is
silent — Save Connection still works while Test Connection drops the field and
the backend falls back to its default (COVERPORT-351).

**Upstream status:** N/A — these fields belong to fork-only plugins.
**Upstream PR:** none — not applicable
**Owner:** @lipka28

**Rebase notes:** The lists are additive; on conflict keep both upstream's fields
and the fork's.

## config-ui: aisdlc plugin registration

**Files:**
- `config-ui/src/plugins/register/index.ts`
- `config-ui/src/plugins/register/aisdlc/`
- `config-ui/src/plugins/components/scope-config-form/index.tsx`
- `config-ui/src/plugins/components/connection-form/index.tsx`
- `config-ui/src/api/connection/index.ts`
- `config-ui/src/types/connection.ts`

**Reason:** The AISDLC plugin is registered in the shared plugin list, and its
station fields are edited on the scope-config form. `gitlabConnectionId` is also
added to the connection test whitelist (same silent-drop as COVERPORT-351). The
connection-form files are shared with the section above; keep both sets of fields.

**Upstream status:** N/A — fork-only plugin.
**Upstream PR:** none — not applicable
**Owner:** @fmuntean

**Rebase notes:** Re-apply the `AisdlcConfig` import, the `plugin === 'aisdlc'`
branch, and the `gitlabConnectionId` entries. Plugin code under
`config-ui/src/plugins/register/aisdlc/` is new and does not conflict with upstream.

## license header: AISDLC fixture files

**Files:**
- `.licenserc.yaml`

**Reason:** The RFE result and strategy-task fixtures are copied source artifacts.
An Apache header would change the bytes the parser tests read, and the strategy
fixture must start with YAML front matter. They are ignored the same way other
testdata inputs are.

**Upstream status:** N/A — fork-only fixtures.
**Upstream PR:** none — not applicable
**Owner:** @fmuntean

**Rebase notes:** Keep the `backend/plugins/aisdlc/tasks/testdata/**/*.md` entry in
`paths-ignore`.

## server/api: enable CORS

**Files:**
- `backend/server/api/api.go`
- `backend/core/config/config_viper.go`

**Reason:** External apps and the config UI call the API from another origin.
CORS is enabled with `DELETE` and `Authorization`, credentials, and a 2-hour
preflight cache. `CORS_ALLOW_ORIGIN` defaults to `*`, and comma-separated values
in one env var are split into separate origins.

**Upstream status:** Pending. Upstream still has the middleware commented out.
**Upstream PR:** none
**Owner:** @kpiwko

**Rebase notes:** Re-apply `parseCORSAllowOrigins` and the live `router.Use(cors.New(...))`
block. Leave `/proceed-db-migration` public; upstream `#8930` and `#9068` removed
the auth requirement the fork still has because it is behind those commits.

## projects: default sync window, weekday schedule, empty metrics

**Files:**
- `backend/server/services/project.go`
- `env.example`

**Reason:** New projects sync the last `DEFAULT_SYNC_DAYS` (default 180) and run
on `0 0 * * 0-5` so Saturday is skipped. Project responses always include
`metrics: []` when none are configured, so the config UI does not call `.find()`
on null.

**Upstream status:** Pending.
**Upstream PR:** none
**Owner:** @psturc, @kpiwko

**Rebase notes:** Keep `validateProjectMetrics`. Only change the blueprint cron,
the `TimeAfter` window, and the metrics slice assignment.

## local dev: MySQL zero dates and Grafana anonymous admin

**Files:**
- `docker-compose-dev-mysql.yml`
- `docker-compose-dev-postgresql.yml`
- `.devcontainer/docker-compose.yml`

**Reason:** GitHub PAT connections store a zero `time.Time` for unused OAuth
expiry fields. MySQL 8 strict mode rejects `0000-00-00`, so local MySQL drops
`NO_ZERO_DATE` from `sql_mode`. Local Grafana allows anonymous Admin so
dashboards open without a login.

**Upstream status:** N/A — local-dev only. `docker-compose-dev.yml` was split
upstream into the mysql and postgresql files; do not restore the old file.
**Upstream PR:** none
**Owner:** @kpiwko

**Rebase notes:** Put the `sql-mode` flag on the MySQL services and the three
`GF_AUTH_ANONYMOUS_*` / `GF_AUTH_DISABLE_LOGIN_FORM` variables on the Grafana
services. The GitHub Actions MySQL service in `.github/workflows/test-e2e.yml`
passes the same `sql-mode` as the container `command` (`options` are docker
create flags). e2e coverage composes `docker-compose-dev-mysql.yml` with
`docker-compose-e2e-coverage.yml`, and that overlay builds config-ui from
`config-ui/Containerfile` with the committed Yarn 4 binary. The builder base
is `ubi9/nodejs-24` because the Vite/Rolldown build imports `util.styleText`,
which Node 18 does not provide.

## SQL identifier quoting for Postgres

**Files:**
- `backend/core/dal/dal.go`
- `backend/core/models/migrationscripts/20260422000002_fix_ai_review_domain_columns.go`
- `backend/plugins/aireview/`
- `backend/plugins/agentready/api/`
- `backend/plugins/schema_e2e/migration_schema_test.go`
- `backend/plugins/codecov/models/migrationscripts/20251116000000_add_comparison_table.go`
- `backend/plugins/codecov/tasks/comparison_converter.go`

**Reason:** Owned-plugin queries quote `project_mapping.table` with MySQL
backticks, and the aireview domain migration uses MySQL column types.
Postgres rejects both. `dal.QuoteIdent` quotes the identifier for the active
dialect, and the migration branches on `db.Dialect()`.

**Upstream status:** Pending. The quoting helper is in core because plugins
must not import each other.
**Upstream PR:** none
**Owner:** @fmuntean

**Rebase notes:** Keep `QuoteIdent`. Do not put MySQL backticks back into raw
SQL that runs on both e2e databases. Do not set `gorm:"type:longtext"` on
owned-plugin models; leave the column type unset so MySQL uses longtext and
Postgres uses text. Codecov dedup migrations that use `CREATE TABLE LIKE` and
`RENAME TABLE` stay MySQL-only. Do not set `gorm:"type:double"` on codecov
comparison floats; leave types unset so Postgres gets `double precision`. `plugins/schema_e2e` must list the owned
plugins so the migration schema check covers them. GitHub reaction enrichment
reads raw JSON with `JSON_EXTRACT` on MySQL and `convert_from(...::json)` on
Postgres. The missing-CI lookup casts `pull_request_key` with `CHAR` on MySQL
and `TEXT` on Postgres (`CAST AS CHAR` is `character(1)` there).
