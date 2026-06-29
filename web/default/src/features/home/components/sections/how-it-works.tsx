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
import { Activity, KeyRound, PlugZap, SlidersHorizontal } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'

export function HowItWorks() {
  const { t } = useTranslation()

  const steps = [
    {
      num: '1',
      title: t('Create your access key'),
      desc: t(
        'Start with one API key for your app, tool, or teammate. Keep raw upstream keys centralized.'
      ),
      icon: <KeyRound className='size-5' strokeWidth={1.5} />,
    },
    {
      num: '2',
      title: t('Choose the model families'),
      desc: t(
        'Use OpenAI-compatible routes, Claude routes, Gemini routes, or your preferred NewAPI-compatible setup.'
      ),
      icon: <SlidersHorizontal className='size-5' strokeWidth={1.5} />,
    },
    {
      num: '3',
      title: t('Connect your AI stack'),
      desc: t(
        'Point Codex, Claude Code, Cursor, Cherry Studio, n8n, or internal tools at the same gateway.'
      ),
      icon: <PlugZap className='size-5' strokeWidth={1.5} />,
    },
    {
      num: '4',
      title: t('Watch reliability and usage'),
      desc: t(
        'Track balance, costs, model usage, and service status before a workflow quietly breaks.'
      ),
      icon: <Activity className='size-5' strokeWidth={1.5} />,
    },
  ]

  return (
    <section className='border-border/60 bg-muted/10 relative z-10 border-y px-6 py-20 md:py-28'>
      <div className='mx-auto max-w-6xl'>
        <AnimateInView className='mb-12 max-w-2xl'>
          <p className='text-muted-foreground mb-3 text-xs font-medium tracking-widest uppercase'>
            {t('How teams use it')}
          </p>
          <h2 className='text-2xl font-bold tracking-tight md:text-3xl'>
            {t('From first key to team-wide AI workflows')}
          </h2>
        </AnimateInView>

        <div className='grid gap-4 md:grid-cols-4'>
          {steps.map((step, index) => (
            <AnimateInView
              key={step.num}
              delay={index * 120}
              animation='fade-up'
              className='border-border/70 bg-background relative rounded-lg border p-5'
            >
              <div className='mb-5 flex items-center justify-between'>
                <div className='text-muted-foreground border-border/60 bg-muted/20 flex size-10 items-center justify-center rounded-lg border'>
                  {step.icon}
                </div>
                <span className='text-muted-foreground font-mono text-xs'>
                  0{step.num}
                </span>
              </div>
              <h3 className='mb-2 text-sm font-semibold'>{step.title}</h3>
              <p className='text-muted-foreground text-xs leading-relaxed'>
                {step.desc}
              </p>
            </AnimateInView>
          ))}
        </div>
      </div>
    </section>
  )
}
