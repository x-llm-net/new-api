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
import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'

import { getStudioCatalog, getStudioTokens } from '../api'
import { loadStudioSettings, saveStudioSettings } from '../lib/storage'
import type { StudioModel, StudioSettings } from '../types'

export function isImageModel(model: StudioModel): boolean {
  return (
    model.supported_endpoint_types?.some((endpoint) =>
      /image/i.test(endpoint)
    ) === true || /image|dall-e|flux|seedream|imagen/i.test(model.id)
  )
}

export function useStudioSettings(scope: string) {
  const [settings, setSettings] = useState(() => loadStudioSettings(scope))
  const [storageError, setStorageError] = useState(false)
  const tokensQuery = useQuery({
    queryKey: ['image-studio-tokens', scope],
    queryFn: getStudioTokens,
    retry: false,
  })
  const tokens = tokensQuery.data
  const chatToken = tokens?.find((token) => token.id === settings.chatTokenId)
  const imageToken = tokens?.find(
    (token) => token.id === (settings.imageTokenId ?? settings.chatTokenId)
  )
  const chatCatalog = useQuery({
    queryKey: ['image-studio-models', scope, chatToken?.id],
    queryFn: ({ signal }) => getStudioCatalog(chatToken?.id ?? 0, signal),
    enabled: Boolean(chatToken),
    retry: false,
  })
  const imageCatalog = useQuery({
    queryKey: ['image-studio-models', scope, imageToken?.id],
    queryFn: ({ signal }) => getStudioCatalog(imageToken?.id ?? 0, signal),
    enabled: Boolean(imageToken),
    retry: false,
  })

  useEffect(() => {
    if (tokens?.length === 1 && settings.chatTokenId === null) {
      setSettings((current) => ({ ...current, chatTokenId: tokens[0].id }))
    }
  }, [tokens, settings.chatTokenId])
  useEffect(() => {
    if (!settings.chatModel && chatCatalog.data?.length) {
      const model =
        chatCatalog.data.find((item) => !isImageModel(item)) ??
        chatCatalog.data[0]
      setSettings((current) => ({ ...current, chatModel: model.id }))
    }
  }, [chatCatalog.data, settings.chatModel])
  useEffect(() => {
    if (!settings.imageModel && imageCatalog.data?.length) {
      const model =
        imageCatalog.data.find((item) => item.id === 'gpt-image-2.5') ??
        imageCatalog.data.find(isImageModel)
      if (model) {
        setSettings((current) => ({ ...current, imageModel: model.id }))
      }
    }
  }, [imageCatalog.data, settings.imageModel])
  useEffect(() => {
    try {
      saveStudioSettings(scope, settings)
      setStorageError(false)
    } catch {
      setStorageError(true)
    }
  }, [scope, settings])

  const update = (next: Partial<StudioSettings>) =>
    setSettings((current) => ({ ...current, ...next }))
  const ready = Boolean(
    chatToken &&
    imageToken &&
    chatCatalog.data?.some((model) => model.id === settings.chatModel) &&
    imageCatalog.data?.some((model) => model.id === settings.imageModel)
  )
  return {
    settings,
    update,
    tokens: tokens ?? [],
    chatToken,
    imageToken,
    chatModels: chatCatalog.data ?? [],
    imageModels: imageCatalog.data ?? [],
    loading:
      tokensQuery.isFetching ||
      chatCatalog.isFetching ||
      imageCatalog.isFetching,
    error: tokensQuery.error ?? chatCatalog.error ?? imageCatalog.error,
    storageError,
    ready,
    refresh: () => {
      void tokensQuery.refetch()
      if (chatToken) void chatCatalog.refetch()
      if (imageToken && imageToken.id !== chatToken?.id) {
        void imageCatalog.refetch()
      }
    },
  }
}
