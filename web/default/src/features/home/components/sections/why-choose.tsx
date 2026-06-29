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
import { Link } from '@tanstack/react-router'
import { Gauge, KeyRound, Layers3, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'

const REASONS = [
  {
    icon: KeyRound,
    title: 'One key, access every model.',
    description:
      'No need to register OpenAI, Anthropic, and Google separately. Configure one key and your agents can reach the model families they need.',
  },
  {
    icon: Layers3,
    title: 'No more rate-limit and token anxiety.',
    description:
      'Choose Plus, Pro, or other billing groups for different workloads, and keep cost-heavy experiments away from your main production routes.',
  },
  {
    icon: Gauge,
    title: 'Stable service, worry-free usage.',
    description:
      'LLMHub Radar shows availability, first-token latency, and incident updates, so you can switch routes based on real service signals.',
  },
] as const

export function WhyChoose() {
  const { t } = useTranslation()

  return (
    <section className='dark:bg-muted/10 relative z-10 bg-[#f3f3f3] px-6 py-16 md:py-20'>
      <div className='mx-auto max-w-7xl'>
        <AnimateInView className='mb-16 grid gap-10 lg:grid-cols-[minmax(0,1fr)_460px] lg:items-center'>
          <div>
            <div className='mb-7 inline-flex rounded-full bg-red-700 px-4 py-2 text-sm font-bold text-white shadow-sm'>
              {t('Pay as you go')}
            </div>
            <h2 className='text-foreground text-5xl leading-none font-semibold tracking-tight md:text-6xl lg:text-7xl'>
              {t('Every token')}
              <br />
              {t('counts for more')}
            </h2>
            <p className='text-muted-foreground mt-7 max-w-2xl text-lg leading-9'>
              {t(
                'Pay by usage, top up whenever you need, and let each model route use the billing group that matches its workload.'
              )}
            </p>
          </div>

          <div className='border-border/70 bg-background overflow-hidden rounded-2xl border shadow-[0_24px_90px_-62px_rgb(15_23_42/0.55)]'>
            <div className='space-y-4 p-6'>
              <div className='border-border/70 bg-muted/30 text-muted-foreground inline-flex items-center gap-2 rounded-full border px-4 py-2 text-sm font-medium'>
                <Zap className='size-4 text-red-700' />
                {t('Transparent pricing - No monthly fee - No subscription')}
              </div>
              <div className='border-border/70 bg-muted/20 rounded-xl border p-5'>
                <div className='flex items-start justify-between gap-4'>
                  <div>
                    <div className='text-foreground text-2xl font-black'>
                      {t('As low as 0.3x after FX')}
                    </div>
                    <p className='text-muted-foreground mt-2 text-sm'>
                      {t('Automatically priced by selected billing group.')}
                    </p>
                  </div>
                  <div className='group relative'>
                    <button
                      type='button'
                      aria-label={t('Pricing discount note')}
                      className='border-border/70 text-muted-foreground focus-visible:ring-ring flex size-8 items-center justify-center rounded-full border text-sm font-bold transition-colors hover:border-red-200 hover:bg-red-50 hover:text-red-700 focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none dark:hover:border-red-500/30 dark:hover:bg-red-500/10 dark:hover:text-red-300'
                    >
                      ?
                    </button>
                    <div
                      role='tooltip'
                      className='border-border/70 bg-background text-foreground pointer-events-none absolute top-[calc(100%+10px)] right-0 z-20 w-72 rounded-lg border px-4 py-3 text-left text-xs leading-5 opacity-0 shadow-[0_18px_60px_-30px_rgb(15_23_42/0.55)] transition-opacity group-focus-within:opacity-100 group-hover:opacity-100'
                    >
                      {t(
                        'Discounts may change with upstream costs. See the live pricing catalog for current rates.'
                      )}
                    </div>
                  </div>
                </div>
              </div>
              <div className='border-border/70 bg-muted/20 text-muted-foreground rounded-xl border px-5 py-4 text-sm font-medium'>
                {t('CNY credits - No minimum spend - Recharge anytime')}
              </div>
            </div>
            <div className='border-border/70 bg-muted/10 border-t p-6'>
              <Link
                to='/wallet'
                className='focus-visible:ring-ring flex h-12 items-center justify-center rounded-lg bg-red-700 text-sm font-bold text-white transition-colors hover:bg-red-700 focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
              >
                {t('Buy credits')}
              </Link>
            </div>
          </div>
        </AnimateInView>

        <AnimateInView className='mb-7'>
          <h2 className='text-2xl leading-tight font-black tracking-tight md:text-[32px]'>
            {t('Why should your API choose us?')}
          </h2>
        </AnimateInView>

        <div className='grid gap-5 md:grid-cols-3'>
          {REASONS.map((reason, index) => {
            const Icon = reason.icon

            return (
              <AnimateInView
                key={reason.title}
                delay={index * 100}
                animation='scale-in'
                className='border-border/70 bg-background rounded-2xl border p-6 shadow-[0_24px_90px_-62px_rgb(15_23_42/0.55)] transition-colors hover:border-red-200 hover:bg-red-50/30 dark:hover:border-red-500/25 dark:hover:bg-red-500/5'
              >
                <div className='mb-5 flex items-center gap-3'>
                  <div className='flex size-8 shrink-0 items-center justify-center rounded-full bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-300'>
                    <Icon className='size-3.5' strokeWidth={2} />
                  </div>
                  <h3 className='min-w-0 text-lg leading-snug font-black tracking-tight text-red-700 dark:text-red-300'>
                    {t(reason.title)}
                  </h3>
                </div>
                <p className='text-muted-foreground text-sm leading-6'>
                  {t(reason.description)}
                </p>
              </AnimateInView>
            )
          })}
        </div>
      </div>
    </section>
  )
}
