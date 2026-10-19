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

import i18next from 'i18next'
import { afterAll, afterEach, beforeEach, test, vi } from 'vitest'

import { runImageTurn } from '../lib/agent'
import type { StudioImage } from '../types'

// Use Node's Fetch-compatible binary primitives at the network boundary.
vi.stubGlobal('Blob', NativeBlob)
vi.stubGlobal('File', NativeFile)
vi.stubGlobal(
  'FormData',
  (await new Response(new URLSearchParams()).formData()).constructor
)
afterAll(() => vi.unstubAllGlobals())

const originalFetch = globalThis.fetch
const originals = new Map(
  ['document', 'FileReader', 'createImageBitmap'].map((key) => [
    key,
    Object.getOwnPropertyDescriptor(globalThis, key),
  ])
)
await i18next.init({ lng: 'en', resources: { en: { translation: {} } } })

// Only browser image decoding/canvas and the network are simulated. The Agent,
// argument parsing, multipart request and image-result handling run unchanged.
beforeEach(() => {
  Object.defineProperty(globalThis, 'FileReader', {
    configurable: true,
    value: class {
      result: string | null = null
      onload?: () => void
      onerror?: () => void
      readAsDataURL(blob: Blob) {
        void blob
          .arrayBuffer()
          .then((bytes) => {
            this.result = `data:${blob.type};base64,${Buffer.from(
              bytes
            ).toString('base64')}`
            this.onload?.()
          })
          .catch(() => this.onerror?.())
      }
    },
  })
  Object.defineProperty(globalThis, 'createImageBitmap', {
    configurable: true,
    value: async (blob: Blob) => ({
      ...JSON.parse(await blob.text()),
      close() {},
    }),
  })
  Object.defineProperty(globalThis, 'document', {
    configurable: true,
    value: {
      createElement(tag: string) {
        assert.equal(tag, 'canvas')
        return {
          width: 0,
          height: 0,
          getContext: () => ({ drawImage() {} }),
          toBlob(callback: (blob: Blob) => void) {
            callback(
              new Blob(
                [JSON.stringify({ width: this.width, height: this.height })],
                { type: 'image/png' }
              )
            )
          },
        }
      },
    },
  })
})
afterEach(() => {
  globalThis.fetch = originalFetch
  for (const [key, descriptor] of originals) {
    if (descriptor) Object.defineProperty(globalThis, key, descriptor)
    else Reflect.deleteProperty(globalThis, key)
  }
})

async function runEdit(options: {
  width: number
  height: number
  size?: string
  output?: { width: number; height: number }
  rejected?: boolean
}) {
  const source: StudioImage = {
    id: 'original',
    name: 'A garden',
    createdAt: 1,
    width: options.width,
    height: options.height,
    blob: new Blob(
      [JSON.stringify({ width: options.width, height: options.height })],
      { type: 'image/png' }
    ),
  }
  const forms: FormData[] = []
  const warnings: string[] = []
  const images: StudioImage[] = []
  const chatBodies: Array<{ messages: Array<{ content: unknown }> }> = []
  const output =
    options.output ??
    (options.size
      ? {
          width: Number(options.size.split('x')[0]),
          height: Number(options.size.split('x')[1]),
        }
      : { width: options.width, height: options.height })
  globalThis.fetch = (async (url, init) => {
    if (url === '/v1/chat/completions') {
      chatBodies.push(JSON.parse(String(init?.body)))
      return Response.json({
        choices: [
          {
            message:
              chatBodies.length === 1
                ? {
                    content: null,
                    tool_calls: [
                      {
                        id: 'edit-call',
                        type: 'function',
                        function: {
                          name: 'edit_image',
                          arguments: JSON.stringify({
                            prompt: 'Add a flower beside the tree',
                            source_image_id: 'original',
                            ...(options.size ? { size: options.size } : {}),
                          }),
                        },
                      },
                    ],
                  }
                : { content: 'Updated the flower.' },
          },
        ],
      })
    }
    assert.equal(url, '/v1/images/edits')
    assert.ok(init?.body instanceof FormData)
    forms.push(init.body)
    if (options.rejected) {
      return Response.json(
        { error: { message: 'Unsupported image size' } },
        { status: 400 }
      )
    }
    return Response.json({ data: [{ b64_json: btoa(JSON.stringify(output)) }] })
  }) as typeof fetch
  let failure: unknown
  try {
    await runImageTurn({
      chatKey: 'test',
      imageKey: 'test',
      chatModel: 'chat',
      imageModel: 'image',
      currentImageId: source.id,
      images: [source],
      messages: [
        {
          id: 'user',
          role: 'user',
          text: options.size
            ? `Change the canvas to ${options.size}`
            : 'Add a flower beside the tree',
          imageIds: [],
          marks: [],
        },
      ],
      signal: new AbortController().signal,
      onStage() {},
      onText() {},
      onImage: (image) => images.push(image),
      onWarning: (warning) => warnings.push(warning),
    })
  } catch (error) {
    failure = error
  }
  return { source, forms, warnings, images, chatBodies, failure }
}

for (const [width, height] of [
  [1536, 1024],
  [1024, 1536],
  [1600, 900],
]) {
  test(`a detail edit of ${width}x${
    height
  } inherits the original dimensions and preserves composition`, async () => {
    const result = await runEdit({ width, height })
    assert.equal(result.failure, undefined)
    assert.equal(result.forms[0].get('size'), `${width}x${height}`)
    assert.equal(
      await (result.forms[0].get('image') as File).text(),
      await result.source.blob.text()
    )
    assert.match(String(result.forms[0].get('prompt')), /Preserve.*composition/)
    assert.ok(
      JSON.stringify(result.chatBodies[0].messages).includes(
        `${width}x${height}`
      )
    )
    assert.deepEqual(result.warnings, [])
  })
}

test('an explicitly requested canvas size overrides the source dimensions', async () => {
  const result = await runEdit({ width: 1600, height: 900, size: '1024x1536' })
  assert.equal(result.failure, undefined)
  assert.equal(result.forms[0].get('size'), '1024x1536')
  assert.deepEqual(result.warnings, [])
})

test('an unexpected square result remains available with a warning and no automatic retry', async () => {
  const result = await runEdit({
    width: 1600,
    height: 900,
    output: { width: 1024, height: 1024 },
  })
  assert.equal(result.failure, undefined)
  assert.equal(result.images.length, 1)
  assert.equal(result.images[0].parentId, 'original')
  assert.equal(result.warnings.length, 1)
  assert.match(result.warnings[0], /1600.*900/)
  assert.match(result.warnings[0], /1024.*1024/)
  assert.equal(result.forms.length, 1)
  assert.equal(result.chatBodies.length, 1)
})

test('a resolution change preserving the requested ratio does not warn', async () => {
  const result = await runEdit({
    width: 1600,
    height: 900,
    output: { width: 800, height: 450 },
  })
  assert.equal(result.failure, undefined)
  assert.deepEqual(result.warnings, [])
})

test('a channel rejecting the original size reports its error without a square fallback', async () => {
  const result = await runEdit({ width: 1600, height: 900, rejected: true })
  assert.match(String(result.failure), /Unsupported image size/)
  assert.equal(result.forms.length, 1)
  assert.equal(result.forms[0].get('size'), '1600x900')
  assert.deepEqual(result.images, [])
})

test('new image generation still has a default size when no source exists', async () => {
  let chatCalls = 0
  globalThis.fetch = (async (url, init) => {
    if (url === '/v1/chat/completions') {
      chatCalls += 1
      return Response.json({
        choices: [
          {
            message:
              chatCalls === 1
                ? {
                    content: null,
                    tool_calls: [
                      {
                        id: 'generate-call',
                        type: 'function',
                        function: {
                          name: 'generate_image',
                          arguments: JSON.stringify({
                            prompt: 'A quiet garden',
                          }),
                        },
                      },
                    ],
                  }
                : { content: 'Created.' },
          },
        ],
      })
    }
    assert.equal(url, '/v1/images/generations')
    assert.equal(JSON.parse(String(init?.body)).size, '1024x1024')
    return Response.json({
      data: [{ b64_json: btoa(JSON.stringify({ width: 1024, height: 1024 })) }],
    })
  }) as typeof fetch
  const images: StudioImage[] = []
  await runImageTurn({
    chatKey: 'test',
    imageKey: 'test',
    chatModel: 'chat',
    imageModel: 'image',
    images: [],
    messages: [
      {
        id: 'user',
        role: 'user',
        text: 'Draw a quiet garden',
        imageIds: [],
        marks: [],
      },
    ],
    signal: new AbortController().signal,
    onStage() {},
    onText() {},
    onWarning: (warning) => assert.fail(warning),
    onImage: (image) => images.push(image),
  })
  assert.equal(images.length, 1)
})
