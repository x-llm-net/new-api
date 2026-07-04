export type XLLMHomeMonitorStatus =
  | 'operational'
  | 'degraded'
  | 'unavailable'
  | 'unknown'

export type XLLMHomeMonitorBucket = {
  ts: number
  status: XLLMHomeMonitorStatus
  success_rate: number
  avg_ttft_ms: number
  p50_ttft_ms: number
  sample_count: number
  success_count: number
  failure_count: number
  ttft_count: number
  last_tested_at: number
}

export type XLLMHomeMonitorItem = {
  group_name: string
  model_name: string
  display_name: string
  status: XLLMHomeMonitorStatus
  success_rate_60m: number
  success_rate_7d: number
  avg_ttft_ms_60m: number
  avg_ttft_ms_7d: number
  p50_ttft_ms_60m: number
  p50_ttft_ms_7d: number
  latest_ttft_ms: number
  last_tested_at: number
  recent_buckets: XLLMHomeMonitorBucket[]
  seven_day_buckets: XLLMHomeMonitorBucket[]
  configured: boolean
  stream_only: boolean
  has_reliable_ttft: boolean
}

export type XLLMHomeMonitorSummary = {
  items: XLLMHomeMonitorItem[]
  total_items: number
  operational_items: number
  degraded_items: number
  unavailable_items: number
  unknown_items: number
  overall_status: XLLMHomeMonitorStatus
  last_tested_at: number
  recent_window_secs: number
  seven_day_window_secs: number
}

export type XLLMHomeMonitorSummaryResponse = {
  success: boolean
  message?: string
  data: XLLMHomeMonitorSummary
}
