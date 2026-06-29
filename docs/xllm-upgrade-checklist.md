# X-LLM Upgrade Checklist

This fork keeps a small set of product customizations on top of upstream new-api. When syncing a new upstream release, verify these items before release.

## Frontend Defaults

- Default frontend theme stays `default`.
- Default documentation link points to the X-LLM Feishu documentation.
- The X-LLM homepage sections are still present on the default home page.

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
- Run at least `bun run typecheck` and `bun run build` in `web/default` after frontend changes.
