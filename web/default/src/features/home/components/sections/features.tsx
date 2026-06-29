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
import {
  Activity,
  BrainCircuit,
  Code2,
  DollarSign,
  GitBranch,
  KeyRound,
  LineChart,
  ShieldCheck,
  Users,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'
import { cn } from '@/lib/utils'

interface FeaturesProps {
  className?: string
}

const MODEL_FAMILIES = ['OpenAI', 'Claude', 'Gemini', 'DeepSeek', 'Qwen', 'xAI']
const STATUS_BLOCKS = [
  'bg-emerald-500',
  'bg-emerald-500',
  'bg-lime-400',
  'bg-emerald-500',
  'bg-emerald-500',
  'bg-amber-300',
  'bg-emerald-500',
  'bg-emerald-500',
] as const

export function Features(_props: FeaturesProps) {
  const { t } = useTranslation()

  const features = [
    {
      id: 'routing',
      num: '01',
      title: t('Multi-model routing'),
      desc: t(
        'One compatible API connects mainstream model families for coding agents, chat apps, and automated workflows.'
      ),
      span: 'md:col-span-2',
      icon: <GitBranch className='size-4 text-emerald-500' />,
      visual: (
        <div className='mt-5 grid grid-cols-3 gap-2'>
          {MODEL_FAMILIES.map((name) => (
            <div
              key={name}
              className='border-border/50 bg-muted/20 text-foreground/75 flex items-center justify-center rounded-lg border px-3 py-2 text-xs font-medium'
            >
              {name}
            </div>
          ))}
        </div>
      ),
    },
    {
      id: 'keys',
      num: '02',
      title: t('Keys for every role'),
      desc: t(
        'Developers, operators, and non-technical teammates can use controlled keys without learning every upstream platform.'
      ),
      span: 'md:col-span-1',
      icon: <KeyRound className='size-4 text-blue-500' />,
      visual: (
        <div className='mt-5 space-y-2'>
          {[t('Developer key'), t('Automation key'), t('Team key')].map(
            (label, index) => (
              <div
                key={label}
                className='border-border/50 bg-muted/20 flex items-center justify-between rounded-lg border px-3 py-2'
              >
                <span className='text-xs font-medium'>{label}</span>
                <span className='text-muted-foreground font-mono text-[10px]'>
                  sk-{index + 1}****
                </span>
              </div>
            )
          )}
        </div>
      ),
    },
    {
      id: 'cost',
      num: '03',
      title: t('Cost-aware operations'),
      desc: t(
        'Track usage and balances, compare model cost, and keep experiments from turning into surprise bills.'
      ),
      span: 'md:col-span-1',
      icon: <DollarSign className='size-4 text-amber-500' />,
      visual: (
        <div className='mt-5 space-y-3'>
          <div className='flex items-center justify-between text-xs'>
            <span className='text-muted-foreground'>{t('Today usage')}</span>
            <span className='font-mono'>128k tokens</span>
          </div>
          <div className='bg-muted h-2 overflow-hidden rounded-full'>
            <div className='h-full w-[68%] rounded-full bg-amber-500' />
          </div>
          <div className='text-muted-foreground text-xs'>
            {t('Budget signals stay visible before scale hurts.')}
          </div>
        </div>
      ),
    },
    {
      id: 'radar',
      num: '04',
      title: t('Transparent service status'),
      desc: t(
        'Connect LLMHub Radar to show real probe results, first-token latency, and incident updates.'
      ),
      span: 'md:col-span-2',
      icon: <Activity className='size-4 text-violet-500' />,
      visual: (
        <div className='mt-5'>
          <div className='mb-2 flex items-center justify-between text-xs'>
            <span className='text-muted-foreground'>{t('Recent probes')}</span>
            <span className='font-medium text-emerald-600'>
              {t('Operational')}
            </span>
          </div>
          <div className='grid grid-cols-8 gap-1'>
            {STATUS_BLOCKS.map((item, index) => (
              <span key={index} className={cn('h-7 rounded-[3px]', item)} />
            ))}
          </div>
        </div>
      ),
    },
  ]

  const additionalFeatures = [
    {
      icon: <Code2 className='size-5' strokeWidth={1.5} />,
      title: t('Developer experience'),
      desc: t('OpenAI-compatible routes and familiar SDK integration.'),
    },
    {
      icon: <BrainCircuit className='size-5' strokeWidth={1.5} />,
      title: t('AI-native workflows'),
      desc: t('For coding agents, content workflows, and internal AI tools.'),
    },
    {
      icon: <Users className='size-5' strokeWidth={1.5} />,
      title: t('Team ready'),
      desc: t('Make model access manageable for small teams and operators.'),
    },
    {
      icon: <ShieldCheck className='size-5' strokeWidth={1.5} />,
      title: t('Risk boundaries'),
      desc: t(
        'Centralize keys, quotas, and access instead of sharing raw keys.'
      ),
    },
    {
      icon: <LineChart className='size-5' strokeWidth={1.5} />,
      title: t('Usage visibility'),
      desc: t('Know which apps and models are driving usage.'),
    },
  ]

  return (
    <section className='relative z-10 px-6 py-20 md:py-28'>
      <div className='mx-auto max-w-6xl'>
        <AnimateInView className='mb-12 max-w-2xl'>
          <p className='text-muted-foreground mb-3 text-xs font-medium tracking-widest uppercase'>
            {t('Why x-llm')}
          </p>
          <h2 className='text-2xl leading-tight font-bold tracking-tight md:text-3xl'>
            {t('A professional access layer,')}
            <br />
            {t('not just another proxy')}
          </h2>
          <p className='text-muted-foreground mt-4 max-w-xl text-sm leading-relaxed'>
            {t(
              'The platform should feel reliable enough for developers and clear enough for the teammates who depend on AI work getting done.'
            )}
          </p>
        </AnimateInView>

        <div className='grid gap-4 md:grid-cols-3'>
          {features.map((feature, index) => (
            <AnimateInView
              key={feature.id}
              delay={index * 100}
              animation='scale-in'
              className={cn(
                'bg-background hover:bg-muted/20 rounded-lg border border-border/70 p-6 transition-colors duration-300 md:p-7',
                feature.span
              )}
            >
              <div className='mb-4 flex items-center gap-3'>
                <span className='border-border/60 bg-muted/30 text-muted-foreground flex size-7 items-center justify-center rounded-md border text-[10px] font-semibold tabular-nums'>
                  {feature.num}
                </span>
                {feature.icon}
                <h3 className='text-sm font-semibold'>{feature.title}</h3>
              </div>
              <p className='text-muted-foreground text-sm leading-relaxed'>
                {feature.desc}
              </p>
              {feature.visual}
            </AnimateInView>
          ))}
        </div>

        <div className='mt-10 grid grid-cols-1 gap-4 md:grid-cols-5'>
          {additionalFeatures.map((feature, index) => (
            <AnimateInView
              key={feature.title}
              delay={index * 80}
              animation='fade-up'
              className='border-border/60 bg-muted/10 rounded-lg border p-4'
            >
              <div className='text-muted-foreground border-border/60 bg-background mb-3 flex size-9 items-center justify-center rounded-lg border'>
                {feature.icon}
              </div>
              <h3 className='mb-1.5 text-sm font-semibold'>{feature.title}</h3>
              <p className='text-muted-foreground text-xs leading-relaxed'>
                {feature.desc}
              </p>
            </AnimateInView>
          ))}
        </div>
      </div>
    </section>
  )
}
