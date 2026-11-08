# X-LLM channel protection

The external TTFT collector reuses its existing bounded request-log reads. Every five minutes it can automatically disable channels belonging to the configured `channel_group`, based on requests from configurable `sample_groups`. Channel status changes apply globally. It does not change channel groups, weights, credentials, billing, or models.

Policy lives in `/data/xllm-channel-protection.json` (override: `XLLM_CHANNEL_PROTECTION_CONFIG`). The default deployment policy uses a 300-second window, at least 50 measured requests per channel, and strictly more than 50% of all observed channel requests exceeding 15,000 ms. Streaming requests use first-response latency; non-streaming requests use total duration. Missing latency never becomes a timeout. More than 5% unknown latency or incomplete time coverage blocks automatic action. The collector deduplicates channel/request pairs and excludes requests started before the latest channel status change.

`fallbacks` contains named channel-ID sets with `min_enabled`; each action rechecks current enabled members. Membership changes take effect on the next request. Fallback protection applies to this monitor's actions, not independent manual changes or New API error-based bans. At least one configured fallback set is required. Channels outside any fallback set can be disabled normally; configured sets do not imply every model has a fallback.

## API and credential boundary

- `GET /api/xllm/channel-protection` returns policy and non-secret channel status metadata.
- `POST /api/xllm/channel-protection` accepts a policy version and bounded evidence batch. It can only change a currently enabled, in-scope channel to auto-disabled (status 3), subject to server-enforced sample/rate/fallback/freshness checks. It cannot enable channels or change channel data.
- A dedicated 256-bit random bearer credential is stored in a separate private file at `<config path>.key`, mode 600. The collector has a separate mode-600 copy outside the repository. Deployments use verified HTTPS; the collector refuses redirects. This credential grants no dashboard access. Missing configuration/key, a wrong credential, a changed policy, stale measurements, or an intervening channel status change fail closed.
- Configuration and credential are re-read each call. Setting `enabled=false` pauses actions without restarting. Removing/rotating the key revokes access immediately for subsequent calls. Do not store secrets in JSON examples, commits, task outputs, or request logs.
- Operations are audited as the `xllm-channel-protection` machine principal using `monitor_key` authentication, without credentials. Ordinary dashboard status endpoints continue to accept only enabled/manual-disabled states.

Authentication review references: OWASP ASVS 5.0.0, requirements 8.1.1, 8.2.1–8.2.3, 8.3.1–8.3.2; Authentication, Session Management, and Authorization Cheat Sheets. Focused tests cover rejected/missing/revoked credentials, scope restrictions, policy version changes, fallback retention, minimum samples, the strict majority boundary, manual-disable preservation, and SQLite/MySQL/PostgreSQL persistence and ability updates. This is a scoped review, not an ASVS certification.

## Recovery and rollback

Status 3 participates in New API's existing automatic channel tests. A successful test may re-enable it when automatic recovery is enabled. Test success does not necessarily mean real-request latency is below 15 seconds; the existing global test latency threshold is not changed. Manual-disabled status 2 is never changed by this collector. After recovery, only new requests contribute to a new disable decision.

Disable protection in the policy first when rolling back. Restore the previous application image and collector files/config; retain current monitor state to avoid duplicate notifications. Existing auto-disabled channels remain recoverable by native tests. Do not blindly re-enable channels during rollback.

Integration hooks to retain during upstream upgrades: the two X-LLM routes in `router/api-router.go`, `controller/xllm_channel_protection.go`, and the compare-before-disable wrapper in `model/channel.go`. No schema migration or frontend changes are introduced.
