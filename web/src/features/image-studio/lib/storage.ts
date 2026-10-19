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
import type { StudioConversation, StudioImage, StudioSettings } from '../types'

const SETTINGS_DEFAULT: StudioSettings = {
  chatTokenId: null,
  imageTokenId: null,
  chatModel: '',
  imageModel: '',
}

export function loadStudioSettings(scope: string): StudioSettings {
  try {
    const raw = JSON.parse(
      localStorage.getItem(`image-studio:${scope}:settings`) || '{}'
    ) as Partial<StudioSettings>
    return {
      chatTokenId: Number.isSafeInteger(raw.chatTokenId)
        ? (raw.chatTokenId ?? null)
        : null,
      imageTokenId: Number.isSafeInteger(raw.imageTokenId)
        ? (raw.imageTokenId ?? null)
        : null,
      chatModel: typeof raw.chatModel === 'string' ? raw.chatModel : '',
      imageModel: typeof raw.imageModel === 'string' ? raw.imageModel : '',
    }
  } catch {
    return { ...SETTINGS_DEFAULT }
  }
}

export function saveStudioSettings(
  scope: string,
  settings: StudioSettings
): void {
  localStorage.setItem(
    `image-studio:${scope}:settings`,
    JSON.stringify(settings)
  )
}

function openStudioDatabase(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open('xllm-image-studio', 1)
    request.onupgradeneeded = () => {
      request.result
        .createObjectStore('conversations', { keyPath: ['scope', 'id'] })
        .createIndex('scope', 'scope')
      request.result.createObjectStore('images', { keyPath: ['scope', 'id'] })
    }
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
    request.onblocked = () =>
      reject(new Error('Image studio storage is blocked'))
  })
}

export async function saveConversation(
  scope: string,
  conversation: StudioConversation,
  images: StudioImage[]
): Promise<void> {
  const db = await openStudioDatabase()
  try {
    await new Promise<void>((resolve, reject) => {
      const tx = db.transaction(['conversations', 'images'], 'readwrite')
      tx.objectStore('conversations').put({ ...conversation, scope })
      for (const image of images) {
        tx.objectStore('images').put({ ...image, scope })
      }
      tx.oncomplete = () => resolve()
      tx.onabort = () => reject(tx.error)
      tx.onerror = () => reject(tx.error)
    })
  } finally {
    db.close()
  }
}

export async function listConversations(
  scope: string
): Promise<StudioConversation[]> {
  const db = await openStudioDatabase()
  try {
    return await new Promise((resolve, reject) => {
      const request = db
        .transaction('conversations')
        .objectStore('conversations')
        .index('scope')
        .getAll(scope)
      request.onsuccess = () =>
        resolve(
          (request.result as StudioConversation[]).sort(
            (a, b) => b.updatedAt - a.updatedAt
          )
        )
      request.onerror = () => reject(request.error)
    })
  } finally {
    db.close()
  }
}

export async function loadConversationImages(
  scope: string,
  ids: string[]
): Promise<StudioImage[]> {
  const db = await openStudioDatabase()
  try {
    const tx = db.transaction('images')
    return await Promise.all(
      ids.map(
        (id) =>
          new Promise<StudioImage>((resolve, reject) => {
            const request = tx.objectStore('images').get([scope, id])
            request.onsuccess = () =>
              request.result
                ? resolve(request.result as StudioImage)
                : reject(new Error('Image not found'))
            request.onerror = () => reject(request.error)
          })
      )
    )
  } finally {
    db.close()
  }
}

export function newConversation(): StudioConversation {
  return {
    id: crypto.randomUUID(),
    title: '',
    updatedAt: Date.now(),
    messages: [],
    imageIds: [],
  }
}
