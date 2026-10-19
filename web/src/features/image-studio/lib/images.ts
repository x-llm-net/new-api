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

import type { ImageMark, ImageRegion, StudioImage } from '../types'

export function regionFromPoints(
  start: { x: number; y: number },
  end: { x: number; y: number },
  point: boolean
): ImageRegion {
  const x1 = Math.max(0, Math.min(1, start.x)),
    y1 = Math.max(0, Math.min(1, start.y))
  const x2 = Math.max(0, Math.min(1, end.x)),
    y2 = Math.max(0, Math.min(1, end.y))
  if (point) return { x: x1, y: y1, w: 0, h: 0 }
  return {
    x: Math.min(x1, x2),
    y: Math.min(y1, y2),
    w: Math.abs(x2 - x1),
    h: Math.abs(y2 - y1),
  }
}

export async function importStudioImage(
  blob: Blob,
  name: string,
  parentId?: string
): Promise<StudioImage> {
  if (
    !['image/png', 'image/jpeg', 'image/webp'].includes(blob.type) ||
    blob.size > 30 * 1024 * 1024
  ) {
    throw new Error(t('Use a PNG, JPEG or WebP image under 30 MB'))
  }
  const bitmap = await createImageBitmap(blob)
  const image = {
    id: crypto.randomUUID(),
    blob,
    name,
    parentId,
    width: bitmap.width,
    height: bitmap.height,
    createdAt: Date.now(),
  }
  bitmap.close()
  return image
}

export async function renderImageContext(
  image: StudioImage,
  marks: ImageMark[] = []
): Promise<Blob> {
  const bitmap = await createImageBitmap(image.blob)
  const ratio = Math.min(1, 1400 / Math.max(bitmap.width, bitmap.height))
  const canvas = document.createElement('canvas')
  canvas.width = Math.max(1, Math.round(bitmap.width * ratio))
  canvas.height = Math.max(1, Math.round(bitmap.height * ratio))
  const ctx = canvas.getContext('2d')
  if (!ctx) {
    bitmap.close()
    throw new Error(t('Image preview is unavailable'))
  }
  ctx.drawImage(bitmap, 0, 0, canvas.width, canvas.height)
  bitmap.close()
  for (const mark of marks.filter((item) => item.imageId === image.id)) {
    const r = mark.region,
      x = r.x * canvas.width,
      y = r.y * canvas.height
    for (const [color, width] of [
      ['white', 6],
      ['#2563eb', 3],
    ] as const) {
      ctx.strokeStyle = color
      ctx.lineWidth = width
      ctx.beginPath()
      if (r.w > 0) ctx.rect(x, y, r.w * canvas.width, r.h * canvas.height)
      else {
        ctx.arc(x, y, 12, 0, 2 * Math.PI)
        ctx.moveTo(x - 20, y)
        ctx.lineTo(x + 20, y)
        ctx.moveTo(x, y - 20)
        ctx.lineTo(x, y + 20)
      }
      ctx.stroke()
    }
    const labelX = Math.max(15, Math.min(canvas.width - 15, x)),
      labelY = Math.max(15, Math.min(canvas.height - 15, y))
    ctx.fillStyle = '#2563eb'
    ctx.beginPath()
    ctx.arc(labelX, labelY, 13, 0, Math.PI * 2)
    ctx.fill()
    ctx.fillStyle = 'white'
    ctx.font = 'bold 14px sans-serif'
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillText(String(mark.number), labelX, labelY)
  }
  return new Promise((resolve, reject) =>
    canvas.toBlob(
      (blob) =>
        blob
          ? resolve(blob)
          : reject(new Error(t('Could not prepare the image'))),
      'image/png'
    )
  )
}

export function blobDataUrl(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result))
    reader.onerror = () => reject(new Error(t('Could not read the image')))
    reader.readAsDataURL(blob)
  })
}
