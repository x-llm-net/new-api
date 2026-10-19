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

import { fetchTokenKey, getApiKeys } from '@/features/keys/api'
import type { ApiKey } from '@/features/keys/types'

import { fetchStudioModels } from './lib/relay-client'

export async function getStudioTokens(): Promise<ApiKey[]> {
  const tokens: ApiKey[] = []
  let page = 1
  while (true) {
    const result = await getApiKeys({ p: page, size: 100 })
    if (!result.success || !result.data) {
      throw new Error(result.message || t('Could not load API keys'))
    }
    tokens.push(...result.data.items)
    if (!result.data.items.length || tokens.length >= result.data.total) break
    page += 1
  }
  const now = Date.now() / 1000
  return tokens.filter(
    (token) =>
      token.status === 1 &&
      (token.expired_time === -1 || token.expired_time > now) &&
      (token.unlimited_quota || token.remain_quota > 0)
  )
}

export async function getStudioKey(id: number): Promise<string> {
  const result = await fetchTokenKey(id)
  if (!result.success || !result.data?.key) {
    throw new Error(result.message || t('Could not load the selected API key'))
  }
  return result.data.key
}

export async function getStudioCatalog(id: number, signal: AbortSignal) {
  const key = await getStudioKey(id)
  signal.throwIfAborted()
  return fetchStudioModels(key, signal)
}
