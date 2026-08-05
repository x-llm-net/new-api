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

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Settings } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { TitledCard } from '@/components/ui/titled-card'

import { getInvoiceConfig, updateInvoiceConfig } from './api'

function toDateTimeLocal(timestamp: number): string {
  if (!timestamp) return ''
  const date = new Date(timestamp * 1000)
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
  return local.toISOString().slice(0, 16)
}

export function InvoiceConfigCard() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const configQuery = useQuery({
    queryKey: ['invoice', 'config'],
    queryFn: getInvoiceConfig,
  })
  const [enabled, setEnabled] = useState(false)
  const [minAmount, setMinAmount] = useState('100')
  const [historyCutoff, setHistoryCutoff] = useState('')
  const [invoiceContent, setInvoiceContent] = useState('')
  const [operatorIDs, setOperatorIDs] = useState('')

  useEffect(() => {
    if (!configQuery.data) return
    setEnabled(configQuery.data.enabled)
    setMinAmount((configQuery.data.min_amount_cents / 100).toFixed(2))
    setHistoryCutoff(toDateTimeLocal(configQuery.data.history_cutoff))
    setInvoiceContent(configQuery.data.invoice_content)
    setOperatorIDs((configQuery.data.operator_user_ids ?? []).join(', '))
  }, [configQuery.data])

  const mutation = useMutation({
    mutationFn: updateInvoiceConfig,
    onSuccess: async () => {
      toast.success(t('Invoice settings saved'))
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['invoice', 'config'] }),
        queryClient.invalidateQueries({ queryKey: ['invoice', 'summary'] }),
      ])
    },
  })

  const save = () => {
    const minAmountCents = Math.round(Number(minAmount) * 100)
    const cutoff = historyCutoff
      ? Math.floor(new Date(historyCutoff).getTime() / 1000)
      : 0
    const ids = operatorIDs
      .split(/[\s,，]+/)
      .filter(Boolean)
      .map(Number)

    if (
      !Number.isInteger(minAmountCents) ||
      minAmountCents <= 0 ||
      ids.some((id) => !Number.isInteger(id) || id <= 0)
    ) {
      toast.error(t('Check the minimum amount and operator user IDs'))
      return
    }
    if (enabled && (!cutoff || !invoiceContent.trim())) {
      toast.error(
        t('Set the history start time and invoice item before enabling')
      )
      return
    }

    mutation.mutate({
      enabled,
      min_amount_cents: minAmountCents,
      history_cutoff: cutoff,
      invoice_content: invoiceContent.trim(),
      operator_user_ids: [...new Set(ids)],
    })
  }

  return (
    <TitledCard
      title={t('Invoice settings')}
      description={t(
        'Configure the application scope and authorized finance accounts.'
      )}
      icon={<Settings />}
      iconTone='neutral'
      disableHoverEffect
      action={
        <Button onClick={save} disabled={mutation.isPending}>
          {t('Save')}
        </Button>
      }
    >
      {configQuery.isLoading ? (
        <div className='grid gap-4 sm:grid-cols-2'>
          {['enabled', 'minimum', 'cutoff', 'operators'].map((key) => (
            <Skeleton key={key} className='h-16 w-full' />
          ))}
        </div>
      ) : (
        <div className='grid gap-5 sm:grid-cols-2'>
          <div className='flex items-center justify-between gap-4 rounded-lg border px-3 py-3 sm:col-span-2'>
            <div>
              <Label htmlFor='invoice-enabled'>
                {t('Accept invoice applications')}
              </Label>
              <p className='text-muted-foreground mt-1 text-xs'>
                {t('Turning this off does not delete existing applications.')}
              </p>
            </div>
            <Switch
              id='invoice-enabled'
              checked={enabled}
              onCheckedChange={setEnabled}
            />
          </div>
          <div className='grid gap-2'>
            <Label htmlFor='invoice-min-amount'>
              {t('Minimum invoice amount')}
            </Label>
            <Input
              id='invoice-min-amount'
              type='number'
              min='0.01'
              step='0.01'
              value={minAmount}
              onChange={(event) => setMinAmount(event.target.value)}
            />
          </div>
          <div className='grid gap-2'>
            <Label htmlFor='invoice-history-cutoff'>
              {t('History start time')}
            </Label>
            <Input
              id='invoice-history-cutoff'
              type='datetime-local'
              value={historyCutoff}
              onChange={(event) => setHistoryCutoff(event.target.value)}
            />
          </div>
          <div className='grid gap-2'>
            <Label htmlFor='invoice-content'>{t('Invoice item')}</Label>
            <Input
              id='invoice-content'
              maxLength={191}
              value={invoiceContent}
              onChange={(event) => setInvoiceContent(event.target.value)}
              placeholder={t('Example: Information technology services')}
            />
          </div>
          <div className='grid gap-2'>
            <Label htmlFor='invoice-operators'>
              {t('Finance operator user IDs')}
            </Label>
            <Input
              id='invoice-operators'
              value={operatorIDs}
              onChange={(event) => setOperatorIDs(event.target.value)}
              placeholder={t('Separate multiple user IDs with commas')}
            />
          </div>
        </div>
      )}
    </TitledCard>
  )
}
