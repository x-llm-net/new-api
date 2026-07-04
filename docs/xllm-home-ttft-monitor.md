# X-LLM Home TTFT Monitor

This document records the X-LLM-owned homepage status monitor plan. It exists to keep this customization visible during upstream new-api upgrades.

## Goal

Replace external status iframes with an X-LLM homepage monitor that shows:

- configured model group availability
- probe time-to-first-token (TTFT)
- recent 60-probe-cycle status, where each block follows `config/xllm-home-monitor.json` `interval_minutes`
- 7-day status trend

The homepage monitor is intentionally narrower than the full new-api channel list. It only monitors entries explicitly configured by X-LLM.

## Non-Goals

- Do not change the meaning of `channels.response_time`.
- Do not change upstream channel auto-disable behavior.
- Do not treat full response duration as TTFT.
- Do not try to monitor every model or every channel on the public homepage.
- Do not expose channel names, keys, base URLs, or provider endpoints in public APIs.

## Data Source Policy

Homepage TTFT is based on active probe requests for configured monitor entries.

Only stream-capable probes are eligible. If a model/channel cannot produce a reliable first response time, it should not be included in the homepage monitor configuration.

For each configured entry:

- send a stream probe request
- record TTFT when the first valid response event is received
- record success/failure and total probe duration for internal diagnostics
- publish TTFT and availability only through aggregate public API responses

Existing real-user `perf_metrics` TTFT can be used as a secondary internal signal, but it should not replace active probe data for homepage availability because it depends on real traffic volume and may be missing for quiet groups.

## Storage Plan

Do not store only the latest value. Latest-only data cannot support recent-cycle and 7-day history.

Use X-LLM-owned tables with an `xllm_` prefix:

### Raw Samples

`xllm_probe_samples`

- `id`
- `probe_version`
- `task_id`
- `selected_channel_id`
- `group_name`
- `model_name`
- `status`
- `error_code`
- `ttft_ms` nullable
- `total_ms`
- `tested_at`
- `created_at`

Raw samples are the source of truth for debugging and rebuilding aggregates.

### Rollups

`xllm_probe_rollups`

- `bucket_seconds` (`60` for minute buckets, `3600` for hourly buckets)
- `probe_version`
- `bucket_ts`
- `selected_channel_id`
- `group_name`
- `model_name`
- `sample_count`
- `success_count`
- `failure_count`
- `ttft_sum_ms`
- `ttft_count`
- `total_sum_ms`
- `total_count`
- `min_total_ms`
- `max_total_ms`
- `last_status`
- `last_tested_at`

Use a unique index on:

```text
probe_version, bucket_seconds, bucket_ts, group_name, model_name
```

Public homepage queries should read rollups, not raw samples.

This feature was rebuilt around `channel_id` before its first production
release. Experimental tables from earlier `channel_id` identities are intentionally dropped
during migration. Do not add compatibility code for those early local samples;
they are not production data and would make future upgrades harder.

The homepage "recent" row always renders 60 blocks. The block width is derived
from `config/xllm-home-monitor.json` `interval_minutes`, so a 5-minute local
interval displays 5 hours and a 10-minute production interval displays 10
hours. Do not hard-code this row as 60 natural minutes in the UI.

### Optional Latest Cache

`xllm_probe_latest` may be added later as a cache for current state. It must remain a derived cache, not the only source of historical data.

## Retention

Suggested starting retention:

- raw samples: 7-14 days
- minute rollups: 7 days
- hourly rollups: 30-90 days

Use indexed cleanup by `tested_at` or `bucket_ts`. For SQLite, delete in small batches to avoid long write locks.

## Configuration

Keep homepage monitor configuration separate from official new-api channel settings.

Recommended shape:

```text
group
model
display_name
enabled
sort_order
```

`group` and `model` are the probe identity. The X-LLM probe selects a normal
new-api route for that group/model, so it observes the same routing pool users
would hit. `display_name` is display-only.

Only entries listed here appear on the homepage. If TTFT cannot be captured reliably, remove the entry from the configuration instead of falling back to misleading full-response timing.

Current implementation reads:

```text
config/xllm-home-monitor.json
```

The path can be overridden with:

```text
XLLM_HOME_MONITOR_CONFIG
```

If the file is missing or invalid, the service returns an empty monitor list. Production deployments should keep the JSON file under deployment config management and set real production channel IDs.

## Current Runtime Behavior

The homepage summary endpoint is:

```text
GET /api/xllm/home-monitor/summary
```

The upstream new-api channel-test task is not used as the homepage data source.
X-LLM registers a separate scheduled system task:

```text
xllm_home_probe
```

For each configured group/model entry, the task selects a route through normal
new-api channel selection, sends a stream probe, and records only aggregate
homepage monitor data.

This X-LLM recording hook:

- records TTFT into `xllm_probe_samples`
- updates 60-second and 3600-second rollups in `xllm_probe_rollups`
- does not update `channels.response_time` or `channels.test_time`
- does not participate in upstream automatic channel disable/enable decisions
- does not consume user quota or write user usage logs
- does consume real upstream API quota for the selected probe request

Stream probe failures are recorded as failed samples. Non-stream channel tests are not recorded as TTFT samples because full response duration is not a first-token metric.

## Store Boundary

Keep all persistence and aggregation behind an X-LLM-owned store/service boundary.

Recommended backend shape:

```text
service/xllm_probe_store.go
service/xllm_probe_rollup.go
controller/xllm_probe_summary.go
model/xllm_probe_sample.go
model/xllm_probe_rollup.go
```

The rest of the codebase should call a small wrapper such as:

```text
xllmprobe.RecordProbeSample(...)
xllmprobe.GetHomeSummary(...)
```

This keeps upstream merge conflicts localized. During upgrades, if upstream rewrites channel testing or performance metrics, the X-LLM store remains the stable owner of homepage monitor data.

Do not store homepage monitor state in frontend-only local storage. The source of truth must be server-side so Docker restarts, browser changes, and multi-admin usage do not erase or fork monitor data.

## Upgrade Safety

This feature should be implemented as an X-LLM-owned module, preferably with names and paths that include `xllm`.

Preferred ownership boundaries:

- `model/xllm_probe_*.go`
- `service/xllm_probe_*.go`
- `controller/xllm_probe_*.go`
- `web/default/src/features/xllm-home-monitor/`
- `docs/xllm-home-ttft-monitor.md`

Expected upstream merge touch points:

- one system-task registration line
- one route registration line for the public aggregate endpoint

Avoid scattering homepage monitor logic across generic upstream modules. Keep all X-LLM-specific logic behind small wrapper functions so future upstream changes are easier to rebase.

## Public UI Rules

- Show TTFT only when `ttft_ms` is available from a reliable stream probe.
- Label missing TTFT as unavailable or omit the entry.
- Do not show full response duration as TTFT.
- If total duration is useful, keep it in tooltip or admin diagnostics only.
- Make clear that homepage status covers configured monitor groups, not every available model.
