# Release Runbook

## Topology

- Source fork repo: `D:\code\new-api`
- Deploy/runtime repo: `D:\code\new-api-ops`
- Fork remote: `origin = https://github.com/leivii/new-api.git`
- Official remote: `upstream = https://github.com/QuantumNous/new-api.git`
- Production host alias: `rockapi`
- Production runtime dir: `/opt/llm-hub`
- Production image repo: `ghcr.io/leivii/new-api`
- Release tag suffix: `rockapi` (for example `v1.0.0-rc.10-rockapi.1`)
- Production deploy entrypoint: `D:\code\new-api-ops\scripts\deploy-rockapi.ps1`

## Release Sequence

1. Check source repo state:
   - `git -C D:\code\new-api status --short --branch`
2. Check ops repo state:
   - `git -C D:\code\new-api-ops status --short --branch`
3. Push fork `main`:
   - `git -C D:\code\new-api push origin main`
4. Create release tag:
   - `git -C D:\code\new-api tag -a v1.0.0-rc.10-rockapi.1 -m "Release v1.0.0-rc.10-rockapi.1"`
5. Push release tag:
   - `git -C D:\code\new-api push origin v1.0.0-rc.10-rockapi.1`
6. Wait for GHCR image:
   - `docker manifest inspect ghcr.io/leivii/new-api:v1.0.0-rc.10-rockapi.1`
7. Deploy from ops repo:
   - `powershell -NoProfile -ExecutionPolicy Bypass -File "D:\code\new-api-ops\scripts\deploy-rockapi.ps1" -ImageTag "v1.0.0-rc.10-rockapi.1"`

## Verification

- `ssh rockapi "docker ps --format '{{.Names}}\t{{.Image}}\t{{.Status}}'"`
- `ssh rockapi "docker inspect llm-hub-new-api-1 --format '{{.Config.Image}}'"`
- `ssh rockapi "docker logs --tail 100 llm-hub-new-api-1"`
- `ssh rockapi "docker compose --env-file /opt/llm-hub/.env -f /opt/llm-hub/compose.prod.yml ps"`

## Known Failure Modes

### 1. `git push origin` returns `403`

Cause:

- Git Credential Manager is authenticated with the wrong GitHub account.

Fix:

- Inspect accounts:
  - `git credential-manager github list`
- Re-login the correct account:
  - `git credential-manager github login --device --force`

### 2. `docker manifest inspect ghcr.io/leivii/new-api:<tag>` says `manifest unknown`

Possible causes:

- GitHub Actions has not finished publishing the image yet.
- The GHCR package is private and the local machine is not logged in.

Fix:

- Wait and retry.
- If private, log in first:
  - `git credential fill` can supply the current GitHub token
  - `docker login ghcr.io`

### 3. Deploy script reports failure after containers are recreated

Cause:

- The final post-deploy probe can return non-zero even when `docker compose up -d` succeeded and the container is healthy.

What to do:

- Do not immediately roll back.
- Check actual container health and runtime logs first.
- If `llm-hub-new-api-1` is running the expected image and is `healthy`, treat rollout as successful and then fix the probe separately.

## Rollback

- Redeploy the last good tag:
  - `powershell -NoProfile -ExecutionPolicy Bypass -File "D:\code\new-api-ops\scripts\deploy-rockapi.ps1" -ImageTag "<previous-tag>"`
- Check `/opt/llm-hub/backups` for timestamped compose/Caddy backups.
