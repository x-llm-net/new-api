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
import { Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'
import { cn } from '@/lib/utils'

type ProviderFilter = 'all' | 'anthropic' | 'openai' | 'google'

type ModelPrice = {
  id: string
  provider: ProviderFilter
  providerLabel: string
  context: string
  officialInput: number
  officialOutput: number
  officialCacheRead?: number
  multiplier: number
}

const USD_CNY_RATE = 6.8

const FILTERS: { id: ProviderFilter; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'anthropic', label: 'Claude' },
  { id: 'openai', label: 'OpenAI' },
  { id: 'google', label: 'Gemini' },
]

const MODEL_PRICES: ModelPrice[] = [
  {
    id: 'claude-opus-4-8',
    provider: 'anthropic',
    providerLabel: 'Anthropic',
    context: '1M',
    officialInput: 5,
    officialOutput: 25,
    officialCacheRead: 0.5,
    multiplier: 0.45,
  },
  {
    id: 'claude-sonnet-4-6',
    provider: 'anthropic',
    providerLabel: 'Anthropic',
    context: '1M',
    officialInput: 3,
    officialOutput: 15,
    officialCacheRead: 0.3,
    multiplier: 0.45,
  },
  {
    id: 'claude-haiku-4-5',
    provider: 'anthropic',
    providerLabel: 'Anthropic',
    context: '200K',
    officialInput: 1,
    officialOutput: 5,
    officialCacheRead: 0.1,
    multiplier: 0.45,
  },
  {
    id: 'gpt-5.5',
    provider: 'openai',
    providerLabel: 'OpenAI',
    context: '1M',
    officialInput: 5,
    officialOutput: 30,
    officialCacheRead: 0.5,
    multiplier: 0.2,
  },
  {
    id: 'gpt-5.4',
    provider: 'openai',
    providerLabel: 'OpenAI',
    context: '1M',
    officialInput: 2.5,
    officialOutput: 15,
    officialCacheRead: 0.25,
    multiplier: 0.2,
  },
  {
    id: 'gpt-5.4-mini',
    provider: 'openai',
    providerLabel: 'OpenAI',
    context: '1M',
    officialInput: 0.75,
    officialOutput: 4.5,
    officialCacheRead: 0.075,
    multiplier: 0.2,
  },
  {
    id: 'gemini-3.1-pro-preview',
    provider: 'google',
    providerLabel: 'Google',
    context: '2M',
    officialInput: 2,
    officialOutput: 12,
    officialCacheRead: 0.2,
    multiplier: 0.6,
  },
  {
    id: 'gemini-3.5-flash',
    provider: 'google',
    providerLabel: 'Google',
    context: '1M',
    officialInput: 1.5,
    officialOutput: 9,
    officialCacheRead: 0.15,
    multiplier: 0.6,
  },
  {
    id: 'gemini-3.1-flash-lite-preview',
    provider: 'google',
    providerLabel: 'Google',
    context: '1M',
    officialInput: 0.25,
    officialOutput: 1.5,
    officialCacheRead: 0.025,
    multiplier: 0.6,
  },
]

function formatUsd(amount: number) {
  return `$${amount.toFixed(4)}`
}

function formatCny(amount: number) {
  return `\u00a5${amount.toFixed(4)}`
}

function formatDiscountValue(multiplier: number) {
  const discount = (multiplier / USD_CNY_RATE) * 10
  return discount.toFixed(1)
}

export function Pricing() {
  const { t } = useTranslation()
  const [activeFilter, setActiveFilter] = useState<ProviderFilter>('all')
  const [query, setQuery] = useState('')

  const filteredPrices = useMemo(() => {
    const keyword = query.trim().toLowerCase()

    return MODEL_PRICES.filter((price) => {
      const matchesFilter =
        activeFilter === 'all' || price.provider === activeFilter
      const matchesQuery =
        keyword.length === 0 ||
        price.id.toLowerCase().includes(keyword) ||
        price.providerLabel.toLowerCase().includes(keyword)

      return matchesFilter && matchesQuery
    })
  }, [activeFilter, query])

  return (
    <section className='dark:bg-background relative z-10 bg-[#f7f7f7] px-6 py-16 md:py-20'>
      <div className='mx-auto max-w-7xl'>
        <AnimateInView>
          <div className='mb-7'>
            <div className='max-w-3xl'>
              <h2 className='text-foreground text-3xl leading-tight font-black tracking-tight md:text-4xl'>
                {t('Live pricing')}
              </h2>
              <p className='text-muted-foreground mt-2 text-sm md:text-base'>
                {t(
                  'Shown prices are estimated from the current lowest available x-llm group. Unit: per 1M tokens.'
                )}
              </p>
              <p className='text-muted-foreground mt-1 text-[11px] leading-relaxed'>
                {t(
                  'Actual billing depends on selected billing group, model route, cache tokens, and settlement records. Equivalent discount uses 1 USD = 6.8 CNY.'
                )}
              </p>
            </div>
          </div>

          <div className='mb-6 flex flex-col gap-4 md:flex-row md:items-center md:justify-between'>
            <div className='flex flex-wrap gap-2'>
              {FILTERS.map((filter) => (
                <button
                  key={filter.id}
                  type='button'
                  onClick={() => setActiveFilter(filter.id)}
                  className={cn(
                    'h-10 rounded-full border px-5 text-sm font-medium transition-colors',
                    activeFilter === filter.id
                      ? 'border-red-500 bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-300'
                      : 'border-border/70 bg-background text-muted-foreground hover:border-red-300 hover:text-foreground'
                  )}
                >
                  {t(filter.label)}
                </button>
              ))}
            </div>

            <label className='relative w-full md:ml-auto md:w-[320px]'>
              <Search className='text-muted-foreground pointer-events-none absolute top-1/2 left-4 size-4 -translate-y-1/2' />
              <input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t('Search models...')}
                className='border-border/70 bg-background placeholder:text-muted-foreground h-12 w-full rounded-lg border pr-4 pl-11 text-sm transition-colors outline-none focus:border-red-500'
              />
            </label>
          </div>

          <div className='border-border/70 bg-background overflow-hidden rounded-2xl border'>
            <div className='overflow-x-auto'>
              <table className='w-full min-w-[1080px] border-collapse text-left'>
                <thead>
                  <tr className='border-border/70 text-foreground/70 dark:bg-muted/30 border-b bg-[#ececec] text-sm'>
                    <th className='w-[260px] px-5 py-4 font-semibold'>
                      {t('Model')}
                    </th>
                    <th className='w-[140px] px-5 py-4 font-semibold'>
                      {t('Provider')}
                    </th>
                    <th className='w-[110px] px-5 py-4 font-semibold'>
                      {t('Context')}
                    </th>
                    <th className='w-[180px] px-5 py-4 font-semibold'>
                      {t('Input official')}
                    </th>
                    <th className='w-[180px] px-5 py-4 font-semibold'>
                      {t('Output official')}
                    </th>
                    <th className='w-[210px] bg-red-50/70 px-5 py-4 font-semibold text-red-700 dark:bg-red-500/10 dark:text-red-300'>
                      {t('Input x-llm')}
                    </th>
                    <th className='w-[210px] bg-red-50/70 px-5 py-4 font-semibold text-red-700 dark:bg-red-500/10 dark:text-red-300'>
                      {t('Output x-llm')}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {filteredPrices.map((price) => {
                    const xllmInput = price.officialInput * price.multiplier
                    const xllmOutput = price.officialOutput * price.multiplier
                    const xllmCacheRead =
                      price.officialCacheRead === undefined
                        ? undefined
                        : price.officialCacheRead * price.multiplier
                    const discount = formatDiscountValue(price.multiplier)

                    return (
                      <tr
                        key={price.id}
                        className='border-border/70 group border-b transition-colors last:border-b-0 hover:bg-red-50/40 dark:hover:bg-red-500/5'
                      >
                        <td className='text-foreground px-5 py-4 text-sm font-bold'>
                          {price.id}
                        </td>
                        <td className='text-foreground/85 px-5 py-4 text-sm font-semibold'>
                          {price.providerLabel}
                        </td>
                        <td className='text-muted-foreground px-5 py-4 text-sm'>
                          {price.context}
                        </td>
                        <td className='px-5 py-4'>
                          <div className='text-foreground decoration-foreground/55 font-mono text-sm line-through'>
                            {formatUsd(price.officialInput)}
                          </div>
                          <div className='text-muted-foreground mt-1 text-xs'>
                            {t('CNY')}:{' '}
                            {formatCny(price.officialInput * USD_CNY_RATE)}
                          </div>
                          {price.officialCacheRead !== undefined ? (
                            <div className='text-muted-foreground mt-0.5 text-xs'>
                              {t('Cache')}: {formatUsd(price.officialCacheRead)}
                            </div>
                          ) : null}
                        </td>
                        <td className='px-5 py-4'>
                          <div className='text-foreground decoration-foreground/55 font-mono text-sm line-through'>
                            {formatUsd(price.officialOutput)}
                          </div>
                          <div className='text-muted-foreground mt-1 text-xs'>
                            {t('CNY')}:{' '}
                            {formatCny(price.officialOutput * USD_CNY_RATE)}
                          </div>
                        </td>
                        <td className='bg-red-50/35 px-5 py-4 transition-colors group-hover:bg-red-50/70 dark:bg-red-500/5 dark:group-hover:bg-red-500/10'>
                          <div className='flex items-center gap-3'>
                            <span className='text-foreground font-mono text-base font-black tabular-nums'>
                              {formatCny(xllmInput)}
                            </span>
                            <span className='rounded-full bg-red-100 px-2 py-0.5 text-xs font-bold text-red-700 dark:bg-red-500/15 dark:text-red-300'>
                              {t('{{value}} equivalent discount', {
                                value: discount,
                              })}
                            </span>
                          </div>
                          {xllmCacheRead !== undefined ? (
                            <div className='text-muted-foreground mt-1 text-xs'>
                              {t('Cache')}: {formatCny(xllmCacheRead)}
                            </div>
                          ) : null}
                        </td>
                        <td className='bg-red-50/35 px-5 py-4 transition-colors group-hover:bg-red-50/70 dark:bg-red-500/5 dark:group-hover:bg-red-500/10'>
                          <div className='flex items-center gap-3'>
                            <span className='text-foreground font-mono text-base font-black tabular-nums'>
                              {formatCny(xllmOutput)}
                            </span>
                            <span className='rounded-full bg-red-100 px-2 py-0.5 text-xs font-bold text-red-700 dark:bg-red-500/15 dark:text-red-300'>
                              {t('{{value}} equivalent discount', {
                                value: discount,
                              })}
                            </span>
                          </div>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>

            {filteredPrices.length === 0 ? (
              <div className='text-muted-foreground px-5 py-10 text-center text-sm'>
                {t('No matching models')}
              </div>
            ) : null}
          </div>
        </AnimateInView>
      </div>
    </section>
  )
}
