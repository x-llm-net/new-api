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
export type XLLMGroupStabilityStatus =
  | 'available'
  | 'degraded'
  | 'operational'
  | 'unavailable'
  | 'unknown'

export type XLLMGroupStabilityBucket = {
  available_runs: number
  status: XLLMGroupStabilityStatus
  success_rate: number
  total_runs: number
  ts: number
}

export type XLLMGroupStabilityItem = {
  display_name: string
  group_name: string
  last_tested_at: number
  recent_buckets: XLLMGroupStabilityBucket[]
  recent_success_rate: number
  seven_day_buckets: XLLMGroupStabilityBucket[]
  seven_day_success_rate: number
  status: XLLMGroupStabilityStatus
}

export type XLLMGroupStabilitySummary = {
  enabled: boolean
  generated_at: number
  items: XLLMGroupStabilityItem[]
  overall_status: XLLMGroupStabilityStatus
  recent_window_runs: number
  seven_day_window_hours: number
}
