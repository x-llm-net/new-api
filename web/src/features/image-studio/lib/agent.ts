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

import type {
  ChatMessage,
  ChatPart,
  ImageMark,
  StudioImage,
  StudioMessage,
  StudioStage,
} from '../types'
import { blobDataUrl, importStudioImage, renderImageContext } from './images'
import { chatCompletion, requestImage } from './relay-client'

const imageArguments = z.object({
  prompt: z.string().trim().min(1).max(16000),
  size: z.enum(['1024x1024', '1536x1024', '1024x1536']).optional(),
  source_image_id: z.string().optional(),
  reference_image_ids: z.array(z.string()).max(4).optional().default([]),
})
const tools = [
  {
    type: 'function',
    function: {
      name: 'generate_image',
      description:
        'Create a new image with the configured image model. Use edit_image when modifying or using an existing reference image.',
      parameters: {
        type: 'object',
        properties: {
          prompt: { type: 'string' },
          size: {
            type: 'string',
            enum: ['1024x1024', '1536x1024', '1024x1536'],
          },
        },
        required: ['prompt'],
      },
    },
  },
  {
    type: 'function',
    function: {
      name: 'edit_image',
      description:
        'Edit only the requested details of the actual source image. Preserve its canvas size, aspect ratio, composition and unrelated content unless the user explicitly requests changing them. Numbered user marks are automatically sent as a visual location guide. Source and references must be image IDs supplied in the current visual context.',
      parameters: {
        type: 'object',
        properties: {
          prompt: { type: 'string' },
          source_image_id: { type: 'string' },
          reference_image_ids: { type: 'array', items: { type: 'string' } },
          size: {
            type: 'string',
            enum: ['1024x1024', '1536x1024', '1024x1536'],
            description:
              'Omit for ordinary detail edits: the original image dimensions are inherited automatically. Set only when the user explicitly requests a different canvas size or aspect ratio.',
          },
        },
        required: ['prompt', 'source_image_id'],
      },
    },
  },
]

export interface ImageTurn {
  chatKey: string
  imageKey: string
  chatModel: string
  imageModel: string
  messages: StudioMessage[]
  images: StudioImage[]
  currentImageId?: string
  signal: AbortSignal
  onStage(stage: StudioStage): void
  onText(text: string): void
  onImage(image: StudioImage): void
  onWarning(warning: string): void
}

export async function runImageTurn(input: ImageTurn): Promise<void> {
  const latest = input.messages.at(-1)
  if (!latest || latest.role !== 'user') {
    throw new Error(t('No message to send'))
  }
  const selectedIds = new Set([
    ...latest.imageIds,
    ...latest.marks.map((mark) => mark.imageId),
  ])
  if (input.currentImageId) selectedIds.add(input.currentImageId)
  const available = new Map(
    input.images
      .filter((image) => selectedIds.has(image.id))
      .map((image) => [image.id, image])
  )
  const context: ChatPart[] = [
    {
      type: 'text',
      text: `Current image: ${input.currentImageId || 'none'}. Image IDs below refer to immutable originals. [number] in the user message refers to that exact marked position, not a guessed object name. A visible image does not itself request editing.`,
    },
  ]
  for (const image of available.values()) {
    input.signal.throwIfAborted()
    const marks = latest.marks.filter((mark) => mark.imageId === image.id)
    context.push({
      type: 'text',
      text: `Image ${image.id}: ${image.name}. Original dimensions: ${image.width}x${image.height} pixels. ${describeMarks(marks)}`,
    })
    context.push({
      type: 'image_url',
      image_url: {
        url: await blobDataUrl(await renderImageContext(image, marks)),
      },
    })
  }
  const messages: ChatMessage[] = [
    {
      role: 'system',
      content:
        'You are a collaborative image creation assistant. Reply in the user’s language. Discuss ideas naturally; use the image tools when the user requests creation or changes. For changes to an existing picture always use edit_image with its actual image ID, never redraw it from text alone. Inspect the supplied visual guide before describing the target. User coordinates take priority over guesses about room or object names. Preserve the user’s natural-language intent; do not invent design decisions. Guides and numeric markers are annotations, never draw them into the artwork. Only report a successful image after a tool succeeds. Keep replies concise. Do not expose internal IDs or credentials. The user may select an earlier version to edit it. Images are generated by a separate configured image model, not by your text response.',
    },
  ]
  for (const message of input.messages.slice(-30)) {
    messages.push({
      role: message.role,
      content:
        message.text +
        (message.imageIds.length
          ? `\n[Attached image IDs: ${message.imageIds.join(', ')}]`
          : '') +
        (message.error
          ? '\n[This previous request failed or was stopped.]'
          : '') +
        (message.warning ? `\n[Image result warning: ${message.warning}]` : ''),
    })
  }
  messages.push({ role: 'user', content: context })
  let completedImages = 0
  for (let round = 0; round < 6; round += 1) {
    input.signal.throwIfAborted()
    input.onStage('thinking')
    const reply = await chatCompletion(
      input.chatKey,
      input.chatModel,
      messages,
      tools,
      input.signal
    )
    input.signal.throwIfAborted()
    if (reply.content) input.onText(reply.content)
    const calls = reply.tool_calls ?? []
    if (!calls.length) {
      if (!reply.content && !completedImages) {
        throw new Error(
          t('The model returned no reply. Try another chat model.')
        )
      }
      return
    }
    messages.push({
      role: 'assistant',
      content: reply.content ?? null,
      tool_calls: calls,
    })
    for (const call of calls) {
      if (!['generate_image', 'edit_image'].includes(call.function.name)) {
        throw new Error(t('The model requested an unsupported tool'))
      }
      if (completedImages >= 4) {
        throw new Error(
          t('Four images completed. Send another message to continue.')
        )
      }
      const args = imageArguments.parse(JSON.parse(call.function.arguments))
      const sources: Blob[] = []
      let source: StudioImage | undefined
      let prompt = args.prompt
      // New images retain their default; detail edits inherit the actual source.
      let size: string = args.size ?? '1024x1024'
      if (call.function.name === 'edit_image') {
        source = available.get(args.source_image_id ?? '')
        if (!source) {
          throw new Error(t('Select the image you want to edit and try again'))
        }
        size = args.size ?? `${source.width}x${source.height}`
        prompt += `\nImage 1 is the clean original to edit, ${source.width}x${source.height} pixels. Make only the changes the user requested. Preserve the composition, framing, perspective, style, lighting and all unrelated content unless the requested change requires otherwise.`
        prompt +=
          args.size && args.size !== `${source.width}x${source.height}`
            ? ` The user requested an output canvas of ${size}; adapt the framing only as needed and do not distort objects.`
            : ` Preserve the original ${size} canvas and aspect ratio. Do not crop, expand, add borders, stretch or recompose the image.`
        sources.push(source.blob)
        const marks = latest.marks.filter((mark) => mark.imageId === source?.id)
        if (marks.length) {
          sources.push(await renderImageContext(source, marks))
          prompt += `\nImage 1 is the clean original to edit. Image 2 is ONLY a location guide. ${describeMarks(marks)} Follow the marked location, even if a verbal object name conflicts. Do not draw the numbered badges or outlines. Preserve content outside the selected area unless requested otherwise.`
        }
        for (const id of args.reference_image_ids.filter(
          (id) => id !== source?.id
        )) {
          const reference = available.get(id)
          if (!reference) {
            throw new Error(
              t('A reference image is unavailable. Select it again.')
            )
          }
          sources.push(reference.blob)
        }
      }
      input.onStage(source ? 'editing' : 'generating')
      const blob = await requestImage({
        key: input.imageKey,
        model: input.imageModel,
        prompt,
        size,
        images: sources.length ? sources : undefined,
        signal: input.signal,
      })
      input.signal.throwIfAborted()
      const image = await importStudioImage(
        blob,
        args.prompt.slice(0, 80),
        source?.id
      )
      input.signal.throwIfAborted()
      available.set(image.id, image)
      input.onImage(image)
      completedImages += 1
      if (source) {
        const [expectedWidth, expectedHeight] = size.split('x').map(Number)
        const ratioDifference = Math.abs(
          image.width / image.height / (expectedWidth / expectedHeight) - 1
        )
        // Allow minor pixel rounding, but never disguise an upstream reframe.
        if (ratioDifference > 0.01) {
          input.onWarning(
            t(
              'The image model returned {{actual}} instead of the requested {{expected}} aspect ratio. Both the original and this result are saved.',
              {
                actual: `${image.width}×${image.height}`,
                expected: `${expectedWidth}×${expectedHeight}`,
              }
            )
          )
          // Do not let the Agent retry a potentially billed image automatically.
          return
        }
      }
      messages.push({
        role: 'tool',
        tool_call_id: call.id,
        content: JSON.stringify({
          image_id: image.id,
          parent_image_id: source?.id,
          width: image.width,
          height: image.height,
          status: 'completed',
          displayed_to_user: true,
        }),
      })
    }
  }
  throw new Error(
    t('This turn reached its limit. Send another message to continue.')
  )
}

export function describeMarks(marks: ImageMark[]): string {
  return marks
    .map(
      (mark) =>
        `Mark [${mark.number}]: x=${mark.region.x}, y=${mark.region.y}, width=${mark.region.w}, height=${mark.region.h} (normalized original coordinates, top-left origin; zero size means a point).`
    )
    .join('\n')
}
