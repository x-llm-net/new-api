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
  Activity,
  KeyRound,
  Mail,
  MessageCircle,
  Wallet,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

const DOCS_URL =
  'https://scny66s85sz6.feishu.cn/wiki/LCXGwWNa8iXpAukZ0zBcSvdon9d'

type FooterLink = {
  href: string
  label: string
}

type FooterColumn = {
  links: FooterLink[]
  title: string
}

function getFooterColumns(statusUrl: string): FooterColumn[] {
  return [
    {
      title: 'Product',
      links: [
        { label: 'Pricing', href: '/pricing' },
        { label: 'Buy credits', href: '/wallet' },
        { label: 'API keys', href: '/keys' },
        { label: 'Service status', href: statusUrl },
      ],
    },
    {
      title: 'Resources',
      links: [
        { label: 'Setup docs', href: DOCS_URL },
        { label: 'Model pricing', href: '/pricing' },
        { label: 'Status transparency', href: statusUrl },
        { label: 'API endpoint: https://x-llm.net/v1', href: DOCS_URL },
      ],
    },
  ]
}

function FooterLinkItem(props: FooterLink) {
  const { t } = useTranslation()
  const isExternal = props.href.startsWith('http')

  if (isExternal) {
    return (
      <a
        href={props.href}
        target='_blank'
        rel='noopener noreferrer'
        className='text-muted-foreground hover:text-foreground text-sm transition-colors'
      >
        {t(props.label)}
      </a>
    )
  }

  return (
    <Link
      to={props.href}
      className='text-muted-foreground hover:text-foreground text-sm transition-colors'
    >
      {t(props.label)}
    </Link>
  )
}

function ContactRow(props: { icon: LucideIcon; label: string; value: string }) {
  const { t } = useTranslation()
  const Icon = props.icon

  return (
    <div className='flex items-start gap-3'>
      <div className='border-border/70 bg-background flex size-8 shrink-0 items-center justify-center rounded-lg border text-red-700'>
        <Icon className='size-4' />
      </div>
      <div className='min-w-0'>
        <div className='text-muted-foreground text-xs'>{t(props.label)}</div>
        <div className='text-foreground mt-0.5 text-sm font-semibold break-all'>
          {props.value}
        </div>
      </div>
    </div>
  )
}

export function HomeFooter() {
  const { t, i18n } = useTranslation()
  const currentYear = new Date().getFullYear()
  const locale = i18n.language?.startsWith('zh') ? 'zh' : 'en'
  const statusUrl = `https://llm-hub.store/x-llm/${locale}`
  const footerColumns = getFooterColumns(statusUrl)

  return (
    <footer className='dark:bg-background border-border/70 relative z-10 border-t bg-white px-6'>
      <div className='mx-auto max-w-7xl py-12 md:py-14'>
        <div className='grid gap-10 lg:grid-cols-[1.15fr_1fr_1fr]'>
          <div className='max-w-md'>
            <Link to='/' className='inline-flex items-center gap-3'>
              <div className='flex size-10 items-center justify-center rounded-xl bg-red-700 text-sm font-black text-white shadow-sm'>
                X
              </div>
              <div>
                <div className='text-foreground text-lg font-black tracking-tight'>
                  x-llm
                </div>
                <div className='text-muted-foreground text-xs font-medium'>
                  {t('Native LLM Router')}
                </div>
              </div>
            </Link>
            <p className='text-muted-foreground mt-5 text-sm leading-7'>
              {t(
                'One key connects top global LLMs for Claude Code, Codex, agents, and internal AI tools, with transparent pricing and visible service status.'
              )}
            </p>
            <div className='mt-5 flex flex-wrap gap-2'>
              {[
                { icon: KeyRound, label: 'One key' },
                { icon: Wallet, label: 'Pay as you go' },
                { icon: Activity, label: 'Observed by LLMHub Radar' },
              ].map((item) => {
                const Icon = item.icon

                return (
                  <span
                    key={item.label}
                    className='border-border/70 bg-background text-muted-foreground inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-semibold'
                  >
                    <Icon className='size-3.5 text-red-700' />
                    {t(item.label)}
                  </span>
                )
              })}
            </div>
          </div>

          <div className='grid gap-8 sm:grid-cols-2 lg:grid-cols-2'>
            {footerColumns.map((column) => (
              <div key={column.title}>
                <h3 className='text-foreground text-sm font-bold'>
                  {t(column.title)}
                </h3>
                <ul className='mt-4 space-y-3'>
                  {column.links.map((link) => (
                    <li key={`${column.title}-${link.label}`}>
                      <FooterLinkItem {...link} />
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>

          <div>
            <h3 className='text-foreground text-sm font-bold'>
              {t('Contact')}
            </h3>
            <div className='mt-4 space-y-4'>
              <ContactRow
                icon={MessageCircle}
                label='QQ group'
                value='189370617'
              />
              <ContactRow
                icon={MessageCircle}
                label='Business WeChat'
                value='keke7u'
              />
              <ContactRow
                icon={Mail}
                label='Support email'
                value='support@x-llm.net'
              />
            </div>
          </div>
        </div>

        <div className='border-border/70 text-muted-foreground mt-10 flex flex-col gap-3 border-t pt-6 text-xs sm:flex-row sm:items-center sm:justify-between'>
          <div>
            &copy; {currentYear} x-llm. {t('All rights reserved.')}
          </div>
          <div className='flex flex-wrap gap-x-4 gap-y-2'>
            <Link to='/user-agreement' className='hover:text-foreground'>
              {t('User Agreement')}
            </Link>
            <Link to='/privacy-policy' className='hover:text-foreground'>
              {t('Privacy Policy')}
            </Link>
            <a
              href='https://github.com/QuantumNous/new-api'
              target='_blank'
              rel='noopener noreferrer'
              className='hover:text-foreground'
            >
              {t('Built on New API')}
            </a>
          </div>
        </div>
      </div>
    </footer>
  )
}
