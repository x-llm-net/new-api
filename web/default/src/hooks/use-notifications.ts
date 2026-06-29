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
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNotificationStore } from '@/stores/notification-store'
import { getNotice } from '@/lib/api'
import { useStatus } from '@/hooks/use-status'

function hashString(input: string): string {
  let hash = 0
  if (!input) return '0'

  for (let i = 0; i < input.length; i += 1) {
    const chr = input.charCodeAt(i)
    hash = (hash << 5) - hash + chr
    hash |= 0
  }

  return hash.toString(36)
}

/**
 * Generate a unique key for an announcement
 * Prefer backend id, fall back to a content hash so edits register
 */
function getAnnouncementKey(item: Record<string, unknown>): string {
  if (!item) return ''

  const fingerprint = JSON.stringify({
    publishDate: (item?.publishDate as string) || '',
    content: ((item?.content as string) || '').trim(),
    extra: ((item?.extra as string) || '').trim(),
    type: (item?.type as string) || '',
    title: ((item?.title as string) || '').trim(),
    link: ((item?.link as string) || '').trim(),
  })
  const hash = hashString(fingerprint)

  if (item.id !== undefined && item.id !== null) {
    return `id:${item.id}:${hash}`
  }

  return `hash:${hash}`
}

/**
 * Hook to manage notifications (Notice + Announcements)
 * Provides unread counts and read status management
 */
export function useNotifications() {
  const [popoverOpen, setPopoverOpen] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [activeTab, setActiveTab] = useState<'notice' | 'announcements'>(
    'notice'
  )
  const [autoDialogOpened, setAutoDialogOpened] = useState(false)

  // Fetch Notice from API
  const {
    data: noticeResponse,
    isLoading: noticeLoading,
    refetch: refetchNotice,
  } = useQuery({
    queryKey: ['notice'],
    queryFn: getNotice,
    staleTime: 1000 * 60 * 5, // 5 minutes
  })

  // Fetch Announcements from status
  const { status, loading: statusLoading } = useStatus()
  const announcementsEnabled = status?.announcements_enabled ?? false
  const announcements: Record<string, unknown>[] = useMemo(() => {
    if (!announcementsEnabled) return []
    return ((status?.announcements || []) as Record<string, unknown>[]).slice(
      0,
      20
    )
  }, [announcementsEnabled, status?.announcements])

  // Notification store
  const {
    lastReadNotice,
    markNoticeRead,
    markAnnouncementsRead,
    isAnnouncementRead,
    setClosedUntilDate,
    isNoticeClosed,
  } = useNotificationStore()

  // Extract notice content
  const noticeContent = noticeResponse?.success
    ? (noticeResponse.data || '').trim()
    : ''

  const unreadAnnouncementKeys = useMemo(
    () =>
      announcements
        .map((item: Record<string, unknown>) => {
          const key = getAnnouncementKey(item)
          return isAnnouncementRead(key) ? '' : key
        })
        .filter(Boolean),
    [announcements, isAnnouncementRead]
  )

  const hasUnreadAnnouncements = unreadAnnouncementKeys.length > 0
  const hasUnreadNotice = !!(noticeContent && noticeContent !== lastReadNotice)

  // Calculate unread counts
  const unreadCounts = useMemo(() => {
    const noticeUnread =
      noticeContent && noticeContent !== lastReadNotice ? 1 : 0

    return {
      notice: noticeUnread,
      announcements: unreadAnnouncementKeys.length,
      total: noticeUnread + unreadAnnouncementKeys.length,
    }
  }, [noticeContent, lastReadNotice, unreadAnnouncementKeys])

  const markAnnouncementsAsRead = useCallback(() => {
    if (unreadAnnouncementKeys.length > 0) {
      markAnnouncementsRead(unreadAnnouncementKeys)
    }
  }, [markAnnouncementsRead, unreadAnnouncementKeys])

  const markTabAsRead = useCallback(
    (tab: 'notice' | 'announcements') => {
      if (tab === 'notice' && noticeContent) {
        markNoticeRead(noticeContent)
      }
      if (tab === 'announcements') {
        markAnnouncementsAsRead()
      }
    },
    [markAnnouncementsAsRead, markNoticeRead, noticeContent]
  )

  const handleCloseToday = useCallback(() => {
    setClosedUntilDate(new Date().toDateString())
    setDialogOpen(false)
    markTabAsRead(activeTab)
  }, [activeTab, markTabAsRead, setClosedUntilDate])

  useEffect(() => {
    if (autoDialogOpened || dialogOpen || noticeLoading || statusLoading) return
    if (isNoticeClosed()) return
    if (!hasUnreadNotice && !hasUnreadAnnouncements) return

    const nextTab =
      hasUnreadNotice || !hasUnreadAnnouncements ? 'notice' : 'announcements'

    // Auto-open is intentionally state-driven after async notice/status loads.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setActiveTab(nextTab)
    setAutoDialogOpened(true)
    setDialogOpen(true)
  }, [
    autoDialogOpened,
    dialogOpen,
    hasUnreadAnnouncements,
    hasUnreadNotice,
    isNoticeClosed,
    markTabAsRead,
    noticeLoading,
    statusLoading,
  ])

  // Handle popover open
  const handleOpenPopover = (tab?: 'notice' | 'announcements') => {
    const nextTab = tab || activeTab

    markTabAsRead(nextTab)

    setActiveTab(nextTab)
    setPopoverOpen(true)
  }

  const handlePopoverOpenChange = (open: boolean) => {
    if (open) {
      handleOpenPopover(activeTab)
      return
    }

    setPopoverOpen(false)
  }

  const handleDialogOpenChange = (open: boolean) => {
    if (!open) {
      markTabAsRead(activeTab)
    }
    setDialogOpen(open)
  }

  // Handle tab change - mark announcements as read when switching to that tab
  const handleTabChange = (tab: 'notice' | 'announcements') => {
    setActiveTab(tab)

    if (tab === 'announcements') {
      markAnnouncementsAsRead()
    }
  }

  return {
    // Data
    notice: noticeContent,
    announcements,
    loading: noticeLoading || statusLoading,

    // Unread counts
    unreadCount: unreadCounts.total,
    unreadNoticeCount: unreadCounts.notice,
    unreadAnnouncementsCount: unreadCounts.announcements,

    // Popover state
    popoverOpen,
    setPopoverOpen: handlePopoverOpenChange,
    dialogOpen,
    setDialogOpen: handleDialogOpenChange,
    activeTab,
    setActiveTab: handleTabChange,

    // Actions
    openPopover: handleOpenPopover,
    closePopover: () => setPopoverOpen(false),
    closeToday: handleCloseToday,
    refetchNotice,
  }
}
