# X-LLM targeted diagnostic capture

This facility temporarily records a bounded copy of matching HTTP relay attempts. It is intended for investigations where normal error logs cannot explain model output, routing, or stream termination behavior.

It is inactive when `/data/diagnostic-captures/config.json` is missing, disabled, invalid, or expired. The process checks the file every two seconds, so capture targets can be changed without restarting the service.

## Filters

Each rule supports these exact-match filters:

- `requested_models`: model name sent by the customer before channel mapping.
- `upstream_models`: model name after channel mapping.
- `channel_ids`: selected channel IDs.
- `user_ids`: customer user IDs.
- `token_ids`: API token IDs, not token strings.
- `groups`: selected groups.
- `protocols`: final upstream relay format, such as `openai`, `gemini`, or `claude`.

Values within one field use OR semantics. Different fields use AND semantics. An empty field is a wildcard. A rule with every filter empty is rejected to prevent accidental global prompt capture. When multiple rules match, the first matching rule wins and only one file is written for that upstream attempt.

Retries are captured separately because each attempt can select a different channel. WebSocket relays are not captured in the first version.

## Operations

Run the helper from the repository root on a Windows administrator workstation with the `x-llm` SSH alias configured.

Capture a requested model for two hours:

```powershell
.\scripts\xllm-diagnostic-capture.ps1 enable `
  -Name gemini-31pro `
  -Model gemini-3.1-pro-preview `
  -Hours 2
```

Capture only attempts for the requested model that route to channel 136:

```powershell
.\scripts\xllm-diagnostic-capture.ps1 enable `
  -Name gemini-31pro-c136 `
  -Model gemini-3.1-pro-preview `
  -Channel 136 `
  -Hours 2
```

Capture a mapped upstream model for one customer token at 10% deterministic sampling:

```powershell
.\scripts\xllm-diagnostic-capture.ps1 enable `
  -Name mapped-model-token `
  -UpstreamModel gemini-3.1-pro-high `
  -TokenId 7300 `
  -SampleRate 0.1 `
  -Hours 1
```

Inspect, disable, or export:

```powershell
.\scripts\xllm-diagnostic-capture.ps1 status
.\scripts\xllm-diagnostic-capture.ps1 disable
.\scripts\xllm-diagnostic-capture.ps1 export
```

`enable` currently replaces the active config with one rule. The JSON format supports multiple rules when an investigation needs several targets at once.

## Record format and limits

Files are stored under:

```text
/data/diagnostic-captures/YYYYMMDD/HH/<rule>/<request-id>-r<retry>-c<channel>-<time>.capture
```

Each file contains metadata, the original client request body, and the raw upstream response body. Request and response headers, gateway credentials, channel credentials, and API token strings are not serialized. The client body is intentionally retained as-is, so any secret a customer places inside the body will also be retained; access to the directory must remain administrator-only.

Defaults:

- request body: first 512 KiB
- upstream response body: first 1 MiB
- pending queue: 1,000 records and 256 MiB
- stored files: 10,000
- stored data: 2 GiB
- retention: 24 hours
- expiration: required for every enabled config

Response capture is transparent: after the capture limit is reached, the complete response continues to the normal relay handler. If the queue is full, the diagnostic record is dropped instead of delaying the customer request. Files and directories use `0600` and `0700` permissions on Linux.

The metadata distinguishes complete responses, truncated captures, early closes, read failures, and transport failures. It also records requested and mapped models, channel, retry index, user, token ID, group, protocol, stream flag, status, and upstream request ID.
