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

import type { TFunction } from 'i18next'

import type { InvoiceStatus } from './types'

const currencyFormatter = new Intl.NumberFormat('zh-CN', {
  style: 'currency',
  currency: 'CNY',
  minimumFractionDigits: 2,
})

export function formatInvoiceAmount(cents: number): string {
  return currencyFormatter.format(cents / 100)
}

export function formatInvoiceTime(timestamp?: number): string {
  if (!timestamp) return '-'
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(new Date(timestamp * 1000))
}

export const invoiceStatuses: InvoiceStatus[] = [
  'pending',
  'issued',
  'rejected',
  'cancelled',
  'red_flushed',
]

export const invoiceStatusLabels: Record<InvoiceStatus, string> = {
  pending: 'Invoice status pending',
  issued: 'Invoice status issued',
  rejected: 'Invoice status rejected',
  cancelled: 'Invoice status cancelled',
  red_flushed: 'Invoice status red-flushed',
}

export function getInvoiceStatusLabel(status: InvoiceStatus, t: TFunction) {
  return t(invoiceStatusLabels[status])
}
