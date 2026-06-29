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
import type { TFunction } from 'i18next'
import { Bell, Megaphone } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getAnnouncementColorClass } from '@/lib/colors'
import { formatDateTimeObject } from '@/lib/time'
import { cn } from '@/lib/utils'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Markdown } from '@/components/ui/markdown'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

interface AnnouncementItem {
  type?: string
  content?: string
  extra?: string
  publishDate?: string | Date
}

interface NotificationDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  activeTab: 'notice' | 'announcements'
  onTabChange: (tab: 'notice' | 'announcements') => void
  notice: string
  announcements: AnnouncementItem[]
  loading: boolean
  onCloseToday: () => void
}

function getRelativeTime(publishDate: string | Date, t: TFunction): string {
  if (!publishDate) return ''

  const now = new Date()
  const pubDate = new Date(publishDate)

  if (isNaN(pubDate.getTime())) {
    return typeof publishDate === 'string' ? publishDate : ''
  }

  const diffMs = now.getTime() - pubDate.getTime()
  const diffSeconds = Math.floor(diffMs / 1000)
  const diffMinutes = Math.floor(diffSeconds / 60)
  const diffHours = Math.floor(diffMinutes / 60)
  const diffDays = Math.floor(diffHours / 24)
  const diffWeeks = Math.floor(diffDays / 7)
  const diffMonths = Math.floor(diffDays / 30)
  const diffYears = Math.floor(diffDays / 365)

  if (diffMs < 0) return formatDateTimeObject(pubDate)
  if (diffSeconds < 60) return t('Just now')
  if (diffMinutes < 60) {
    return diffMinutes === 1
      ? t('1 minute ago')
      : t('{{count}} minutes ago', { count: diffMinutes })
  }
  if (diffHours < 24) {
    return diffHours === 1
      ? t('1 hour ago')
      : t('{{count}} hours ago', { count: diffHours })
  }
  if (diffDays < 7) {
    return diffDays === 1
      ? t('1 day ago')
      : t('{{count}} days ago', { count: diffDays })
  }
  if (diffWeeks < 4) {
    return diffWeeks === 1
      ? t('1 week ago')
      : t('{{count}} weeks ago', { count: diffWeeks })
  }
  if (diffMonths < 12) {
    return diffMonths === 1
      ? t('1 month ago')
      : t('{{count}} months ago', { count: diffMonths })
  }
  if (diffYears < 2) return t('1 year ago')
  return formatDateTimeObject(pubDate)
}

function AnnouncementDot({ type }: { type?: string }) {
  return (
    <span
      className={cn(
        'mt-1.5 inline-block size-2 shrink-0 rounded-full',
        getAnnouncementColorClass(type)
      )}
    />
  )
}

function NoticePanel({
  notice,
  loading,
}: {
  notice: string
  loading: boolean
}) {
  const { t } = useTranslation()

  if (loading) {
    return (
      <div className='text-muted-foreground py-10 text-center'>
        {t('Loading...')}
      </div>
    )
  }

  if (!notice) {
    return (
      <div className='text-muted-foreground py-10 text-center'>
        {t('No announcements at this time')}
      </div>
    )
  }

  return (
    <ScrollArea className='h-[min(52vh,28rem)] pr-3'>
      <Markdown>{notice}</Markdown>
    </ScrollArea>
  )
}

function AnnouncementsPanel({
  announcements,
  loading,
}: {
  announcements: AnnouncementItem[]
  loading: boolean
}) {
  const { t } = useTranslation()

  if (loading) {
    return (
      <div className='text-muted-foreground py-10 text-center'>
        {t('Loading...')}
      </div>
    )
  }

  if (announcements.length === 0) {
    return (
      <div className='text-muted-foreground py-10 text-center'>
        {t('No system announcements')}
      </div>
    )
  }

  return (
    <ScrollArea className='h-[min(52vh,28rem)] pr-3'>
      <div className='flex flex-col'>
        {announcements.map((item, idx) => {
          const publishDate = item.publishDate
            ? new Date(item.publishDate)
            : null
          const relativeTime = publishDate
            ? getRelativeTime(publishDate, t)
            : ''
          const absoluteTime = publishDate
            ? formatDateTimeObject(publishDate)
            : ''
          const displayTime = [relativeTime, absoluteTime]
            .filter(Boolean)
            .join(' - ')

          return (
            <div key={idx}>
              <div className='py-3'>
                <div className='flex items-start gap-3'>
                  <AnnouncementDot type={item.type} />
                  <div className='flex min-w-0 flex-1 flex-col gap-2'>
                    <div className='text-sm'>
                      <Markdown>{item.content || ''}</Markdown>
                    </div>
                    {item.extra ? (
                      <div className='text-muted-foreground text-xs'>
                        <Markdown>{item.extra}</Markdown>
                      </div>
                    ) : null}
                    {displayTime ? (
                      <div className='text-muted-foreground text-xs'>
                        {displayTime}
                      </div>
                    ) : null}
                  </div>
                </div>
              </div>
              {idx < announcements.length - 1 ? <Separator /> : null}
            </div>
          )
        })}
      </div>
    </ScrollArea>
  )
}

export function NotificationDialog(props: NotificationDialogProps) {
  const { t } = useTranslation()

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('System Notice')}
      description={t('Latest platform updates and notices')}
      contentClassName='sm:max-w-3xl'
      footer={
        <>
          <Button variant='outline' onClick={props.onCloseToday}>
            {t('Close Today')}
          </Button>
          <Button onClick={() => props.onOpenChange(false)}>
            {t('Close')}
          </Button>
        </>
      }
    >
      <Tabs
        value={props.activeTab}
        onValueChange={props.onTabChange as (value: string) => void}
      >
        <TabsList className='grid w-full grid-cols-2'>
          <TabsTrigger value='notice' className='gap-1.5'>
            <Bell className='size-3.5' />
            {t('Notice')}
          </TabsTrigger>
          <TabsTrigger value='announcements' className='gap-1.5'>
            <Megaphone className='size-3.5' />
            {t('Timeline')}
          </TabsTrigger>
        </TabsList>

        <TabsContent value='notice' className='mt-3'>
          <NoticePanel notice={props.notice} loading={props.loading} />
        </TabsContent>
        <TabsContent value='announcements' className='mt-3'>
          <AnnouncementsPanel
            announcements={props.announcements}
            loading={props.loading}
          />
        </TabsContent>
      </Tabs>
    </Dialog>
  )
}
