# X-LLM Upstream Sync

## Source Of Truth

- `xllm/main` is the only long-lived X-LLM integration branch.
- Every X-LLM release must descend from `xllm/main` and the previous X-LLM release tag.
- Feature behavior belongs in code, tests, and feature documentation, not in this file.

## Merge Rule

Upgrade from the current X-LLM history:

```text
xllm/main
  -> xllm/merge-upstream-<version>
  -> merge the reviewed upstream release tag
  -> verify
  -> merge back into xllm/main
```

Never rebuild the product branch from an upstream tag and manually copy or cherry-pick selected X-LLM changes.

## Release Gate

Before merging or tagging, run checks appropriate to the changed areas and always run:

```text
git diff --check
node scripts/xllm-release-guard.mjs
```

Frontend changes also require typecheck and build. A release tag is created only from the verified merged history; production deployment and rollback follow the `xllm-new-api-release` Skill.
