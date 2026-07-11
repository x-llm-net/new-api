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
import { ShieldCheck } from 'lucide-react'
import { useEffect, useRef, type CSSProperties } from 'react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { getXLLMGroupStabilitySummary } from '@/features/xllm-group-stability/api'
import type {
  XLLMGroupStabilityBucket,
  XLLMGroupStabilityItem,
} from '@/features/xllm-group-stability/types'
import { cn } from '@/lib/utils'

type XLLMStabilityTone =
  | 'healthy'
  | 'minor'
  | 'elevated'
  | 'major'
  | 'unavailable'
  | 'unknown'

const BUCKET_COLOR_CLASSES: Record<XLLMStabilityTone, string> = {
  elevated: 'bg-yellow-300 hover:bg-yellow-300',
  healthy: 'bg-emerald-500 hover:bg-emerald-500',
  major: 'bg-orange-400 hover:bg-orange-400',
  minor: 'bg-emerald-300 hover:bg-emerald-300',
  unavailable: 'bg-red-500 hover:bg-red-500',
  unknown:
    'bg-zinc-200 hover:bg-zinc-300 dark:bg-zinc-800 dark:hover:bg-zinc-700',
}

const STATUS_DOT_CLASSES: Record<XLLMStabilityTone, string> = {
  elevated: 'bg-yellow-300',
  healthy: 'bg-emerald-500',
  major: 'bg-orange-400',
  minor: 'bg-emerald-300',
  unavailable: 'bg-red-500',
  unknown: 'bg-zinc-300 dark:bg-zinc-700',
}

function formatTime(timestamp: number) {
  if (!timestamp) {
    return '--'
  }
  return new Intl.DateTimeFormat(undefined, {
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    month: '2-digit',
  }).format(timestamp * 1000)
}

function formatHourWindow(timestamp: number) {
  if (!timestamp) {
    return '--'
  }
  const start = new Date(timestamp * 1000)
  const end = new Date((timestamp + 3599) * 1000)
  const date = new Intl.DateTimeFormat(undefined, {
    day: '2-digit',
    month: '2-digit',
  }).format(start)
  const startTime = new Intl.DateTimeFormat(undefined, {
    hour: '2-digit',
    minute: '2-digit',
  }).format(start)
  const endTime = new Intl.DateTimeFormat(undefined, {
    hour: '2-digit',
    minute: '2-digit',
  }).format(end)
  return `${date} ${startTime}-${endTime}`
}

function formatPercent(value: number) {
  return Number.isInteger(value) ? String(value) : value.toFixed(2)
}

function bucketTone(bucket: XLLMGroupStabilityBucket): XLLMStabilityTone {
  if (bucket.total_runs <= 0) {
    return 'unknown'
  }
  const failedRuns = bucket.total_runs - bucket.available_runs
  if (failedRuns <= 0) {
    return 'healthy'
  }
  if (bucket.total_runs < 3) {
    return 'minor'
  }
  if (failedRuns >= bucket.total_runs) {
    return 'unavailable'
  }
  if (failedRuns === 1) {
    return 'minor'
  }
  if (failedRuns === 2) {
    return 'elevated'
  }
  return 'major'
}

function toneLabel(tone: XLLMStabilityTone) {
  const labels: Record<XLLMStabilityTone, string> = {
    elevated: 'Elevated errors',
    healthy: 'Operational',
    major: 'Major fluctuation',
    minor: 'Minor fluctuation',
    unavailable: 'Unavailable',
    unknown: 'No data',
  }
  return labels[tone]
}

function latestKnownTone(item: XLLMGroupStabilityItem): XLLMStabilityTone {
  for (let index = item.seven_day_buckets.length - 1; index >= 0; index--) {
    const bucket = item.seven_day_buckets[index]
    if (bucket.total_runs > 0) {
      return bucketTone(bucket)
    }
  }
  return 'unknown'
}

function BucketTooltipContent(props: { bucket: XLLMGroupStabilityBucket }) {
  const { t } = useTranslation()
  const bucket = props.bucket
  const tone = bucketTone(bucket)

  return (
    <div className='min-w-36 space-y-1.5 text-left'>
      <div className='text-xs font-semibold whitespace-nowrap'>
        {formatHourWindow(bucket.ts)}
      </div>
      <div className='text-[11px] opacity-90'>
        {t('Status')}: {t(toneLabel(tone))}
      </div>
      {bucket.total_runs > 0 ? (
        <div className='text-[11px] opacity-90'>
          {t('Available samples')}: {bucket.available_runs}/{bucket.total_runs}
        </div>
      ) : null}
    </div>
  )
}

function StabilityBlocks(props: { buckets: XLLMGroupStabilityBucket[] }) {
  const scrollerRef = useRef<HTMLDivElement>(null)
  const autoScrolledRef = useRef(false)
  const gridStyle = {
    '--xllm-stability-cols': props.buckets.length,
    '--xllm-stability-mobile-cell': '3px',
  } as CSSProperties

  useEffect(() => {
    const scroller = scrollerRef.current
    if (!scroller || autoScrolledRef.current) {
      return
    }

    if (!window.matchMedia('(max-width: 639px)').matches) {
      return
    }

    requestAnimationFrame(() => {
      scroller.scrollLeft = scroller.scrollWidth
      autoScrolledRef.current = true
    })
  }, [props.buckets.length])

  return (
    <div
      ref={scrollerRef}
      className='w-full max-w-full [scrollbar-width:none] overflow-x-auto overflow-y-hidden sm:overflow-visible [&::-webkit-scrollbar]:hidden'
    >
      <div
        className='grid w-max grid-cols-[repeat(var(--xllm-stability-cols),var(--xllm-stability-mobile-cell))] gap-px sm:w-full sm:grid-cols-[repeat(var(--xllm-stability-cols),minmax(0,1fr))] sm:gap-[2px]'
        style={gridStyle}
      >
        {props.buckets.map((bucket, index) => {
          const trigger = (
            <button
              type='button'
              aria-label={`status block ${index + 1}`}
              className={cn(
                'h-5 min-w-0 rounded-[3px] transition-transform duration-150 hover:scale-y-125 hover:ring-2 hover:ring-foreground/20 focus-visible:ring-2 focus-visible:ring-foreground/30 focus-visible:outline-none',
                BUCKET_COLOR_CLASSES[bucketTone(bucket)]
              )}
            />
          )

          return (
            <Tooltip key={`${bucket.ts}-${index}`}>
              <TooltipTrigger render={trigger} />
              <TooltipContent
                side='top'
                sideOffset={8}
                className='bg-foreground text-background max-w-48 items-start px-3 py-2'
              >
                <BucketTooltipContent bucket={bucket} />
              </TooltipContent>
            </Tooltip>
          )
        })}
      </div>
    </div>
  )
}

function StabilityLegend() {
  const { t } = useTranslation()
  const scaleClasses = [
    'bg-emerald-500',
    'bg-emerald-300',
    'bg-yellow-300',
    'bg-orange-400',
    'bg-red-500',
  ]

  return (
    <div className='flex items-center gap-2 whitespace-nowrap'>
      <span>{t('Operational')}</span>
      <span
        aria-hidden='true'
        className='border-border/70 flex h-3 w-24 overflow-hidden rounded-full border'
      >
        {scaleClasses.map((className, index) => (
          <span
            key={`${className}-${index}`}
            className={cn('h-full flex-1', className)}
          />
        ))}
      </span>
      <span>{t('Unavailable')}</span>
    </div>
  )
}

function GroupStatusRow(props: { item: XLLMGroupStabilityItem }) {
  const { t } = useTranslation()
  const item = props.item
  const tone = latestKnownTone(item)
  const availabilityRate = formatPercent(item.seven_day_success_rate)

  return (
    <div className='space-y-3 px-5 py-4'>
      <div className='flex min-w-0 flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
        <div className='flex min-w-0 items-center gap-3'>
          <span
            className={cn(
              'size-3 shrink-0 rounded-full',
              STATUS_DOT_CLASSES[tone]
            )}
          />
          <div className='text-foreground min-w-0 truncate text-sm font-bold'>
            {item.display_name}
          </div>
        </div>
        <div className='text-muted-foreground flex flex-wrap items-center gap-x-2 gap-y-1 pl-6 text-xs sm:justify-end sm:pl-0'>
          <span className='text-foreground font-semibold'>
            {t('{{value}}% availability', { value: availabilityRate })}
          </span>
          <span className='text-border hidden sm:inline'>/</span>
          <span>
            {t('Updated')} {formatTime(item.last_tested_at)}
          </span>
        </div>
      </div>
      <StabilityBlocks buckets={item.seven_day_buckets} />
    </div>
  )
}

export function GroupStability() {
  const { t } = useTranslation()
  const summaryQuery = useQuery({
    queryKey: ['xllm-group-stability-summary'],
    queryFn: getXLLMGroupStabilitySummary,
    refetchInterval: 60_000,
  })
  const summary = summaryQuery.data?.data

  if (!summary?.enabled || summary.items.length === 0) {
    return null
  }

  return (
    <section
      id='service-status'
      className='dark:bg-background relative z-10 bg-[#f7f7f7] px-6 py-16 md:py-20'
    >
      <div className='mx-auto max-w-7xl'>
        <AnimateInView>
          <div className='mb-8 max-w-3xl'>
            <div className='max-w-3xl'>
              <div className='mb-4 inline-flex items-center gap-2 rounded-full border border-red-200 bg-red-50 px-3 py-1 text-xs font-semibold text-red-700 dark:border-red-500/25 dark:bg-red-500/10 dark:text-red-300'>
                <ShieldCheck className='size-3.5' />
                {t('Service status')}
              </div>
              <h2 className='text-foreground text-3xl leading-tight font-black tracking-tight md:text-4xl'>
                {t(
                  'Low cost is not enough. Stability is what makes it usable long term.'
                )}
              </h2>
              <p className='text-muted-foreground mt-3 max-w-2xl text-sm leading-relaxed md:text-base'>
                {t(
                  'We publish availability for key service groups, so you can judge whether the service is reliable enough before long-running work.'
                )}
              </p>
            </div>
          </div>
        </AnimateInView>

        <TooltipProvider delay={80}>
          <div className='border-border/70 bg-background divide-border/70 rounded-xl border shadow-[0_18px_70px_-58px_rgb(15_23_42/0.55)]'>
            <div className='text-muted-foreground flex flex-col gap-2 px-5 pt-3 pb-2 text-xs font-semibold sm:flex-row sm:items-center sm:justify-between'>
              <span>{t('Service group')}</span>
              <div className='flex flex-wrap items-center gap-x-4 gap-y-1 sm:justify-end'>
                <StabilityLegend />
                <span className='text-border hidden sm:inline'>/</span>
                <span>{t('7-day availability')}</span>
              </div>
            </div>
            {summary.items.map((item, index) => (
              <AnimateInView
                key={item.group_name}
                delay={index * 80}
                animation='fade-up'
              >
                <div className='border-border/70 border-t'>
                  <GroupStatusRow item={item} />
                </div>
              </AnimateInView>
            ))}
          </div>
        </TooltipProvider>
      </div>
    </section>
  )
}
