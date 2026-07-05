# X-LLM Group Stability Dashboard

The homepage stability dashboard is an X-LLM customization. It reuses the existing `channel_test` scheduled task and does not send additional upstream probe requests.

Runtime config path:

```json
data/xllm-group-stability.json
```

Override path with:

```text
XLLM_GROUP_STABILITY_CONFIG=/path/to/xllm-group-stability.json
```

Example:

```json
{
  "enabled": true,
  "groups": [
    {
      "group": "codex-pro",
      "display_name": "Codex Pro",
      "enabled": true,
      "sort_order": 10
    }
  ]
}
```

Only `group` decides which channel-test samples are included. `display_name` is presentation-only.

Data model:

- Every complete `channel_test` channel result is recorded in `xllm_group_stability_samples`.
- Passive recovery runs are not recorded because they only test auto-disabled channels and would skew homepage stability.
- A group is available for one task run if at least one channel in that group succeeds.
- Homepage recent trend shows the latest 60 `channel_test` runs.
- Homepage 7-day trend shows 168 hourly buckets.
- Samples older than 10 days are cleaned after channel-test runs.
- The public summary API does not backfill history from logs. Historical samples, if needed, must be imported by an explicit one-off script so homepage traffic never scans the log database and optimistic backfill is clearly intentional.

Upgrade note:

- Keep this document and the `xllm_*` backend/frontend modules when rebasing upstream.
- Keep the runtime config file outside version-controlled assets, usually under `data/`.
