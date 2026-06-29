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
import { Activity } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'
import { cn } from '@/lib/utils'

const DEFAULT_IFRAME_HEIGHT = 860
const MIN_IFRAME_HEIGHT = 720
const MAX_IFRAME_HEIGHT = 3600

type RadarEmbedMessage = {
  height?: unknown
  source?: unknown
  type?: unknown
}

function getRadarBaseUrl() {
  const configured = import.meta.env.VITE_LLMHUB_RADAR_URL as string | undefined
  if (configured) {
    return configured.replace(/\/$/, '')
  }
  if (import.meta.env.DEV) {
    return 'http://localhost:3001'
  }
  return 'https://llm-hub.store'
}

function clampHeight(height: number) {
  return Math.min(
    Math.max(Math.ceil(height), MIN_IFRAME_HEIGHT),
    MAX_IFRAME_HEIGHT
  )
}

function isRadarEmbedMessage(data: unknown): data is RadarEmbedMessage {
  return (
    typeof data === 'object' &&
    data !== null &&
    (data as RadarEmbedMessage).source === 'llmhub-radar'
  )
}

export function LlmhubRadar() {
  const { t, i18n } = useTranslation()
  const [height, setHeight] = useState(DEFAULT_IFRAME_HEIGHT)
  const [isLoaded, setIsLoaded] = useState(false)
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const radarBaseUrl = getRadarBaseUrl()
  const radarOrigin = useMemo(() => {
    try {
      return new URL(radarBaseUrl).origin
    } catch {
      return null
    }
  }, [radarBaseUrl])
  const locale = i18n.language?.startsWith('zh') ? 'zh' : 'en'
  const embedUrl = `${radarBaseUrl}/x-llm/${locale}?embed=1&theme=light`

  useEffect(() => {
    const handleMessage = (event: MessageEvent) => {
      if (radarOrigin && event.origin !== radarOrigin) {
        return
      }
      if (!isRadarEmbedMessage(event.data)) {
        return
      }
      if (
        (event.data.type === 'llmhub:embed-ready' ||
          event.data.type === 'llmhub:embed-height') &&
        typeof event.data.height === 'number' &&
        Number.isFinite(event.data.height)
      ) {
        setHeight(clampHeight(event.data.height))
      }
      if (event.data.type === 'llmhub:embed-ready') {
        setIsLoaded(true)
      }
    }

    window.addEventListener('message', handleMessage)
    return () => window.removeEventListener('message', handleMessage)
  }, [radarOrigin])

  return (
    <section className='dark:bg-background relative z-10 bg-[#f7f7f7] px-6 py-16 md:py-20'>
      <div className='mx-auto max-w-7xl'>
        <AnimateInView>
          <div className='mb-8 max-w-3xl'>
            <div className='max-w-3xl'>
              <div className='mb-4 inline-flex items-center gap-2 rounded-full border border-red-200 bg-red-50 px-3 py-1 text-xs font-semibold text-red-700 dark:border-red-500/25 dark:bg-red-500/10 dark:text-red-300'>
                <Activity className='size-3.5' />
                {t('Observed by LLMHub Radar')}
              </div>
              <h2 className='text-foreground text-3xl leading-tight font-black tracking-tight md:text-4xl'>
                {t('Real service status before you choose a route')}
              </h2>
              <p className='text-muted-foreground mt-3 max-w-2xl text-sm leading-relaxed md:text-base'>
                {t(
                  'Availability blocks and API key detail cards are embedded from the public LLMHub Radar status page, using the same probe data users see on the full status page.'
                )}
              </p>
            </div>
          </div>
        </AnimateInView>

        <AnimateInView animation='scale-in'>
          <div className='border-border/70 bg-background relative overflow-hidden rounded-2xl border p-3 shadow-[0_24px_90px_-62px_rgb(15_23_42/0.55)] md:p-5'>
            {!isLoaded ? (
              <div className='pointer-events-none absolute inset-x-0 top-0 z-10 h-1 overflow-hidden bg-red-50'>
                <div className='h-full w-1/3 animate-pulse bg-red-500' />
              </div>
            ) : null}
            <iframe
              ref={iframeRef}
              src={embedUrl}
              title={t('LLMHub Radar x-llm service status')}
              className={cn(
                'block w-full rounded-xl border-0 bg-white transition-[height] duration-300',
                !isLoaded && 'opacity-95'
              )}
              style={{ height }}
              loading='eager'
              onLoad={() => setIsLoaded(true)}
            />
          </div>
        </AnimateInView>
      </div>
    </section>
  )
}
