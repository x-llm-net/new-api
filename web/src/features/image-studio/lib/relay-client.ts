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
import { t } from 'i18next'
import { z } from 'zod'

import type { ChatMessage, StudioModel } from '../types'

const modelResponse = z.object({
  data: z.array(
    z.object({
      id: z.string(),
      supported_endpoint_types: z.array(z.string()).optional(),
    })
  ),
})
const chatResponse = z.object({
  choices: z
    .array(
      z.object({
        message: z.object({
          content: z.string().nullable().optional(),
          tool_calls: z
            .array(
              z.object({
                id: z.string(),
                type: z.literal('function'),
                function: z.object({ name: z.string(), arguments: z.string() }),
              })
            )
            .optional(),
        }),
      })
    )
    .min(1),
})
const imageResponse = z.object({
  data: z
    .array(
      z.object({
        b64_json: z.string().optional(),
        url: z.string().optional(),
      })
    )
    .min(1),
})

/** Model authentication is deliberately independent of dashboard JWT refresh. */
export async function relayRequest(
  path:
    | '/v1/models'
    | '/v1/chat/completions'
    | '/v1/images/generations'
    | '/v1/images/edits',
  key: string,
  body?: object | FormData,
  signal?: AbortSignal
): Promise<unknown> {
  const headers: Record<string, string> = {
    Authorization: `Bearer ${key.startsWith('sk-') ? key : `sk-${key}`}`,
  }
  let requestBody: BodyInit | undefined
  if (body instanceof FormData) requestBody = body
  else if (body) {
    headers['Content-Type'] = 'application/json'
    requestBody = JSON.stringify(body)
  }
  const response = await fetch(path, {
    method: body ? 'POST' : 'GET',
    headers,
    body: requestBody,
    signal,
    credentials: 'omit',
    redirect: 'error',
  })
  const data: unknown = await response.json().catch(() => null)
  if (!response.ok) {
    const error = z
      .object({
        error: z.object({ message: z.string() }).optional(),
        message: z.string().optional(),
      })
      .safeParse(data)
    throw new Error(
      error.success
        ? error.data.error?.message ||
            error.data.message ||
            `${t('Image studio request failed')} (${response.status})`
        : `${t('Image studio request failed')} (${response.status})`
    )
  }
  return data
}

export async function fetchStudioModels(
  key: string,
  signal?: AbortSignal
): Promise<StudioModel[]> {
  return modelResponse.parse(
    await relayRequest('/v1/models', key, undefined, signal)
  ).data
}

export async function chatCompletion(
  key: string,
  model: string,
  messages: ChatMessage[],
  tools: object[],
  signal: AbortSignal
) {
  const data = chatResponse.parse(
    await relayRequest(
      '/v1/chat/completions',
      key,
      {
        model,
        messages,
        tools,
        tool_choice: 'auto',
        stream: false,
      },
      signal
    )
  )
  return data.choices[0].message
}

export async function requestImage(input: {
  key: string
  model: string
  prompt: string
  size: string
  images?: Blob[]
  signal: AbortSignal
}): Promise<Blob> {
  let body: object | FormData
  if (input.images?.length) {
    const form = new FormData()
    form.append('model', input.model)
    form.append('prompt', input.prompt)
    form.append('n', '1')
    form.append('size', input.size)
    const field = input.images.length > 1 ? 'image[]' : 'image'
    const extensions: Record<string, string> = {
      'image/jpeg': 'jpg',
      'image/webp': 'webp',
      'image/png': 'png',
    }
    input.images.forEach((blob, index) =>
      form.append(
        field,
        blob,
        `reference-${index + 1}.${extensions[blob.type] ?? 'png'}`
      )
    )
    body = form
  } else {
    body = { model: input.model, prompt: input.prompt, n: 1, size: input.size }
  }
  const data = imageResponse.parse(
    await relayRequest(
      input.images?.length ? '/v1/images/edits' : '/v1/images/generations',
      input.key,
      body,
      input.signal
    )
  )
  const image = data.data[0]
  if (image.b64_json) {
    const raw = atob(image.b64_json)
    if (raw.length > 30 * 1024 * 1024) {
      throw new Error(t('Image exceeds the 30 MB limit'))
    }
    return new Blob(
      [Uint8Array.from(raw, (character) => character.charCodeAt(0))],
      { type: 'image/png' }
    )
  }
  if (!image.url) throw new Error(t('The model returned no image'))
  const url = new URL(image.url)
  if (url.protocol !== 'https:' && url.protocol !== 'http:') {
    throw new Error(t('Invalid image URL'))
  }
  // Never forward the model key or dashboard cookies to an upstream image URL.
  const response = await fetch(url, {
    signal: input.signal,
    credentials: 'omit',
    referrerPolicy: 'no-referrer',
  })
  if (!response.ok) throw new Error(t('Could not download the generated image'))
  const blob = await response.blob()
  if (!blob.type.startsWith('image/') || blob.size > 30 * 1024 * 1024) {
    throw new Error(t('Invalid image or image exceeds the 30 MB limit'))
  }
  return blob
}
