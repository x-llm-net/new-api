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

import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ArrowRight, FileText } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'

import { getInvoiceSummary } from './api'
import { formatInvoiceAmount } from './shared'

export function WalletInvoiceEntry() {
  const { t } = useTranslation()
  const summaryQuery = useQuery({
    queryKey: ['invoice', 'summary'],
    queryFn: getInvoiceSummary,
  })

  if (summaryQuery.data && !summaryQuery.data.enabled) return null

  let description = t('View invoice applications and issued invoices')
  if (summaryQuery.data?.enabled) {
    description = t('{{amount}} available for invoicing', {
      amount: formatInvoiceAmount(summaryQuery.data.available_amount_cents),
    })
  }

  return (
    <div className='flex flex-col gap-4 rounded-lg border px-4 py-4 sm:flex-row sm:items-center sm:justify-between sm:px-5'>
      <div className='flex min-w-0 items-center gap-3'>
        <IconBadge tone='info' size='title'>
          <FileText />
        </IconBadge>
        <div className='min-w-0'>
          <div className='font-medium'>{t('Invoices')}</div>
          <div className='text-muted-foreground mt-0.5 text-sm'>
            {summaryQuery.isLoading ? (
              <Skeleton className='h-4 w-40' />
            ) : (
              description
            )}
          </div>
        </div>
      </div>
      <Button variant='outline' render={<Link to='/invoices' />}>
        {t('Invoice center')}
        <ArrowRight />
      </Button>
    </div>
  )
}
