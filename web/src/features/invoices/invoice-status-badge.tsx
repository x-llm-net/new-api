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

import { FileCheck2, FileClock, FileX2, RotateCcw, XCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusVariant } from '@/components/status-badge'

import { invoiceStatusLabels } from './shared'
import type { InvoiceStatus } from './types'

const statusMeta: Record<
  InvoiceStatus,
  {
    variant: StatusVariant
    icon: typeof FileClock
  }
> = {
  pending: { variant: 'warning', icon: FileClock },
  issued: { variant: 'success', icon: FileCheck2 },
  rejected: { variant: 'danger', icon: FileX2 },
  cancelled: { variant: 'neutral', icon: XCircle },
  red_flushed: { variant: 'neutral', icon: RotateCcw },
}

export function InvoiceStatusBadge({ status }: { status: InvoiceStatus }) {
  const { t } = useTranslation()
  const meta = statusMeta[status]
  return (
    <StatusBadge
      label={t(invoiceStatusLabels[status])}
      variant={meta.variant}
      icon={meta.icon}
      copyable={false}
    />
  )
}
