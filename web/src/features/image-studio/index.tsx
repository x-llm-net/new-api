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
  History,
  ImageIcon,
  MessageSquare,
  PanelLeftClose,
  Plus,
  Settings2,
} from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { ImagePreview } from './components/image-preview'
import {
  StudioComposer,
  type ComposerHandle,
} from './components/studio-composer'
import { ImageThumbnail, StudioMessages } from './components/studio-messages'
import { StudioSettingsPanel } from './components/studio-settings'
import { useStudioConversation } from './hooks/use-studio-conversation'
import { useStudioSettings } from './hooks/use-studio-settings'
import type { ImageMark, ImageRegion } from './types'

import './studio.css'

export function ImageStudio() {
  const userId = useAuthStore((state) => state.auth.user?.id)
  if (!userId) return null
  const scope = `${window.location.host}:${userId}`
  return (
    <PublicLayout showMainContainer={false}>
      <StudioWorkspace key={scope} scope={scope} />
    </PublicLayout>
  )
}

function StudioWorkspace(props: { scope: string }) {
  const { t } = useTranslation()
  const configuration = useStudioSettings(props.scope)
  const studio = useStudioConversation(props.scope)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [collapsed, setCollapsed] = useState(false)
  const [marks, setMarks] = useState<ImageMark[]>([])
  const [reference, setReference] = useState<ImageMark | null>(null)
  const [attachments, setAttachments] = useState<string[]>([])
  const composer = useRef<ComposerHandle>(null)
  const number = useRef(1)
  const selectedId = studio.conversation.selectedImageId
  const selected = studio.images.find((image) => image.id === selectedId)
  const hasContent =
    studio.conversation.messages.length > 0 || studio.images.length > 0
  const showingImage = previewOpen && Boolean(selected)
  useEffect(() => {
    if (selectedId) setPreviewOpen(true)
  }, [selectedId])

  const selectImage = (id: string) => {
    setReference(null)
    studio.selectImage(id)
    setPreviewOpen(true)
  }
  const openReference = (mark: ImageMark) => {
    if (!studio.images.some((image) => image.id === mark.imageId)) return
    studio.selectImage(mark.imageId)
    setReference({ ...mark })
    setPreviewOpen(true)
  }
  const addMark = (region: ImageRegion) => {
    if (!selected || studio.busy) return
    const mark: ImageMark = {
      id: crypto.randomUUID(),
      number: number.current++,
      imageId: selected.id,
      region,
    }
    setMarks((current) => [...current, mark])
    composer.current?.insertMark(mark)
  }
  const resetDraft = () => {
    composer.current?.clear()
    setMarks([])
    setAttachments([])
    setReference(null)
    number.current = 1
  }
  const upload = (files: File[]) => {
    void studio.upload(files).then((images) => {
      if (images) {
        setAttachments((current) =>
          [...current, ...images.map((image) => image.id)].slice(-4)
        )
      }
    })
  }
  const composerView = (
    <StudioComposer
      ref={composer}
      compact={showingImage && collapsed}
      busy={studio.busy}
      model={configuration.settings.imageModel}
      attachments={studio.images.filter((image) =>
        attachments.includes(image.id)
      )}
      onSettings={() => setSettingsOpen(true)}
      onExpand={() => setCollapsed(false)}
      onStop={studio.stop}
      onUpload={upload}
      onRemoveAttachment={(id) =>
        setAttachments((current) => current.filter((item) => item !== id))
      }
      onMarksChange={(ids) =>
        setMarks((current) => current.filter((mark) => ids.includes(mark.id)))
      }
      onOpenMark={(id) => {
        const mark = marks.find((item) => item.id === id)
        if (mark) openReference(mark)
      }}
      onSubmit={(text, ids) => {
        if (!configuration.ready) {
          setSettingsOpen(true)
          return false
        }
        if (!text.trim()) return false
        const sent = studio.send(
          text,
          marks.filter((mark) => ids.includes(mark.id)),
          attachments,
          configuration.settings
        )
        if (sent) {
          setReference(null)
          setMarks([])
          setAttachments([])
        }
        return sent
      }}
    />
  )

  return (
    <main className='mx-auto flex h-svh max-w-[1800px] flex-col px-3 pt-24 pb-4 sm:px-6 sm:pb-6'>
      <header className='mb-3 flex h-10 shrink-0 items-center justify-between gap-3'>
        <div className='flex min-w-0 items-center gap-2'>
          {hasContent && (
            <span className='truncate text-sm font-medium'>
              {studio.conversation.title || t('Image creation')}
            </span>
          )}
          {hasContent && showingImage && (
            <Button
              variant='ghost'
              size='icon-sm'
              title={
                collapsed
                  ? t('Expand conversation')
                  : t('Collapse conversation')
              }
              aria-label={
                collapsed
                  ? t('Expand conversation')
                  : t('Collapse conversation')
              }
              aria-expanded={!collapsed}
              onClick={() => setCollapsed((current) => !current)}
            >
              {collapsed ? (
                <MessageSquare className='size-4' />
              ) : (
                <PanelLeftClose className='size-4' />
              )}
            </Button>
          )}
        </div>
        <div className='flex shrink-0 items-center gap-1'>
          {!showingImage && selected && (
            <Button
              variant='ghost'
              size='icon'
              aria-label={t('Open image')}
              title={t('Open image')}
              onClick={() => setPreviewOpen(true)}
            >
              <ImageIcon className='size-4' />
            </Button>
          )}
          <Button
            variant='ghost'
            size='sm'
            className='rounded-full'
            aria-label={t('New creation')}
            title={t('New creation')}
            disabled={studio.busy}
            onClick={() => {
              studio.startNew()
              resetDraft()
              setPreviewOpen(false)
              setCollapsed(false)
            }}
          >
            <Plus className='size-4' />
            <span className='hidden sm:inline'>{t('New creation')}</span>
          </Button>
          <Popover open={historyOpen} onOpenChange={setHistoryOpen}>
            <PopoverTrigger
              render={
                <Button
                  variant='ghost'
                  size='icon'
                  aria-label={t('Creation history')}
                  title={t('Creation history')}
                />
              }
            >
              <History className='size-4' />
            </PopoverTrigger>
            <PopoverContent align='end' className='w-80 p-3'>
              <p className='px-2 py-1 font-medium'>{t('Creation history')}</p>
              <div className='max-h-80 overflow-y-auto'>
                {studio.history.map((item) => (
                  <Button
                    variant='ghost'
                    type='button'
                    key={item.id}
                    disabled={studio.busy}
                    className='hover:bg-accent block h-auto w-full rounded-lg px-2 py-2 text-left disabled:opacity-50'
                    onClick={() => {
                      resetDraft()
                      setHistoryOpen(false)
                      void studio.open(item)
                    }}
                  >
                    <p className='truncate'>
                      {item.title || t('Untitled creation')}
                    </p>
                    <p className='text-muted-foreground text-xs'>
                      {new Date(item.updatedAt).toLocaleString()}
                    </p>
                  </Button>
                ))}
                {!studio.history.length && (
                  <p className='text-muted-foreground p-3 text-sm'>
                    {t('Your creations will appear here')}
                  </p>
                )}
              </div>
              <p className='text-muted-foreground border-t px-2 pt-2 text-xs'>
                {t('Saved in this browser')}
              </p>
            </PopoverContent>
          </Popover>
          <Button
            variant='ghost'
            size='icon'
            aria-label={t('Creation settings')}
            title={t('Creation settings')}
            onClick={() => setSettingsOpen(true)}
          >
            <Settings2 className='size-4' />
          </Button>
        </div>
      </header>
      {(studio.error || configuration.storageError) && (
        <p role='alert' className='text-destructive mb-2 shrink-0 text-sm'>
          {studio.error || t('Settings could not be saved in this browser.')}
        </p>
      )}
      <div className='relative flex min-h-0 flex-1 flex-col gap-4 md:flex-row'>
        <section
          className={cn(
            'flex min-h-0 flex-col',
            !hasContent &&
              'mx-auto w-full max-w-3xl flex-1 justify-center gap-8 pb-[12vh]',
            hasContent && !showingImage && 'mx-auto h-full w-full max-w-4xl',
            showingImage &&
              !collapsed &&
              'h-[42%] shrink-0 md:h-full md:w-[30%] md:max-w-[420px]',
            showingImage &&
              collapsed &&
              'absolute right-4 bottom-4 left-4 z-10 mx-auto max-w-xl md:left-auto md:w-[min(560px,calc(100%-2rem))]'
          )}
        >
          {!hasContent && (
            <h1 className='text-center text-2xl font-medium tracking-tight sm:text-3xl'>
              {t('What would you like to create?')}
            </h1>
          )}
          {hasContent && (!showingImage || !collapsed) && (
            <StudioMessages
              messages={studio.conversation.messages}
              images={studio.images}
              busy={studio.busy}
              stage={studio.stage}
              onSelectImage={selectImage}
              onOpenMark={openReference}
            />
          )}
          <div className='shrink-0 pt-2'>{composerView}</div>
        </section>
        {showingImage && selected && (
          <section
            aria-label={t('Image preview')}
            className='bg-muted/25 flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-2xl p-2'
          >
            <div className='min-h-0 flex-1'>
              <ImagePreview
                key={selected.id}
                image={selected}
                marks={marks}
                reference={reference}
                disabled={studio.busy}
                onMark={addMark}
                onRemoveMark={(id) => {
                  if (reference?.id === id) setReference(null)
                  composer.current?.removeMark(id)
                  setMarks((current) =>
                    current.filter((mark) => mark.id !== id)
                  )
                }}
                onClose={() => {
                  setReference(null)
                  setPreviewOpen(false)
                  setCollapsed(false)
                }}
              />
            </div>
            <div
              aria-label={t('Images and versions')}
              className={cn(
                'flex shrink-0 justify-center gap-2 overflow-x-auto px-2 py-1.5',
                collapsed && 'pb-24'
              )}
            >
              {studio.images.map((image) => (
                <ImageThumbnail
                  key={image.id}
                  image={image}
                  selected={image.id === selected.id}
                  disabled={studio.busy}
                  onClick={() => selectImage(image.id)}
                />
              ))}
            </div>
          </section>
        )}
      </div>
      <StudioSettingsPanel
        open={settingsOpen}
        onOpenChange={setSettingsOpen}
        settings={configuration.settings}
        onChange={configuration.update}
        tokens={configuration.tokens}
        chatModels={configuration.chatModels}
        imageModels={configuration.imageModels}
        loading={configuration.loading}
        error={configuration.error}
        busy={studio.busy}
        onRefresh={configuration.refresh}
      />
    </main>
  )
}
