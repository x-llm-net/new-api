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
import {
  ArrowRight,
  BookOpen,
  CheckCircle2,
  Code2,
  KeyRound,
  Terminal,
  Workflow,
  Zap,
} from 'lucide-react'
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type PointerEvent,
} from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

interface HeroProps {
  className?: string
  isAuthenticated?: boolean
}

type HeroStyle = CSSProperties & {
  '--hero-grid-col'?: string
  '--hero-grid-row'?: string
  '--hero-hover-opacity'?: string
}

type ConnectTab = {
  id: string
  label: string
  title: string
  description: string
  badge: string
  icon: 'code' | 'terminal' | 'workflow' | 'key'
  docsPath: string
  steps: string[]
}

type ConnectDocLink = {
  label: string
  href: string
  icon?: 'workflow' | 'key'
}

const INSTALL_DOC_FALLBACK =
  'https://scny66s85sz6.feishu.cn/wiki/LCXGwWNa8iXpAukZ0zBcSvdon9d'

type WaveCenter = {
  id: number
  column: number
  row: number
}

const CONNECT_TABS: ConnectTab[] = [
  {
    id: 'codex-desktop',
    label: 'Codex Desktop',
    title: 'Connect x-llm to Codex Desktop',
    description:
      'Create an API key, click CC Switch in the x-llm key menu, and send the key configuration to CC Switch without typing any endpoint.',
    badge: 'One-click setup',
    icon: 'code',
    docsPath: '/docs/install-codex-desktop',
    steps: [
      'Create an x-llm API key and choose a billing group',
      'Click CC Switch from the key menu',
      'Open Codex Desktop and start with the selected model route',
    ],
  },
  {
    id: 'claude-code',
    label: 'Claude Code Terminal',
    title: 'Connect x-llm to Claude Code Terminal',
    description:
      'Create an API key, click CC Switch in the x-llm key menu, and send the key configuration to CC Switch before launching Claude Code.',
    badge: 'Terminal workflow',
    icon: 'terminal',
    docsPath: '/docs/install-claude-code',
    steps: [
      'Create an x-llm API key and choose a billing group',
      'Click CC Switch from the key menu',
      'Open Claude Code Terminal and start with the selected model route',
    ],
  },
  {
    id: 'codex-terminal',
    label: 'Codex Terminal',
    title: 'Connect x-llm to Codex Terminal',
    description:
      'Create an API key, click CC Switch in the x-llm key menu, and send the key configuration to CC Switch before launching Codex Terminal.',
    badge: 'CLI workflow',
    icon: 'terminal',
    docsPath: '/docs/install-codex',
    steps: [
      'Create an x-llm API key and choose a billing group',
      'Click CC Switch from the key menu',
      'Open Codex Terminal and start with the selected model route',
    ],
  },
] as const

const CONNECT_DOC_LINKS: ConnectDocLink[] = [
  {
    label: 'CC-Switch',
    href: '/docs/install-cc-switch',
    icon: 'workflow',
  },
  {
    label: 'OpenClaw',
    href: '/docs/install-openclaw',
  },
  {
    label: 'Hermes',
    href: '/docs/install-hermes',
  },
  {
    label: 'WorkBuddy',
    href: '/docs/install-workbuddy',
  },
  {
    label: 'Other',
    href: '/docs',
  },
]

const ROUTE_BADGES = [
  'Codex Desktop',
  'Claude Code Terminal',
  'Codex Terminal',
  'CC-Switch',
]
const GRID_CELL_SIZE = 48
const GRID_COLUMNS = 60
const GRID_ROWS = 30
const GRID_CELLS = Array.from(
  { length: GRID_COLUMNS * GRID_ROWS },
  (_, index) => {
    const column = index % GRID_COLUMNS
    const row = Math.floor(index / GRID_COLUMNS)
    return {
      id: `${column}-${row}`,
      column,
      row,
    }
  }
)
const WAVE_COOLDOWN_MS = 2050
const WAVE_GRID_DISTANCE = 1
const WAVE_MAX_DISTANCE = 22

function getWaveCells(activeWave: WaveCenter) {
  const minColumn = Math.max(0, activeWave.column - WAVE_MAX_DISTANCE)
  const maxColumn = Math.min(
    GRID_COLUMNS - 1,
    activeWave.column + WAVE_MAX_DISTANCE
  )
  const minRow = Math.max(0, activeWave.row - WAVE_MAX_DISTANCE)
  const maxRow = Math.min(GRID_ROWS - 1, activeWave.row + WAVE_MAX_DISTANCE)
  const cells: Array<{
    color: string
    column: number
    delay: number
    id: string
    opacity: string
    row: number
  }> = []

  for (let row = minRow; row <= maxRow; row += 1) {
    for (let column = minColumn; column <= maxColumn; column += 1) {
      const distance = Math.hypot(
        column - activeWave.column,
        row - activeWave.row
      )
      if (distance > WAVE_MAX_DISTANCE) {
        continue
      }

      const opacity = Math.max(0.018, 0.15 - distance * 0.006)
      const delay = Math.min(distance * 34, 1080)
      const isWarmCenter = distance < 1.35

      cells.push({
        color: isWarmCenter ? 'rgb(185 28 28 / 1)' : 'rgb(15 23 42 / 1)',
        column,
        delay,
        id: `${column}-${row}`,
        opacity: opacity.toFixed(3),
        row,
      })
    }
  }

  return cells
}

function CodexIcon() {
  return (
    <svg
      aria-hidden='true'
      className='size-4 shrink-0'
      fill='currentColor'
      fillRule='evenodd'
      viewBox='0 0 24 24'
      xmlns='http://www.w3.org/2000/svg'
    >
      <path
        clipRule='evenodd'
        d='M8.086.457a6.105 6.105 0 013.046-.415c1.333.153 2.521.72 3.564 1.7a.117.117 0 00.107.029c1.408-.346 2.762-.224 4.061.366l.063.03.154.076c1.357.703 2.33 1.77 2.918 3.198.278.679.418 1.388.421 2.126a5.655 5.655 0 01-.18 1.631.167.167 0 00.04.155 5.982 5.982 0 011.578 2.891c.385 1.901-.01 3.615-1.183 5.14l-.182.22a6.063 6.063 0 01-2.934 1.851.162.162 0 00-.108.102c-.255.736-.511 1.364-.987 1.992-1.199 1.582-2.962 2.462-4.948 2.451-1.583-.008-2.986-.587-4.21-1.736a.145.145 0 00-.14-.032c-.518.167-1.04.191-1.604.185a5.924 5.924 0 01-2.595-.622 6.058 6.058 0 01-2.146-1.781c-.203-.269-.404-.522-.551-.821a7.74 7.74 0 01-.495-1.283 6.11 6.11 0 01-.017-3.064.166.166 0 00.008-.074.115.115 0 00-.037-.064 5.958 5.958 0 01-1.38-2.202 5.196 5.196 0 01-.333-1.589 6.915 6.915 0 01.188-2.132c.45-1.484 1.309-2.648 2.577-3.493.282-.188.55-.334.802-.438.286-.12.573-.22.861-.304a.129.129 0 00.087-.087A6.016 6.016 0 015.635 2.31C6.315 1.464 7.132.846 8.086.457zm-.804 7.85a.848.848 0 00-1.473.842l1.694 2.965-1.688 2.848a.849.849 0 001.46.864l1.94-3.272a.849.849 0 00.007-.854l-1.94-3.393zm5.446 6.24a.849.849 0 000 1.695h4.848a.849.849 0 000-1.696h-4.848z'
      />
    </svg>
  )
}

function getTabIcon(tab: ConnectTab) {
  if (tab.id === 'codex-desktop') {
    return <CodexIcon />
  }
  if (tab.icon === 'terminal') {
    return <Terminal className='size-4' />
  }
  if (tab.icon === 'workflow') {
    return <Workflow className='size-4' />
  }
  if (tab.icon === 'key') {
    return <KeyRound className='size-4' />
  }
  return <Code2 className='size-4' />
}

function getDocLinkIcon(link: ConnectDocLink) {
  if (!link.icon) {
    return null
  }
  if (link.icon === 'key') {
    return <KeyRound className='size-4' />
  }
  return <Workflow className='size-4' />
}

export function Hero(props: HeroProps) {
  const { t } = useTranslation()
  const heroRef = useRef<HTMLDivElement>(null)
  const gridRef = useRef<HTMLDivElement>(null)
  const frameRef = useRef<number | null>(null)
  const latestPointerRef = useRef({ column: 30, row: 6 })
  const lastWaveRef = useRef({ column: 30, row: 6 })
  const waveLockedRef = useRef(false)
  const waveUnlockTimerRef = useRef<number | null>(null)
  const [activeWave, setActiveWave] = useState<WaveCenter>({
    id: 0,
    column: 30,
    row: 6,
  })
  const [activeTab, setActiveTab] = useState(CONNECT_TABS[0].id)
  const waveCells = useMemo(() => getWaveCells(activeWave), [activeWave])
  const activeConnectTab =
    CONNECT_TABS.find((tab) => tab.id === activeTab) || CONNECT_TABS[0]

  useEffect(() => {
    return () => {
      if (frameRef.current !== null) {
        window.cancelAnimationFrame(frameRef.current)
      }
      if (waveUnlockTimerRef.current !== null) {
        window.clearTimeout(waveUnlockTimerRef.current)
      }
    }
  }, [])

  const updatePointerVars = () => {
    frameRef.current = null
    const node = heroRef.current
    if (!node) {
      return
    }
    node.style.setProperty(
      '--hero-grid-col',
      `${latestPointerRef.current.column}`
    )
    node.style.setProperty('--hero-grid-row', `${latestPointerRef.current.row}`)
    node.style.setProperty('--hero-hover-opacity', '1')
  }

  const getGridPosition = (event: PointerEvent<HTMLDivElement>) => {
    const rect =
      gridRef.current?.getBoundingClientRect() ||
      event.currentTarget.getBoundingClientRect()
    const x = event.clientX - rect.left
    const y = event.clientY - rect.top

    return {
      column: Math.max(
        0,
        Math.min(GRID_COLUMNS - 1, Math.floor(x / GRID_CELL_SIZE))
      ),
      row: Math.max(0, Math.min(GRID_ROWS - 1, Math.floor(y / GRID_CELL_SIZE))),
    }
  }

  const pushGridWave = (column: number, row: number, force = false) => {
    const lastWave = lastWaveRef.current
    const movedDistance = Math.hypot(
      column - lastWave.column,
      row - lastWave.row
    )
    const canCreateWave = movedDistance >= WAVE_GRID_DISTANCE

    if (waveLockedRef.current && !force) {
      return
    }

    if (!force && !canCreateWave) {
      return
    }

    waveLockedRef.current = true
    if (waveUnlockTimerRef.current !== null) {
      window.clearTimeout(waveUnlockTimerRef.current)
    }
    lastWaveRef.current = { column, row }
    setActiveWave((currentWave) => ({
      id: currentWave.id + 1,
      column,
      row,
    }))

    waveUnlockTimerRef.current = window.setTimeout(() => {
      waveLockedRef.current = false
      waveUnlockTimerRef.current = null
    }, WAVE_COOLDOWN_MS)
  }

  const handlePointerMove = (event: PointerEvent<HTMLDivElement>) => {
    const nextPointer = getGridPosition(event)
    latestPointerRef.current = nextPointer
    if (frameRef.current === null) {
      frameRef.current = window.requestAnimationFrame(updatePointerVars)
    }
  }

  const handlePointerDown = (event: PointerEvent<HTMLDivElement>) => {
    const nextPointer = getGridPosition(event)
    latestPointerRef.current = nextPointer
    updatePointerVars()
    pushGridWave(nextPointer.column, nextPointer.row, true)
  }

  const handlePointerLeave = () => {
    if (heroRef.current) {
      heroRef.current.style.setProperty('--hero-hover-opacity', '0')
    }
  }

  const getDocsHref = (path = '/docs') => {
    if (path.startsWith('http')) {
      return path
    }
    return INSTALL_DOC_FALLBACK
  }
  const activeDocsHref = getDocsHref(activeConnectTab.docsPath)
  const activeDocsExternal = activeDocsHref.startsWith('http')

  const renderDocsButton = () => {
    return (
      <Button
        variant='outline'
        className='border-border/70 bg-background/85 hover:bg-background h-12 rounded-lg px-6 text-sm font-semibold shadow-sm backdrop-blur'
        render={
          <a
            href={INSTALL_DOC_FALLBACK}
            target='_blank'
            rel='noopener noreferrer'
          />
        }
      >
        <BookOpen className='mr-1.5 size-4' />
        {t('Docs')}
      </Button>
    )
  }

  const heroStyle: HeroStyle = {
    '--hero-grid-col': '30',
    '--hero-grid-row': '6',
    '--hero-hover-opacity': '0',
  }

  return (
    <section
      ref={heroRef}
      className={cn(
        'relative z-10 overflow-hidden px-4 pt-16 pb-20 sm:px-6 md:pt-20 md:pb-24',
        props.className
      )}
      style={heroStyle}
      onPointerMove={handlePointerMove}
      onPointerDown={handlePointerDown}
      onPointerLeave={handlePointerLeave}
    >
      <div
        ref={gridRef}
        aria-hidden
        className='bg-background absolute top-0 left-1/2 -z-20 grid -translate-x-1/2'
        style={{
          gridTemplateColumns: `repeat(${GRID_COLUMNS}, ${GRID_CELL_SIZE}px)`,
          gridTemplateRows: `repeat(${GRID_ROWS}, ${GRID_CELL_SIZE}px)`,
          width: GRID_COLUMNS * GRID_CELL_SIZE,
          height: GRID_ROWS * GRID_CELL_SIZE,
        }}
      >
        {GRID_CELLS.map((cell) => (
          <span
            key={cell.id}
            className='xllm-hero-grid-cell'
            style={
              {
                '--cell-col': cell.column,
                '--cell-row': cell.row,
              } as CSSProperties
            }
          />
        ))}
        <span className='xllm-hero-hover-cell' />
      </div>
      <div
        aria-hidden
        className='pointer-events-none absolute top-0 left-1/2 -z-10 grid -translate-x-1/2'
        style={{
          gridTemplateColumns: `repeat(${GRID_COLUMNS}, ${GRID_CELL_SIZE}px)`,
          gridTemplateRows: `repeat(${GRID_ROWS}, ${GRID_CELL_SIZE}px)`,
          width: GRID_COLUMNS * GRID_CELL_SIZE,
          height: GRID_ROWS * GRID_CELL_SIZE,
        }}
      >
        {waveCells.map((cell) => (
          <span
            key={`${activeWave.id}-${cell.id}`}
            className='xllm-hero-wave-cell'
            style={
              {
                '--cell-delay': `${cell.delay}ms`,
                '--cell-opacity': cell.opacity,
                '--cell-color': cell.color,
                gridColumn: cell.column + 1,
                gridRow: cell.row + 1,
              } as CSSProperties
            }
          />
        ))}
      </div>
      <div
        aria-hidden
        className='from-background via-background/55 absolute inset-x-0 bottom-0 -z-10 h-48 bg-gradient-to-t to-transparent'
      />

      <div className='mx-auto max-w-7xl'>
        <div className='mx-auto flex max-w-5xl flex-col items-center text-center'>
          <div
            className='landing-animate-fade-up border-border/70 bg-background/80 text-muted-foreground mb-7 inline-flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs font-medium opacity-0 shadow-sm backdrop-blur'
            style={{ animationDelay: '0ms' }}
          >
            <Zap className='size-3.5 text-red-500' />
            {t('Model access layer for AI-native teams')}
          </div>

          <h1
            className='landing-animate-fade-up max-w-5xl text-[clamp(2rem,4.6vw,3.75rem)] leading-[1.08] font-black tracking-tight text-balance opacity-0'
            style={{ animationDelay: '60ms' }}
          >
            {t('Built for your Claude Code and Codex')}
            <br />
            <span className='bg-gradient-to-r from-red-700 via-red-500 to-orange-400 bg-clip-text text-transparent'>
              {t('Native LLM Router')}
            </span>
          </h1>

          <p
            className='landing-animate-fade-up text-muted-foreground mt-6 max-w-2xl text-base leading-relaxed opacity-0 md:text-lg'
            style={{ animationDelay: '120ms' }}
          >
            {t(
              'One key connects top global LLMs, with pricing as low as 0.3x official rates.'
            )}
          </p>

          <div
            className='landing-animate-fade-up mt-8 flex flex-wrap items-center justify-center gap-3 opacity-0'
            style={{ animationDelay: '180ms' }}
          >
            {props.isAuthenticated ? (
              <Button
                className='group h-12 rounded-lg bg-red-700 px-7 text-sm font-semibold text-white shadow-lg shadow-red-700/20 hover:bg-red-700'
                render={<Link to='/dashboard' />}
              >
                {t('Go to Dashboard')}
                <ArrowRight className='ml-1.5 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
              </Button>
            ) : (
              <Button
                className='group h-12 rounded-lg bg-red-700 px-7 text-sm font-semibold text-white shadow-lg shadow-red-700/20 hover:bg-red-700'
                render={<Link to='/sign-up' />}
              >
                {t('Get API Key')}
                <ArrowRight className='ml-1.5 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
              </Button>
            )}
            <Button
              variant='outline'
              className='border-border/70 bg-background/85 hover:bg-background h-12 rounded-lg px-6 text-sm font-semibold shadow-sm backdrop-blur'
              render={<Link to='/pricing' />}
            >
              {t('View Pricing')}
            </Button>
            {renderDocsButton()}
          </div>
        </div>

        <div
          className='landing-animate-fade-up mx-auto mt-16 max-w-5xl opacity-0'
          style={{ animationDelay: '260ms' }}
        >
          <div className='bg-background/92 overflow-hidden rounded-xl border border-red-500/20 p-5 shadow-[0_30px_100px_-55px_rgb(15_23_42/0.5)] backdrop-blur md:p-7'>
            <div className='flex gap-2 overflow-x-auto pb-2'>
              {CONNECT_TABS.map((tab) => (
                <button
                  type='button'
                  key={tab.id}
                  className={cn(
                    'inline-flex h-10 shrink-0 items-center gap-2 rounded-full border px-4 text-sm font-semibold transition-colors',
                    activeTab === tab.id
                      ? 'border-red-700 bg-red-700 text-white shadow-sm shadow-red-700/20'
                      : 'border-border bg-muted/20 text-muted-foreground hover:border-border/80 hover:bg-muted/45 hover:text-foreground'
                  )}
                  onClick={() => setActiveTab(tab.id)}
                >
                  {getTabIcon(tab)}
                  {t(tab.label)}
                </button>
              ))}
              {CONNECT_DOC_LINKS.map((link) => {
                const href = getDocsHref(link.href)

                return (
                  <a
                    key={link.label}
                    href={href}
                    target='_blank'
                    rel='noreferrer'
                    className='border-border bg-muted/20 text-muted-foreground hover:border-border/80 hover:bg-muted/45 hover:text-foreground inline-flex h-10 shrink-0 items-center gap-2 rounded-full border px-4 text-sm font-semibold transition-colors'
                  >
                    {getDocLinkIcon(link)}
                    {t(link.label)}
                  </a>
                )
              })}
            </div>

            <div
              className='grid gap-8 pt-5 md:grid-cols-[minmax(0,1fr)_360px] md:items-start'
              data-testid='xllm-connect-content'
            >
              <div className='min-w-0'>
                <div className='mb-4 inline-flex items-center gap-2 rounded-full bg-red-50 px-3 py-1 text-xs font-semibold text-red-700 dark:bg-red-500/10 dark:text-red-300'>
                  <CheckCircle2 className='size-3.5' />
                  {t(activeConnectTab.badge)}
                </div>
                <h2 className='text-2xl leading-tight font-bold tracking-tight md:text-3xl'>
                  {t(activeConnectTab.title)}
                </h2>
                <p className='text-muted-foreground mt-3 max-w-2xl text-sm leading-relaxed md:text-base'>
                  {t(activeConnectTab.description)}
                </p>

                <div className='mt-6 grid gap-3'>
                  {activeConnectTab.steps.map((step, index) => (
                    <div key={step} className='flex items-start gap-3'>
                      <span className='mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full bg-red-700 text-xs font-bold text-white'>
                        {index + 1}
                      </span>
                      <span className='text-foreground/85 text-sm leading-6'>
                        {t(step)}
                      </span>
                    </div>
                  ))}
                </div>

                <div className='mt-7 flex flex-wrap items-center gap-3'>
                  <Button
                    className='h-11 rounded-lg bg-red-700 px-5 text-sm font-semibold text-white hover:bg-red-700'
                    render={<Link to='/sign-up' />}
                  >
                    {t('Create API key')}
                  </Button>
                  <Button
                    variant='outline'
                    className='border-border/70 bg-background/85 hover:bg-background h-11 rounded-lg px-5 text-sm font-semibold shadow-sm backdrop-blur'
                    render={
                      <a
                        href={activeDocsHref}
                        target={activeDocsExternal ? '_blank' : undefined}
                        rel={
                          activeDocsExternal ? 'noopener noreferrer' : undefined
                        }
                      />
                    }
                  >
                    <BookOpen className='mr-1.5 size-4' />
                    {t('View setup guide')}
                  </Button>
                </div>
              </div>

              <div
                className='border-border/70 bg-muted/10 overflow-hidden rounded-lg border md:self-start'
                data-testid='xllm-routing-preview'
              >
                <div className='border-border/70 flex items-center justify-between border-b px-4 py-3'>
                  <span className='text-sm font-semibold'>
                    {t('x-llm routing preview')}
                  </span>
                  <span className='flex items-center gap-1.5 rounded-full bg-emerald-500/10 px-2.5 py-1 text-xs font-semibold text-emerald-700 dark:text-emerald-300'>
                    <span className='size-1.5 rounded-full bg-emerald-500' />
                    {t('Operational')}
                  </span>
                </div>
                <div className='p-4'>
                  <div className='rounded-lg bg-slate-950 p-4 text-xs text-slate-100 shadow-inner'>
                    <div className='text-slate-500'>
                      {t('Recommended path')}
                    </div>
                    <div className='mt-3 space-y-3'>
                      {[
                        'Create API key in x-llm',
                        'Click CC Switch in the key menu',
                        'Open your AI coding tool',
                      ].map((item, index) => (
                        <div key={item} className='flex items-center gap-3'>
                          <span className='flex size-6 shrink-0 items-center justify-center rounded-full bg-red-500 text-[11px] font-bold text-white'>
                            {index + 1}
                          </span>
                          <span className='text-slate-200'>{t(item)}</span>
                        </div>
                      ))}
                    </div>
                    <div className='mt-4 flex flex-wrap gap-1.5 border-t border-white/10 pt-4'>
                      {ROUTE_BADGES.map((route) => (
                        <span
                          key={route}
                          className='rounded border border-white/10 bg-white/8 px-2 py-1 text-[11px] text-slate-200'
                        >
                          {t(route)}
                        </span>
                      ))}
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <div className='border-border/70 text-muted-foreground mt-7 border-t pt-4 text-xs leading-relaxed'>
              {t(
                'Compatible with NewAPI multi-protocol configuration, while making model access easier for the whole team.'
              )}
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
