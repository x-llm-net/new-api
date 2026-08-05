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

import { api } from '@/lib/api'

import type {
  ApiResponse,
  CreateInvoiceApplicationPayload,
  InvoiceApplication,
  InvoiceApplicationPage,
  InvoiceConfig,
  InvoiceStatus,
  InvoiceSummary,
} from './types'

function requireSuccess<T>(response: ApiResponse<T>): T {
  if (!response.success) {
    throw new Error(response.message || 'Request failed')
  }
  return response.data
}

export async function getInvoiceSummary(): Promise<InvoiceSummary> {
  const response = await api.get<ApiResponse<InvoiceSummary>>(
    '/api/invoice/summary'
  )
  return requireSuccess(response.data)
}

export async function getMyInvoiceApplications(
  page: number,
  size: number
): Promise<InvoiceApplicationPage> {
  const response = await api.get<ApiResponse<InvoiceApplicationPage>>(
    '/api/invoice/applications',
    { params: { page, size } }
  )
  return requireSuccess(response.data)
}

export async function createInvoiceApplication(
  payload: CreateInvoiceApplicationPayload
): Promise<InvoiceApplication> {
  const response = await api.post<ApiResponse<InvoiceApplication>>(
    '/api/invoice/applications',
    payload
  )
  return requireSuccess(response.data)
}

export async function cancelInvoiceApplication(id: number): Promise<void> {
  const response = await api.post<ApiResponse<null>>(
    `/api/invoice/applications/${id}/cancel`
  )
  requireSuccess(response.data)
}

export async function downloadInvoice(
  id: number,
  requestNo: string
): Promise<void> {
  const response = await api.get<Blob>(
    `/api/invoice/applications/${id}/download`,
    { responseType: 'blob', disableDuplicate: true }
  )
  const url = URL.createObjectURL(response.data)
  const link = document.createElement('a')
  link.href = url
  link.download = `invoice-${requestNo}.pdf`
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

export async function getOperatorInvoiceApplications(
  page: number,
  size: number,
  status: InvoiceStatus | ''
): Promise<InvoiceApplicationPage> {
  const response = await api.get<ApiResponse<InvoiceApplicationPage>>(
    '/api/invoice/operator/applications',
    { params: { page, size, status } }
  )
  return requireSuccess(response.data)
}

export async function issueInvoiceApplication(
  id: number,
  invoiceNumber: string,
  file: File
): Promise<void> {
  const form = new FormData()
  form.append('invoice_number', invoiceNumber)
  form.append('file', file)
  const response = await api.post<ApiResponse<null>>(
    `/api/invoice/operator/applications/${id}/issue`,
    form
  )
  requireSuccess(response.data)
}

export async function rejectInvoiceApplication(
  id: number,
  reason: string
): Promise<void> {
  const response = await api.post<ApiResponse<null>>(
    `/api/invoice/operator/applications/${id}/reject`,
    { reason }
  )
  requireSuccess(response.data)
}

export async function redFlushInvoiceApplication(id: number): Promise<void> {
  const response = await api.post<ApiResponse<null>>(
    `/api/invoice/operator/applications/${id}/red-flush`
  )
  requireSuccess(response.data)
}

export async function getInvoiceConfig(): Promise<InvoiceConfig> {
  const response = await api.get<ApiResponse<InvoiceConfig>>(
    '/api/invoice/config'
  )
  return requireSuccess(response.data)
}

export async function updateInvoiceConfig(
  config: InvoiceConfig
): Promise<InvoiceConfig> {
  const response = await api.put<ApiResponse<InvoiceConfig>>(
    '/api/invoice/config',
    config
  )
  return requireSuccess(response.data)
}
