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
import { LoaderCircle } from 'lucide-react'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { Response } from '@/components/ai-elements/response'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import { useImageUrl } from '../hooks/use-image-url'
import type {
  ImageMark,
  StudioImage,
  StudioMessage,
  StudioStage,
} from '../types'

export function ImageThumbnail(props: {
  image: StudioImage
  selected?: boolean
  disabled?: boolean
  onClick(): void
}) {
  const url = useImageUrl(props.image.blob)
  return (
    <Button
      variant='ghost'
      type='button'
      title={props.image.name}
      aria-label={props.image.name}
      aria-pressed={props.selected}
      disabled={props.disabled}
      onClick={props.onClick}
      className={cn(
        'bg-muted size-16 p-0 shrink-0 overflow-hidden rounded-xl border-2 transition-colors',
        props.selected
          ? 'border-blue-500'
          : 'border-transparent hover:border-border'
      )}
    >
      {url && <img src={url} alt='' className='size-full object-cover' />}
    </Button>
  )
}

export function StudioMessages(props: {
  messages: StudioMessage[]
  images: StudioImage[]
  busy: boolean
  stage: StudioStage
  onSelectImage(id: string): void
  onOpenMark(mark: ImageMark): void
}) {
  const { t } = useTranslation()
  const viewport = useRef<HTMLDivElement>(null)
  const follow = useRef(true)
  useEffect(() => {
    const element = viewport.current
    if (element && follow.current) element.scrollTop = element.scrollHeight
  }, [props.messages, props.stage])
  let stage = t('Thinking…')
  if (props.stage === 'generating') stage = t('Creating your image…')
  if (props.stage === 'editing') stage = t('Editing your image…')
  return (
    <div
      ref={viewport}
      role='log'
      aria-label={t('Creation conversation')}
      className='min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 py-5 sm:px-6'
      onScroll={(event) => {
        const element = event.currentTarget
        follow.current =
          element.scrollHeight - element.scrollTop - element.clientHeight < 80
      }}
    >
      <div className='mx-auto max-w-3xl space-y-7'>
        {props.messages.map((message, index) => (
          <article
            key={message.id}
            className={cn(
              'text-sm leading-7',
              message.role === 'user' && 'flex flex-col items-end'
            )}
          >
            {message.text && (
              <div
                className={cn(
                  'max-w-full break-words',
                  message.role === 'user' &&
                    'bg-muted/70 rounded-2xl px-4 py-2.5'
                )}
              >
                {message.role === 'user' ? (
                  <p className='whitespace-pre-wrap'>
                    {Array.from(
                      message.text.matchAll(/\[\d+\]|[^[]+|\[/g),
                      (match) => {
                        const part = match[0]
                        const mark = message.marks.find(
                          (item) => part === `[${item.number}]`
                        )
                        if (
                          !mark ||
                          !props.images.some(
                            (image) => image.id === mark.imageId
                          )
                        ) {
                          return part
                        }
                        return (
                          <Button
                            variant='ghost'
                            key={`${mark.id}:${match.index}`}
                            type='button'
                            className='studio-reference h-auto p-0'
                            aria-label={`${t('Image reference')} ${mark.number}`}
                            title={`${t('Image reference')} ${mark.number}`}
                            onClick={() => props.onOpenMark(mark)}
                          >
                            {mark.number}
                          </Button>
                        )
                      }
                    )}
                  </p>
                ) : (
                  <Response>{message.text}</Response>
                )}
              </div>
            )}
            {message.imageIds.length > 0 && (
              <div className='mt-3 flex flex-wrap gap-2'>
                {message.imageIds.map((id) => {
                  const image = props.images.find((item) => item.id === id)
                  return image ? (
                    <ImageThumbnail
                      key={id}
                      image={image}
                      onClick={() => props.onSelectImage(id)}
                    />
                  ) : null
                })}
              </div>
            )}
            {message.error && (
              <p
                role='alert'
                className='text-destructive mt-2 text-xs leading-5'
              >
                {message.error}
              </p>
            )}
            {message.warning && (
              <p
                role='status'
                className='mt-2 text-xs leading-5 text-amber-700 dark:text-amber-400'
              >
                {message.warning}
              </p>
            )}
            {message.role === 'assistant' &&
              props.busy &&
              index === props.messages.length - 1 && (
                <div
                  role='status'
                  className='text-muted-foreground mt-3 flex items-center gap-2 text-xs'
                >
                  <LoaderCircle className='size-3.5 animate-spin' />
                  {stage}
                </div>
              )}
            {message.role === 'assistant' &&
              !props.busy &&
              !message.text &&
              !message.imageIds.length &&
              !message.error && (
                <p className='text-muted-foreground text-xs'>
                  {t(
                    'This reply was interrupted. You can send another message.'
                  )}
                </p>
              )}
          </article>
        ))}
      </div>
    </div>
  )
}
