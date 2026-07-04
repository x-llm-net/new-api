export type ModelMonitorStatus =
  | 'operational'
  | 'degraded'
  | 'unavailable'
  | 'unknown'

export type ModelMonitorModel = {
  model_name: string
  status: ModelMonitorStatus
  available_routes: number
  total_routes: number
  tested_routes: number
  unavailable_routes: number
  unknown_routes: number
  min_latency_ms: number
  avg_latency_ms: number
  last_test_time: number
}

export type ModelMonitorGroup = {
  group_name: string
  status: ModelMonitorStatus
  available_routes: number
  total_routes: number
  tested_routes: number
  unavailable_routes: number
  unknown_routes: number
  model_count: number
  model_names: string[]
  routes: ModelMonitorRoute[]
  min_latency_ms: number
  avg_latency_ms: number
  last_test_time: number
}

export type ModelMonitorRoute = {
  status: number
  test_time: number
  response_time: number
}

export type ModelMonitorTask = {
  status: 'pending' | 'running' | 'succeeded' | 'failed'
  result?: unknown
  error?: string
  created_at: number
  updated_at: number
}

export type ModelMonitorProbe = {
  status: ModelMonitorStatus
  task_status: 'pending' | 'running' | 'succeeded' | 'failed'
  success_rate: number
  tested: number
  succeeded: number
  failed: number
  disabled: number
  enabled: number
  created_at: number
  updated_at: number
}

export type ModelMonitorSummary = {
  models: ModelMonitorModel[]
  groups: ModelMonitorGroup[]
  history: ModelMonitorProbe[]
  total_models: number
  operational_models: number
  degraded_models: number
  unavailable_models: number
  unknown_models: number
  available_routes: number
  total_routes: number
  last_test_time: number
  stale_after_seconds: number
  latest_channel_test?: ModelMonitorTask
}

export type ModelMonitorSummaryResponse = {
  success: boolean
  message?: string
  data: ModelMonitorSummary
}
