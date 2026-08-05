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

export type InvoiceStatus =
  | 'pending'
  | 'issued'
  | 'rejected'
  | 'cancelled'
  | 'red_flushed'

export type InvoiceTitleType = 'personal' | 'company'

export interface InvoiceSummary {
  enabled: boolean
  eligible_amount_cents: number
  pending_amount_cents: number
  issued_amount_cents: number
  available_amount_cents: number
  min_amount_cents: number
}

export interface InvoiceApplication {
  id: number
  user_id: number
  username?: string
  request_no: string
  amount_cents: number
  status: InvoiceStatus
  title_type: InvoiceTitleType
  title_name: string
  tax_number?: string
  email: string
  invoice_content: string
  invoice_number?: string
  reject_reason?: string
  has_file: boolean
  issued_at?: number
  red_flushed_at?: number
  created_at: number
  updated_at: number
}

export interface InvoiceApplicationPage {
  items: InvoiceApplication[]
  total: number
  page: number
  size: number
}

export interface CreateInvoiceApplicationPayload {
  amount_cents: number
  title_type: InvoiceTitleType
  title_name: string
  tax_number: string
  email: string
  idempotency_key: string
}

export interface InvoiceConfig {
  enabled: boolean
  min_amount_cents: number
  history_cutoff: number
  invoice_content: string
  operator_user_ids: number[]
}

export interface ApiResponse<T> {
  success: boolean
  message?: string
  data: T
}
