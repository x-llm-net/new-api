# X-LLM Upgrade Checklist

This fork keeps a small set of product customizations on top of upstream new-api. When syncing a new upstream release, verify these items before release.

## Branching Rule

- Maintain `xllm/main` as the long-lived X-LLM integration branch.
- Sync upstream by merging the upstream release into `xllm/main`; do not create a fresh branch from upstream and cherry-pick selected X-LLM commits.
- Release branches and `xllm-*` release tags must be descendants of `xllm/main` and the previous X-LLM release tag.
- Run `node scripts/xllm-release-guard.mjs` before creating a release tag.

## Upstream Release Check

- Before every X-LLM release, update upstream metadata:
  - `git fetch upstream --tags --prune`
  - `git tag --list 'v*' --sort=-v:refname | head`
  - `git rev-list --left-right --count xllm/main...upstream/main`
  - `git log --oneline xllm/main..upstream/main`
- If upstream has a newer release tag, make an explicit release decision before tagging:
  - **Merge now**: create an integration branch from `xllm/main`, merge the upstream release tag or `upstream/main`, resolve conflicts while preserving X-LLM customizations, run the full verification gate, then release from that merged history.
  - **Defer**: release the current X-LLM branch only, record the newer upstream tag and the reason it was deferred, and schedule a separate upstream merge release.
- Do not mix an unplanned upstream upgrade into a release window. If the upstream diff includes auth, billing, migration, channel routing, frontend build, or session changes, treat it as a separate upgrade unless the user explicitly approves the extra risk.

## Upstream Merge Mechanism

- Use this command shape for upstream upgrades:
  - `git switch xllm/main`
  - `git pull --ff-only origin xllm/main`
  - `git switch -c xllm/merge-upstream-<version>`
  - `git merge --no-ff <upstream-tag-or-upstream/main>`
- Resolve conflicts in favor of upstream fixes unless they replace X-LLM product customizations listed in this file.
- After resolving conflicts, run the full release gate and `node scripts/xllm-release-guard.mjs`.
- Merge the integration branch back into `xllm/main` only after verification passes.
- The guard is not the update mechanism; it is only the gate that prevents a bad merge from being released.

## Frontend Defaults

- Default frontend theme stays `default`.
- Default documentation link points to the X-LLM Feishu documentation.
- The X-LLM homepage sections are still present on the default home page.
- X-LLM `logo.png`, `favicon.ico`, and favicon references are preserved in both `web/default` and `web/classic`.

## System Announcements

- System announcements still support the notification center popover.
- New unread system notices or announcements auto-open a dialog after notice/status data loads.
- Announcement read keys include both the announcement id and a content fingerprint, so editing an existing announcement can show it again.
- Auto-open must not mark the announcement as read before the dialog is shown; mark it read when the user closes the dialog or opens the notification center.
- The dialog supports `Close Today` and should not repeatedly pop up again that day.
- Closing the auto-open dialog does not remove the right-header notification entry.

## Release

- X-LLM Docker image publishing uses the GHCR workflow and does not publish to the upstream Docker Hub image.
- X-LLM release tags use the `xllm-*` prefix; upstream Docker Hub and GitHub Release workflows must exclude that prefix.
- `node scripts/xllm-release-guard.mjs` must pass locally before pushing the release tag.
- Adding the release guard to the GitHub workflow requires a GitHub credential with `workflow` scope and should be done as a separate maintenance change.
- Run at least `bun run typecheck` and `bun run build` in `web/default` after frontend changes.
