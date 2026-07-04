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
import type { PointerEvent } from 'react'
import { useState } from 'react'
import { createPortal } from 'react-dom'
import { useQuery } from '@tanstack/react-query'
import { AxiosError } from 'axios'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'
import { Skeleton } from '@/components/ui/skeleton'
import { getXLLMHomeMonitorSummary } from '@/features/xllm-home-monitor/api'
import type {
  XLLMHomeMonitorBucket,
  XLLMHomeMonitorItem,
  XLLMHomeMonitorStatus,
} from '@/features/xllm-home-monitor/types'
import { formatTimestampRelative } from '@/lib/format'
import { cn } from '@/lib/utils'

const SKELETON_ROWS = ['xllm-status-row-1', 'xllm-status-row-2'] as const

type TTFTLevel =
  | 'fastest'
  | 'fast'
  | 'normal'
  | 'slow'
  | 'verySlow'
  | 'extremelySlow'
  | 'failed'
  | 'unknown'

type TTFTTone = {
  className: string
  label: string
  levelLabel: string
}

type TimelineBucket = XLLMHomeMonitorBucket

type TimelineTooltipState = {
  bucket: TimelineBucket
  bucketSeconds: number
  x: number
  y: number
}

const EXPECTED_RECENT_BUCKETS = 60
const EXPECTED_SEVEN_DAY_BUCKETS = 168
const DEFAULT_RECENT_BUCKET_SECONDS = 60
const SEVEN_DAY_BUCKET_SECONDS = 3600
const TOOLTIP_WIDTH = 212
const TOOLTIP_HEIGHT = 142

const TTFT_TONE: Record<TTFTLevel, TTFTTone> = {
  fastest: {
    label: 'TTFT under 3s',
    levelLabel: 'Probe speed: fastest',
    className: 'bg-emerald-500',
  },
  fast: {
    label: 'TTFT under 8s',
    levelLabel: 'Probe speed: fast',
    className: 'bg-emerald-400',
  },
  normal: {
    label: 'TTFT under 15s',
    levelLabel: 'Probe speed: normal',
    className: 'bg-lime-400',
  },
  slow: {
    label: 'TTFT under 30s',
    levelLabel: 'Probe speed: slow',
    className: 'bg-yellow-300',
  },
  verySlow: {
    label: 'TTFT under 50s',
    levelLabel: 'Probe speed: very slow',
    className: 'bg-yellow-500',
  },
  extremelySlow: {
    label: 'TTFT over 50s',
    levelLabel: 'Probe speed: extremely slow',
    className: 'bg-yellow-600',
  },
  failed: {
    label: 'Probe failed',
    levelLabel: 'Probe result: failed',
    className: 'bg-red-500',
  },
  unknown: {
    label: 'No probe data',
    levelLabel: 'Probe result: no data',
    className: 'bg-slate-300 dark:bg-slate-700',
  },
}

const STATUS_TONE: Record<
  XLLMHomeMonitorStatus,
  { dotClassName: string; label: string; textClassName: string }
> = {
  operational: {
    label: 'Operational',
    dotClassName: 'bg-emerald-500',
    textClassName: 'text-emerald-600 dark:text-emerald-400',
  },
  degraded: {
    label: 'Degraded',
    dotClassName: 'bg-amber-500',
    textClassName: 'text-amber-600 dark:text-amber-400',
  },
  unavailable: {
    label: 'Unavailable',
    dotClassName: 'bg-red-500',
    textClassName: 'text-red-600 dark:text-red-400',
  },
  unknown: {
    label: 'Waiting',
    dotClassName: 'bg-slate-400',
    textClassName: 'text-muted-foreground',
  },
}

function ttftLevel(ms: number, hasData: boolean): TTFTLevel {
  if (!hasData) return 'unknown'
  if (ms <= 0 || !Number.isFinite(ms)) return 'unknown'
  if (ms < 3_000) return 'fastest'
  if (ms < 8_000) return 'fast'
  if (ms < 15_000) return 'normal'
  if (ms < 30_000) return 'slow'
  if (ms <= 50_000) return 'verySlow'
  return 'extremelySlow'
}

function bucketLevel(
  bucket: XLLMHomeMonitorBucket,
  bucketSeconds: number
): TTFTLevel {
  if (bucket.sample_count > 0 && bucket.success_count <= 0) return 'failed'
  return ttftLevel(
    bucketDisplayTTFTMs(bucket, bucketSeconds),
    bucket.ttft_count > 0
  )
}

function bucketDisplayTTFTMs(
  bucket: XLLMHomeMonitorBucket,
  bucketSeconds: number
): number {
  if (bucketSeconds === SEVEN_DAY_BUCKET_SECONDS) {
    return bucket.p50_ttft_ms || bucket.avg_ttft_ms
  }
  return bucket.avg_ttft_ms
}

function formatTTFT(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return '-'
  if (ms >= 1_000) return `${(ms / 1_000).toFixed(2)}s`
  return `${Math.round(ms)}ms`
}

function formatPercent(value: number): string {
  if (!Number.isFinite(value)) return '-'
  return `${value.toFixed(2)}%`
}

function formatLastProbe(timestamp: number, locale?: string): string {
  return formatTimestampRelative(timestamp, 'seconds', locale)
}

function formatBucketTimeRange(
  timestamp: number,
  bucketSeconds: number,
  locale?: string
): string {
  if (!Number.isFinite(timestamp) || timestamp <= 0) return '-'

  const dateFormatter = new Intl.DateTimeFormat(locale, {
    month: '2-digit',
    day: '2-digit',
  })
  const calendarFormatter = new Intl.DateTimeFormat(locale, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  })
  const timeFormatter = new Intl.DateTimeFormat(locale, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
  const clockFormatter = new Intl.DateTimeFormat(locale, {
    hour: '2-digit',
    minute: '2-digit',
  })
  const startDate = new Date(timestamp * 1000)
  const endDate = new Date((timestamp + bucketSeconds) * 1000)
  const startDay = calendarFormatter.format(startDate)
  const endDay = calendarFormatter.format(endDate)
  if (startDay === endDay) {
    return `${dateFormatter.format(startDate)} ${clockFormatter.format(startDate)}-${clockFormatter.format(endDate)}`
  }
  return `${timeFormatter.format(startDate)}-${timeFormatter.format(endDate)}`
}

function timelineBucketSeconds(
  windowSeconds: number | undefined,
  expectedCount: number,
  fallbackSeconds: number
): number {
  if (!Number.isFinite(windowSeconds) || !windowSeconds || windowSeconds <= 0) {
    return fallbackSeconds
  }
  return Math.max(fallbackSeconds, Math.round(windowSeconds / expectedCount))
}

function formatWindowLabel(
  windowSeconds: number | undefined,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  if (!Number.isFinite(windowSeconds) || !windowSeconds || windowSeconds <= 0) {
    return t('60 minutes')
  }
  const minutes = Math.max(1, Math.round(windowSeconds / 60))
  if (minutes % (24 * 60) === 0) {
    return t('{{count}} days', { count: minutes / (24 * 60) })
  }
  if (minutes % 60 === 0) {
    return t('{{count}} hours', { count: minutes / 60 })
  }
  return t('{{count}} minutes', { count: minutes })
}

function normalizeBuckets(
  buckets: XLLMHomeMonitorBucket[],
  expectedCount: number,
  fallbackBucketSeconds: number
): TimelineBucket[] {
  const normalizedBuckets =
    buckets.length >= expectedCount ? buckets.slice(-expectedCount) : buckets
  if (buckets.length === 0) return []

  const firstBucket = normalizedBuckets[0]
  const interval =
    normalizedBuckets.length > 1
      ? Math.max(1, normalizedBuckets[1].ts - firstBucket.ts)
      : fallbackBucketSeconds
  const missingCount = expectedCount - normalizedBuckets.length
  const paddedBuckets = Array.from({ length: missingCount }, (_, index) => ({
    ts: firstBucket.ts - interval * (missingCount - index),
    status: 'unknown' as XLLMHomeMonitorStatus,
    success_rate: 0,
    avg_ttft_ms: 0,
    p50_ttft_ms: 0,
    sample_count: 0,
    success_count: 0,
    failure_count: 0,
    ttft_count: 0,
    last_tested_at: 0,
  }))

  return [...paddedBuckets, ...normalizedBuckets]
}

function monitorErrorDetails(error: unknown): string {
  if (error instanceof AxiosError) {
    const status = error.response?.status
    const data = error.response?.data as { message?: string } | undefined
    const parts = [
      status ? `HTTP ${status}` : 'Network error',
      error.config?.method?.toUpperCase() ?? 'GET',
      error.config?.url ?? '/api/xllm/home-monitor/summary',
      data?.message || error.message,
    ]
    return parts.filter(Boolean).join(' - ')
  }
  if (error instanceof Error) return error.message
  return 'Unknown monitor request error'
}

export function ServiceStatus() {
  const { t, i18n } = useTranslation()
  const [timelineTooltip, setTimelineTooltip] =
    useState<TimelineTooltipState | null>(null)
  const monitorQuery = useQuery({
    queryKey: ['xllm-home-monitor-summary'],
    queryFn: getXLLMHomeMonitorSummary,
    staleTime: 30 * 1000,
    refetchInterval: 60 * 1000,
    retry: false,
  })

  const summary = monitorQuery.data?.data
  const recentWindowSeconds =
    summary?.recent_window_secs ??
    EXPECTED_RECENT_BUCKETS * DEFAULT_RECENT_BUCKET_SECONDS
  const recentBucketSeconds = timelineBucketSeconds(
    recentWindowSeconds,
    EXPECTED_RECENT_BUCKETS,
    DEFAULT_RECENT_BUCKET_SECONDS
  )
  const recentWindowLabel = formatWindowLabel(recentWindowSeconds, t)
  const overallTone = STATUS_TONE[summary?.overall_status ?? 'unknown']
  const hasMonitorError = !monitorQuery.isLoading && monitorQuery.isError
  const realtimeClock = new Intl.DateTimeFormat(i18n.language, {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date())

  return (
    <section id='service-status' className='relative z-10 px-6 py-16 md:py-20'>
      <div className='mx-auto max-w-7xl'>
        <AnimateInView>
          <div className='mb-5 flex flex-col gap-4 md:flex-row md:items-start md:justify-between'>
            <div>
              <h2 className='text-2xl leading-tight font-black tracking-tight md:text-[32px]'>
                {t('Cheap is not the only answer. Stability is built for long-term use.')}
              </h2>
              <TTFTLegend />
            </div>
            <div className='border-border/70 bg-background flex items-center gap-2 rounded-lg border px-3 py-2 font-mono text-xs shadow-sm'>
              <span
                className={cn('size-2.5 rounded-full', overallTone.dotClassName)}
                aria-hidden='true'
              />
              <span>{t(overallTone.label)}</span>
              <span>{realtimeClock}</span>
            </div>
          </div>
        </AnimateInView>

        <AnimateInView animation='scale-in'>
          <div className='border-border/70 bg-background overflow-hidden rounded-lg border shadow-[0_24px_90px_-62px_rgb(15_23_42/0.55)]'>
            <div className='border-border/70 bg-muted/10 flex flex-wrap items-center gap-x-6 gap-y-2 border-b px-5 py-3'>
              <Metric
                label={t('Monitored groups')}
                value={hasMonitorError ? '-' : String(summary?.total_items ?? 0)}
              />
              <Metric
                label={t('Healthy groups')}
                value={
                  hasMonitorError
                    ? '-'
                    : String(summary?.operational_items ?? 0)
                }
              />
              <Metric
                label={t('Last probe')}
                value={
                  hasMonitorError
                    ? '-'
                    : formatLastProbe(summary?.last_tested_at ?? 0, i18n.language)
                }
              />
            </div>

            {monitorQuery.isLoading ? <LoadingRows /> : null}

            {!monitorQuery.isLoading && monitorQuery.isError ? (
              <div className='text-muted-foreground px-5 py-10 text-center text-sm'>
                <div>{t('Monitor data is temporarily unavailable.')}</div>
                <div className='mt-2 font-mono text-xs'>
                  {monitorErrorDetails(monitorQuery.error)}
                </div>
              </div>
            ) : null}

            {!monitorQuery.isLoading &&
            !monitorQuery.isError &&
            (!summary || summary.items.length === 0) ? (
              <div className='text-muted-foreground px-5 py-10 text-center text-sm'>
                {t('No first token probe data is available yet.')}
              </div>
            ) : null}

            {!monitorQuery.isLoading &&
            !monitorQuery.isError &&
            summary &&
            summary.items.length > 0 ? (
              <div className='divide-border/70 divide-y'>
                {summary.items.map((item) => (
                  <MonitorRow
                    key={`${item.group_name}:${item.model_name}`}
                    item={item}
                    recentBucketSeconds={recentBucketSeconds}
                    recentWindowLabel={recentWindowLabel}
                    onTooltipChange={setTimelineTooltip}
                  />
                ))}
              </div>
            ) : null}
          </div>
        </AnimateInView>
      </div>
      <TimelineTooltip data={timelineTooltip} locale={i18n.language} />
    </section>
  )
}

function Metric(props: { label: string; value: string }) {
  return (
    <div className='flex items-baseline gap-2 whitespace-nowrap'>
      <span className='text-muted-foreground text-xs'>{props.label}</span>
      <span className='text-foreground font-mono text-sm font-bold'>
        {props.value}
      </span>
    </div>
  )
}

function TTFTLegend() {
  const { t } = useTranslation()
  return (
    <div className='text-muted-foreground mt-4 flex flex-wrap items-center gap-x-3 gap-y-2 text-xs md:text-sm'>
      <span className='text-foreground font-medium'>{t('First token latency')}</span>
      <div
        className='h-2.5 w-52 rounded-full bg-[linear-gradient(90deg,#10b981_0%,#34d399_20%,#a3e635_40%,#fde047_62%,#eab308_82%,#ca8a04_100%)]'
        aria-hidden='true'
      />
      <span className='font-mono text-[11px]'>&lt;3s / 8s / 15s / 30s / 50s+</span>
      <span className='flex items-center gap-1.5'>
        <span
          className={cn('h-2.5 w-5 rounded-sm', TTFT_TONE.failed.className)}
          aria-hidden='true'
        />
        <span>{t('Failed')}</span>
      </span>
    </div>
  )
}

function LoadingRows() {
  return (
    <div className='space-y-4 p-5'>
      {SKELETON_ROWS.map((row) => (
        <div key={row} className='space-y-3'>
          <Skeleton className='h-7 w-64' />
          <Skeleton className='h-7 w-full' />
          <Skeleton className='h-5 w-full' />
        </div>
      ))}
    </div>
  )
}

function MonitorRow(props: {
  item: XLLMHomeMonitorItem
  recentBucketSeconds: number
  recentWindowLabel: string
  onTooltipChange: (state: TimelineTooltipState | null) => void
}) {
  const { t, i18n } = useTranslation()
  const tone = STATUS_TONE[props.item.status]
  const currentLevel = ttftLevel(
    props.item.latest_ttft_ms ||
      props.item.p50_ttft_ms_60m ||
      props.item.avg_ttft_ms_60m,
    props.item.has_reliable_ttft
  )

  return (
    <div className='px-5 py-5'>
      <div className='mb-4 flex flex-col gap-3 md:flex-row md:items-start md:justify-between'>
        <div className='min-w-0'>
          <div className='flex min-w-0 flex-wrap items-center gap-2'>
            <h3 className='text-foreground min-w-0 truncate text-xl font-black'>
              {props.item.display_name || props.item.group_name}
            </h3>
            <span className='text-muted-foreground max-w-full truncate font-mono text-xs'>
              {props.item.model_name}
            </span>
          </div>
          <div className='text-muted-foreground mt-2 flex flex-wrap items-center gap-3 text-xs'>
            <span>
              {t('{{window}} success rate', {
                window: props.recentWindowLabel,
              })}
              : {formatPercent(props.item.success_rate_60m)}
            </span>
            <span>{t('7d success')}: {formatPercent(props.item.success_rate_7d)}</span>
            <span>
              {t('Last probe')}:{' '}
              {formatLastProbe(props.item.last_tested_at, i18n.language)}
            </span>
          </div>
        </div>
        <div className='flex items-center gap-2 font-mono'>
          <span className={cn('text-lg font-black', tone.textClassName)}>
            {formatTTFT(props.item.latest_ttft_ms || props.item.avg_ttft_ms_60m)}
          </span>
          <span
            className={cn('size-4 rounded-sm', TTFT_TONE[currentLevel].className)}
            aria-hidden='true'
          />
        </div>
      </div>

      <TimelineRow
        bucketSeconds={props.recentBucketSeconds}
        label={props.recentWindowLabel}
        buckets={normalizeBuckets(
          props.item.recent_buckets,
          EXPECTED_RECENT_BUCKETS,
          props.recentBucketSeconds
        )}
        onTooltipChange={props.onTooltipChange}
      />

      <TimelineRow
        bucketSeconds={SEVEN_DAY_BUCKET_SECONDS}
        label={t('7 days')}
        buckets={normalizeBuckets(
          props.item.seven_day_buckets,
          EXPECTED_SEVEN_DAY_BUCKETS,
          SEVEN_DAY_BUCKET_SECONDS
        )}
        onTooltipChange={props.onTooltipChange}
      />
    </div>
  )
}

function TimelineRow(props: {
  bucketSeconds: number
  buckets: TimelineBucket[]
  label: string
  onTooltipChange: (state: TimelineTooltipState | null) => void
}) {
  const { t } = useTranslation()

  function handlePointerMove(event: PointerEvent<HTMLDivElement>) {
    const target = event.target
    if (!(target instanceof HTMLElement)) return

    const block = target.closest<HTMLElement>('[data-bucket-index]')
    if (!block || !event.currentTarget.contains(block)) return

    const bucketIndex = Number(block.dataset.bucketIndex)
    const bucket = props.buckets[bucketIndex]
    if (!bucket) return

    props.onTooltipChange({
      bucket,
      bucketSeconds: props.bucketSeconds,
      x: event.clientX,
      y: event.clientY,
    })
  }

  return (
    <div className='grid gap-2 py-1.5 md:grid-cols-[72px_minmax(0,1fr)] md:items-center'>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div>
        <div
          className='grid min-h-5 w-full gap-px'
          onPointerLeave={() => props.onTooltipChange(null)}
          onPointerMove={handlePointerMove}
          style={{
            gridTemplateColumns: `repeat(${Math.max(props.buckets.length, 1)}, minmax(0, 1fr))`,
          }}
        >
          {props.buckets.length > 0 ? (
            props.buckets.map((bucket, bucketIndex) => {
              const level = bucketLevel(bucket, props.bucketSeconds)
              return (
                <span
                  key={bucket.ts}
                  data-bucket-index={bucketIndex}
                  className={cn(
                    'relative h-5 min-w-0 transform-gpu rounded-[3px] transition-transform duration-100 ease-out hover:z-10 hover:scale-x-[1.35] hover:scale-y-125 hover:ring-2 hover:ring-background hover:brightness-105 hover:shadow-sm motion-reduce:hover:scale-100',
                    TTFT_TONE[level].className
                  )}
                  aria-hidden='true'
                />
              )
            })
          ) : (
            <div className='bg-muted text-muted-foreground flex h-5 w-full items-center rounded px-3 text-xs'>
              {t('No first token probe data is available yet.')}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function TimelineTooltip(props: {
  data: TimelineTooltipState | null
  locale?: string
}) {
  const { t } = useTranslation()
  if (!props.data || typeof document === 'undefined') return null

  const viewportWidth =
    typeof window === 'undefined' ? TOOLTIP_WIDTH : window.innerWidth
  const viewportHeight =
    typeof window === 'undefined' ? TOOLTIP_HEIGHT : window.innerHeight
  const left = Math.min(
    Math.max(12, props.data.x + 14),
    Math.max(12, viewportWidth - TOOLTIP_WIDTH - 12)
  )
  const preferredTop =
    props.data.y > viewportHeight - TOOLTIP_HEIGHT - 24
      ? props.data.y - TOOLTIP_HEIGHT - 14
      : props.data.y + 14
  const top = Math.min(
    Math.max(12, preferredTop),
    Math.max(12, viewportHeight - TOOLTIP_HEIGHT - 12)
  )
  const level = bucketLevel(props.data.bucket, props.data.bucketSeconds)
  const tone = TTFT_TONE[level]
  const isSevenDay = props.data.bucketSeconds === SEVEN_DAY_BUCKET_SECONDS
  const isFailed = level === 'failed'
  const sampleText =
    props.data.bucket.sample_count > 0
      ? `${t('{{count}} samples', { count: props.data.bucket.sample_count })} / ${t('Success')}: ${props.data.bucket.success_count} / ${t('Failed')}: ${props.data.bucket.failure_count}`
      : t('No samples in this bucket')

  return createPortal(
    <div
      className='bg-popover text-popover-foreground border-border/70 pointer-events-none fixed z-50 rounded-lg border px-2.5 py-2 text-xs shadow-xl'
      data-xllm-timeline-tooltip='true'
      role='tooltip'
      style={{ left, top, width: TOOLTIP_WIDTH }}
    >
      <div className='space-y-1.5'>
        <TooltipRow
          label={t('Time')}
          value={formatBucketTimeRange(
            props.data.bucket.ts,
            props.data.bucketSeconds,
            props.locale
          )}
        />
        <TooltipRow
          label={t('Status')}
          value={
            <span className='inline-flex items-center justify-end gap-1.5'>
              <span
                className={cn('size-2.5 rounded-sm', tone.className)}
                aria-hidden='true'
              />
              <span>{t(tone.levelLabel)}</span>
            </span>
          }
        />
        <TooltipRow
          emphasis
          label={isSevenDay ? t('First token P50') : t('First token')}
          value={
            props.data.bucket.ttft_count > 0
              ? formatTTFT(
                  bucketDisplayTTFTMs(
                    props.data.bucket,
                    props.data.bucketSeconds
                  )
                )
              : '-'
          }
        />
        {isSevenDay || isFailed ? (
          <TooltipRow
            label={t('Success rate')}
            value={
              props.data.bucket.sample_count > 0
                ? formatPercent(props.data.bucket.success_rate)
                : '-'
            }
          />
        ) : null}

        {isSevenDay || isFailed ? (
          <TooltipRow label={t('Samples')} value={sampleText} />
        ) : null}
      </div>
    </div>,
    document.body
  )
}

function TooltipRow(props: {
  emphasis?: boolean
  label: string
  value: string | React.ReactNode
}) {
  return (
    <div
      className={cn(
        'flex items-center justify-between gap-2',
        props.emphasis ? 'text-sm' : ''
      )}
    >
      <span className='text-muted-foreground shrink-0'>{props.label}</span>
      <span
        className={cn(
          'text-right font-mono whitespace-nowrap',
          props.emphasis ? 'text-foreground text-base font-black' : ''
        )}
      >
        {props.value}
      </span>
    </div>
  )
}
