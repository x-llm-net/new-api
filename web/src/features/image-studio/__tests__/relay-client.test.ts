/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { Blob as NativeBlob, File as NativeFile } from 'node:buffer'

import { afterAll, afterEach, describe, test, vi } from 'vitest'

import {
  fetchStudioModels,
  relayRequest,
  requestImage,
} from '../lib/relay-client'

// Use Node's Fetch-compatible binary primitives at the network boundary.
vi.stubGlobal('Blob', NativeBlob)
vi.stubGlobal('File', NativeFile)
vi.stubGlobal(
  'FormData',
  (await new Response(new URLSearchParams()).formData()).constructor
)
afterAll(() => vi.unstubAllGlobals())

const originalFetch = globalThis.fetch
afterEach(() => {
  globalThis.fetch = originalFetch
})

describe('image studio relay authentication', () => {
  test('model listing uses the selected model key without dashboard cookies', async () => {
    globalThis.fetch = (async (url, init) => {
      assert.equal(url, '/v1/models')
      assert.equal(
        new Headers(init?.headers).get('Authorization'),
        'Bearer sk-user-selected'
      )
      assert.equal(init?.credentials, 'omit')
      assert.equal(init?.redirect, 'error')
      return Response.json({ data: [{ id: 'custom-multimodal-model' }] })
    }) as typeof fetch
    assert.deepEqual(await fetchStudioModels('user-selected'), [
      { id: 'custom-multimodal-model' },
    ])
  })

  test('invalid model key reports its error without retrying or calling login refresh', async () => {
    const paths: unknown[] = []
    globalThis.fetch = (async (url) => {
      paths.push(url)
      return Response.json(
        { error: { message: 'Token expired' } },
        { status: 401 }
      )
    }) as typeof fetch
    await assert.rejects(
      relayRequest('/v1/models', 'sk-expired'),
      /Token expired/
    )
    assert.deepEqual(paths, ['/v1/models'])
  })

  test('editing sends the original and guide as multipart image files to edits', async () => {
    const original = new Blob(['original-bytes'], { type: 'image/png' })
    const guide = new Blob(['guide-bytes'], { type: 'image/png' })
    globalThis.fetch = (async (url, init) => {
      assert.equal(url, '/v1/images/edits')
      assert.equal(new Headers(init?.headers).get('Content-Type'), null)
      assert.ok(init?.body instanceof FormData)
      assert.equal(init.body.get('model'), 'custom-image-model')
      assert.equal(init.body.get('prompt'), 'Change only the numbered area')
      const files = init.body.getAll('image[]') as File[]
      assert.deepEqual(await Promise.all(files.map((file) => file.text())), [
        'original-bytes',
        'guide-bytes',
      ])
      return Response.json({ data: [{ b64_json: btoa('generated-bytes') }] })
    }) as typeof fetch
    const result = await requestImage({
      key: 'sk-model-key',
      model: 'custom-image-model',
      prompt: 'Change only the numbered area',
      size: '1024x1024',
      images: [original, guide],
      signal: new AbortController().signal,
    })
    assert.equal(await result.text(), 'generated-bytes')
  })

  test('a returned image URL never receives the model key or cookies', async () => {
    globalThis.fetch = (async (url, init) => {
      if (String(url) === '/v1/images/generations') {
        return Response.json({
          data: [{ url: 'https://images.example.test/result.png' }],
        })
      }
      assert.equal(String(url), 'https://images.example.test/result.png')
      assert.equal(new Headers(init?.headers).get('Authorization'), null)
      assert.equal(init?.credentials, 'omit')
      return new Response(new Blob(['image'], { type: 'image/png' }))
    }) as typeof fetch
    assert.equal(
      (
        await requestImage({
          key: 'sk-private',
          model: 'image-model',
          prompt: 'A garden',
          size: '1024x1024',
          signal: new AbortController().signal,
        })
      ).type,
      'image/png'
    )
  })
})
