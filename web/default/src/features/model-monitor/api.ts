import { api } from '@/lib/api'

import type { ModelMonitorSummaryResponse } from './types'

export async function getModelMonitorSummary(): Promise<ModelMonitorSummaryResponse> {
  const res = await api.get<ModelMonitorSummaryResponse>(
    '/api/model-monitor/summary'
  )
  return res.data
}
