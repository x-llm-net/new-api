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
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { getStudioKey } from '../api'
import { runImageTurn } from '../lib/agent'
import { importStudioImage } from '../lib/images'
import {
  listConversations,
  loadConversationImages,
  newConversation,
  saveConversation,
} from '../lib/storage'
import type {
  ImageMark,
  StudioConversation,
  StudioImage,
  StudioMessage,
  StudioSettings,
  StudioStage,
} from '../types'

interface StudioState {
  conversation: StudioConversation
  images: StudioImage[]
}

export function useStudioConversation(scope: string) {
  const { t } = useTranslation()
  const [state, setState] = useState<StudioState>(() => ({
    conversation: newConversation(),
    images: [],
  }))
  const latest = useRef(state)
  const [history, setHistory] = useState<StudioConversation[]>([])
  const [busy, setBusy] = useState(false)
  const [stage, setStage] = useState<StudioStage>('thinking')
  const [error, setError] = useState('')
  const operation = useRef<AbortController | null>(null)
  const mounted = useRef(true)
  const writes = useRef(Promise.resolve())

  useEffect(() => {
    mounted.current = true
    void listConversations(scope)
      .then((items) => {
        if (mounted.current) setHistory(items)
      })
      .catch(() => {
        if (mounted.current) {
          setError(
            t('Local history is unavailable. Download images to keep them.')
          )
        }
      })
    return () => {
      mounted.current = false
      operation.current?.abort()
    }
  }, [scope, t])

  const commit = useCallback(
    (next: StudioState, persist = true) => {
      if (!mounted.current) return
      latest.current = next
      setState(next)
      if (
        !persist ||
        (!next.conversation.messages.length && !next.images.length)
      ) {
        return
      }
      writes.current = writes.current
        .then(async () => {
          await saveConversation(scope, next.conversation, next.images)
          const items = await listConversations(scope)
          if (mounted.current) setHistory(items)
        })
        .catch(() => {
          if (mounted.current) {
            setError(
              t('Could not save locally. Download your images before leaving.')
            )
          }
        })
    },
    [scope, t]
  )

  const selectImage = (id: string) => {
    const current = latest.current
    if (!current.images.some((image) => image.id === id)) return
    commit({
      ...current,
      conversation: { ...current.conversation, selectedImageId: id },
    })
  }

  const upload = async (files: File[]) => {
    if (operation.current || !files.length) return
    const controller = new AbortController()
    operation.current = controller
    setBusy(true)
    setError('')
    try {
      const added = await Promise.all(
        files.slice(0, 4).map((file) => importStudioImage(file, file.name))
      )
      controller.signal.throwIfAborted()
      const current = latest.current
      commit({
        images: [...current.images, ...added],
        conversation: {
          ...current.conversation,
          imageIds: [
            ...current.conversation.imageIds,
            ...added.map((image) => image.id),
          ],
          selectedImageId: added.at(-1)?.id,
          updatedAt: Date.now(),
        },
      })
      return added
    } catch (cause) {
      if (mounted.current) {
        setError(
          cause instanceof Error ? cause.message : t('Could not read the image')
        )
      }
    } finally {
      if (operation.current === controller) operation.current = null
      if (mounted.current) setBusy(false)
    }
  }

  const send = (
    text: string,
    marks: ImageMark[],
    attachedIds: string[],
    settings: StudioSettings
  ): boolean => {
    if (
      operation.current ||
      !text.trim() ||
      !settings.chatTokenId ||
      !settings.chatModel ||
      !settings.imageModel
    ) {
      return false
    }
    const controller = new AbortController()
    operation.current = controller
    setBusy(true)
    setStage('thinking')
    setError('')
    const current = latest.current
    const user: StudioMessage = {
      id: crypto.randomUUID(),
      role: 'user',
      text: text.trim(),
      marks: structuredClone(marks),
      imageIds: [...attachedIds],
    }
    const assistant: StudioMessage = {
      id: crypto.randomUUID(),
      role: 'assistant',
      text: '',
      marks: [],
      imageIds: [],
    }
    const messages = [...current.conversation.messages, user]
    const conversation = {
      ...current.conversation,
      title: current.conversation.title || text.trim().slice(0, 48),
      updatedAt: Date.now(),
      messages: [...messages, assistant],
    }
    commit({ ...current, conversation })

    const updateAssistant = (
      update: Partial<StudioMessage>,
      persist = false
    ) => {
      if (controller.signal.aborted || !mounted.current) return
      const value = latest.current
      commit(
        {
          ...value,
          conversation: {
            ...value.conversation,
            messages: value.conversation.messages.map((message) =>
              message.id === assistant.id ? { ...message, ...update } : message
            ),
          },
        },
        persist
      )
    }

    void (async () => {
      try {
        const chatKey = await getStudioKey(settings.chatTokenId ?? 0)
        controller.signal.throwIfAborted()
        const imageKey =
          settings.imageTokenId &&
          settings.imageTokenId !== settings.chatTokenId
            ? await getStudioKey(settings.imageTokenId)
            : chatKey
        controller.signal.throwIfAborted()
        let reply = ''
        await runImageTurn({
          chatKey,
          imageKey,
          chatModel: settings.chatModel,
          imageModel: settings.imageModel,
          messages,
          images: current.images,
          currentImageId: current.conversation.selectedImageId,
          signal: controller.signal,
          onStage: (next) => {
            if (mounted.current) setStage(next)
          },
          onText: (textPart) => {
            reply += (reply ? '\n\n' : '') + textPart
            updateAssistant({ text: reply })
          },
          onWarning: (warning) => updateAssistant({ warning }, true),
          onImage: (image) => {
            const value = latest.current
            const updated = value.conversation.messages.map((message) =>
              message.id === assistant.id
                ? { ...message, imageIds: [...message.imageIds, image.id] }
                : message
            )
            commit({
              images: [...value.images, image],
              conversation: {
                ...value.conversation,
                messages: updated,
                imageIds: [...value.conversation.imageIds, image.id],
                selectedImageId: image.id,
                updatedAt: Date.now(),
              },
            })
          },
        })
      } catch (cause) {
        if (!mounted.current) return
        const value = latest.current
        let messageError =
          cause instanceof Error
            ? cause.message
            : t('Image studio request failed')
        if (controller.signal.aborted) messageError = t('Stopped')
        commit({
          ...value,
          conversation: {
            ...value.conversation,
            messages: value.conversation.messages.map((message) =>
              message.id === assistant.id
                ? { ...message, error: messageError }
                : message
            ),
          },
        })
      } finally {
        if (operation.current === controller) operation.current = null
        if (mounted.current) {
          setBusy(false)
          commit(latest.current)
        }
      }
    })()
    return true
  }

  const open = async (conversation: StudioConversation) => {
    if (operation.current) return
    const controller = new AbortController()
    operation.current = controller
    setBusy(true)
    setError('')
    try {
      const images = await loadConversationImages(scope, conversation.imageIds)
      controller.signal.throwIfAborted()
      commit({ conversation, images }, false)
    } catch {
      if (mounted.current) setError(t('Could not load this creation'))
    } finally {
      if (operation.current === controller) operation.current = null
      if (mounted.current) setBusy(false)
    }
  }

  const startNew = () => {
    if (!operation.current) {
      setError('')
      commit({ conversation: newConversation(), images: [] }, false)
    }
  }
  return {
    ...state,
    history,
    busy,
    stage,
    error,
    send,
    upload,
    selectImage,
    open,
    startNew,
    stop: () => operation.current?.abort(),
  }
}
