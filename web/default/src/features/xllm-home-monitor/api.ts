import { api } from '@/lib/api'

import type { XLLMHomeMonitorSummaryResponse } from './types'

export async function getXLLMHomeMonitorSummary(): Promise<XLLMHomeMonitorSummaryResponse> {
  const res = await api.get<XLLMHomeMonitorSummaryResponse>(
    '/api/xllm/home-monitor/summary',
    {
      disableDuplicate: true,
      headers: {
        'Cache-Control': 'no-store',
        Pragma: 'no-cache',
      },
      params: { _t: Date.now() },
      skipErrorHandler: true,
    }
  )
  return res.data
}
