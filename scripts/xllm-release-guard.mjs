#!/usr/bin/env node
import { execFileSync } from 'node:child_process'
import { existsSync, readFileSync } from 'node:fs'

const checks = []
const failures = []

function git(args, options = {}) {
  try {
    return execFileSync('git', args, {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe'],
      ...options,
    }).trim()
  } catch (error) {
    if (options.allowFailure) {
      return null
    }
    throw error
  }
}

function pass(message) {
  checks.push({ ok: true, message })
}

function fail(message) {
  checks.push({ ok: false, message })
  failures.push(message)
}

function requireFile(path) {
  if (existsSync(path)) {
    pass(`${path} exists`)
    return readFileSync(path, 'utf8')
  }
  fail(`${path} is missing`)
  return ''
}

function requireContains(path, pattern, description) {
  const content = requireFile(path)
  if (!content) return
  if (pattern.test(content)) {
    pass(description)
  } else {
    fail(`${description} not found in ${path}`)
  }
}

function requireAncestor(ancestor, descendant, description) {
  const ok = git(['merge-base', '--is-ancestor', ancestor, descendant], {
    allowFailure: true,
  })
  if (ok === null) {
    fail(`${description}: ${ancestor} is not an ancestor of ${descendant}`)
  } else {
    pass(description)
  }
}

function resolveRef(ref) {
  const resolved = git(['rev-parse', '--verify', ref], { allowFailure: true })
  return resolved || ''
}

function currentRef() {
  const githubRef = process.env.GITHUB_REF
  if (githubRef?.startsWith('refs/tags/')) {
    return githubRef.slice('refs/tags/'.length)
  }
  return 'HEAD'
}

function latestPreviousXLLMTag(ref) {
  const tags = git(['tag', '--merged', ref, '--sort=-creatordate', '--list', 'xllm-*'])
    .split(/\r?\n/)
    .map((tag) => tag.trim())
    .filter(Boolean)
  const current = process.env.TAG || process.env.GITHUB_REF_NAME
  return tags.find((tag) => tag !== current) || ''
}

function requireBlob(path, expected, description) {
  if (!existsSync(path)) {
    fail(`${description}: ${path} is missing`)
    return
  }
  const actual = git(['hash-object', path], { allowFailure: true })
  if (!actual) {
    fail(`${description}: cannot hash ${path}`)
    return
  }
  if (actual === expected) {
    pass(description)
  } else {
    fail(`${description}: ${path} blob ${actual} != expected ${expected}`)
  }
}

function main() {
  const ref = currentRef()
  const tag = process.env.TAG || process.env.GITHUB_REF_NAME || ''
  const localIntegrationRef = resolveRef('xllm/main')
  const remoteIntegrationRef = resolveRef('origin/xllm/main')
  const integrationRef = localIntegrationRef
    ? 'xllm/main'
    : remoteIntegrationRef
      ? 'origin/xllm/main'
      : ''

  if (integrationRef) {
    const resolved = localIntegrationRef || remoteIntegrationRef
    pass(`long-lived X-LLM integration branch: ${integrationRef} -> ${resolved.slice(0, 12)}`)
  } else {
    fail('long-lived X-LLM integration branch does not exist')
  }

  if (tag && !tag.startsWith('xllm-')) {
    fail(`X-LLM release tag must start with xllm-, got ${tag}`)
  } else if (tag) {
    pass(`release tag uses xllm-* prefix: ${tag}`)
  }

  if (integrationRef) {
    requireAncestor(integrationRef, ref, `release includes long-lived ${integrationRef} history`)
  }

  const previousTag = process.env.XLLM_PREVIOUS_TAG || latestPreviousXLLMTag(ref)
  if (previousTag) {
    requireAncestor(previousTag, ref, `release includes previous X-LLM release ${previousTag}`)
  } else {
    fail('cannot determine previous X-LLM release tag; set XLLM_PREVIOUS_TAG')
  }

  requireBlob(
    'web/public/logo.png',
    'a47c4beabc03de009a7d8ea1d147dd8d5baa2c7a',
    'frontend keeps X-LLM logo asset'
  )
  requireBlob(
    'web/public/favicon.ico',
    '67a6e6db843c0b3bf7693379da1ddf651ecd9525',
    'frontend keeps X-LLM favicon asset'
  )

  requireContains(
    'web/index.html',
    /<link\s+rel=["']icon["'][^>]*href=["']\/favicon\.ico["']/,
    'frontend uses favicon.ico as favicon'
  )
  requireContains(
    '.github/workflows/docker-build.yml',
    /!\s*xllm-\*/,
    'upstream Docker workflow excludes xllm-* tags'
  )
  requireContains(
    '.github/workflows/release.yml',
    /!\s*xllm-\*/,
    'upstream GitHub Release workflow excludes xllm-* tags'
  )
  requireContains(
    '.github/workflows/xllm-docker-image.yml',
    /ghcr\.io\/\$REPOSITORY:\$TAG/,
    'X-LLM workflow publishes GHCR immutable tag'
  )
  requireContains(
    'router/api-router.go',
    /\/xllm\/group-stability\/summary/,
    'X-LLM group stability API route is present'
  )
  requireContains(
    'web/src/features/home/index.tsx',
    /<GroupStability\s*\/>/,
    'X-LLM homepage group stability section is mounted'
  )
  requireContains(
    'web/src/features/home/components/sections/home-footer.tsx',
    /support@x-llm\.net/,
    'X-LLM homepage footer branding is present'
  )
  requireFile('web/src/features/xllm-group-stability/api.ts')
  requireFile('web/src/features/xllm-group-stability/types.ts')

  for (const check of checks) {
    const marker = check.ok ? 'PASS' : 'FAIL'
    console.log(`[${marker}] ${check.message}`)
  }

  if (failures.length > 0) {
    console.error(`\nX-LLM release guard failed with ${failures.length} issue(s).`)
    process.exit(1)
  }

  console.log('\nX-LLM release guard passed.')
}

main()
