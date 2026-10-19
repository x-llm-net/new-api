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
  ArrowUp,
  MessageSquare,
  Plus,
  SlidersHorizontal,
  Square,
  X,
} from 'lucide-react'
import { useImperativeHandle, useRef, useState, type Ref } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import { useImageUrl } from '../hooks/use-image-url'
import { readComposerDocument } from '../lib/composer-document'
import type { ImageMark, StudioImage } from '../types'

export interface ComposerHandle {
  insertMark(mark: ImageMark): void
  removeMark(id: string): void
  clear(): void
}

interface ComposerProps {
  ref?: Ref<ComposerHandle>
  compact: boolean
  busy: boolean
  model: string
  attachments: StudioImage[]
  onSubmit(text: string, markIds: string[]): boolean
  onUpload(files: File[]): void
  onRemoveAttachment(id: string): void
  onSettings(): void
  onStop(): void
  onExpand(): void
  onMarksChange(ids: string[]): void
  onOpenMark?(id: string): void
}

function Attachment(props: { image: StudioImage; onRemove(): void }) {
  const url = useImageUrl(props.image.blob)
  const { t } = useTranslation()
  return (
    <div className='relative shrink-0'>
      <img
        src={url}
        alt={props.image.name}
        className='size-14 rounded-lg object-cover'
      />
      <Button
        variant='ghost'
        size='icon-xs'
        type='button'
        aria-label={t('Remove reference image')}
        onClick={props.onRemove}
        className='bg-background text-foreground absolute -top-1 -right-1 h-auto w-auto rounded-full border p-0.5 shadow-sm'
      >
        <X className='size-3' />
      </Button>
    </div>
  )
}

export function StudioComposer(props: ComposerProps) {
  const { t } = useTranslation()
  const editor = useRef<HTMLDivElement>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  const selection = useRef<Range | null>(null)
  const [hasText, setHasText] = useState(false)
  const [tooLong, setTooLong] = useState(false)
  const update = () => {
    if (!editor.current) return
    const draft = readComposerDocument(editor.current)
    setHasText(Boolean(draft.text))
    setTooLong(draft.text.length > 16000)
    props.onMarksChange(draft.markIds)
    if (!draft.text) editor.current.replaceChildren()
  }
  const rememberSelection = () => {
    const selected = window.getSelection()
    if (
      selected?.rangeCount &&
      editor.current?.contains(selected.getRangeAt(0).commonAncestorContainer)
    ) {
      selection.current = selected.getRangeAt(0).cloneRange()
    }
  }
  const insertNode = (node: Node) => {
    const target = editor.current
    if (!target) return
    const range =
      selection.current &&
      target.contains(selection.current.commonAncestorContainer)
        ? selection.current
        : document.createRange()
    if (!selection.current || !target.contains(range.commonAncestorContainer)) {
      range.selectNodeContents(target)
      range.collapse(false)
    }
    range.deleteContents()
    range.insertNode(node)
    range.setStartAfter(node)
    range.collapse(true)
    target.focus()
    const selected = window.getSelection()
    selected?.removeAllRanges()
    selected?.addRange(range)
    selection.current = range.cloneRange()
    update()
  }
  useImperativeHandle(props.ref, () => ({
    insertMark: (mark) => {
      const chip = document.createElement('span')
      chip.dataset.markId = mark.id
      chip.dataset.markNumber = String(mark.number)
      chip.contentEditable = 'false'
      chip.className = 'studio-reference'
      chip.setAttribute('role', 'button')
      chip.tabIndex = 0
      chip.textContent = String(mark.number)
      chip.setAttribute('aria-label', `${t('Image reference')} ${mark.number}`)
      insertNode(chip)
    },
    removeMark: (id) => {
      editor.current?.querySelectorAll('[data-mark-id]').forEach((node) => {
        if ((node as HTMLElement).dataset.markId === id) node.remove()
      })
      update()
    },
    clear: () => {
      editor.current?.replaceChildren()
      selection.current = null
      setHasText(false)
      setTooLong(false)
    },
  }))
  const submit = () => {
    if (!editor.current || props.busy) return
    const draft = readComposerDocument(editor.current)
    if (draft.text.length > 16000) return
    if (props.onSubmit(draft.text, draft.markIds)) {
      editor.current.replaceChildren()
      selection.current = null
      setHasText(false)
    }
  }

  return (
    <div
      className={cn(
        'bg-card border-border rounded-3xl border shadow-[0_8px_40px_-20px_rgba(0,0,0,0.25)]',
        props.compact ? 'p-2.5' : 'p-4'
      )}
    >
      {props.attachments.length > 0 && (
        <div className='mb-3 flex gap-3 overflow-x-auto px-1 pt-1'>
          {props.attachments.map((image) => (
            <Attachment
              key={image.id}
              image={image}
              onRemove={() => props.onRemoveAttachment(image.id)}
            />
          ))}
        </div>
      )}
      <div className={cn(props.compact && 'flex items-end gap-2')}>
        {props.compact && (
          <Button
            variant='ghost'
            size='icon'
            aria-label={t('Expand conversation')}
            title={t('Expand conversation')}
            onClick={props.onExpand}
          >
            <MessageSquare className='size-4' />
          </Button>
        )}
        <div
          ref={editor}
          role='textbox'
          aria-label={t('Describe your image')}
          aria-multiline='true'
          contentEditable
          suppressContentEditableWarning
          data-placeholder={t(
            'Describe what you want to create, or mark an image to tell AI what to change…'
          )}
          className={cn(
            'studio-editor min-w-0 flex-1 overflow-y-auto text-sm leading-7 whitespace-pre-wrap outline-none',
            props.compact ? 'max-h-32 min-h-9 py-1' : 'max-h-52 min-h-24'
          )}
          onInput={update}
          onKeyUp={rememberSelection}
          onMouseUp={rememberSelection}
          onBlur={rememberSelection}
          onClick={(event) => {
            const chip = (event.target as HTMLElement).closest<HTMLElement>(
              '[data-mark-id]'
            )
            if (chip?.dataset.markId) props.onOpenMark?.(chip.dataset.markId)
          }}
          onKeyDown={(event) => {
            const chip = (event.target as HTMLElement).closest<HTMLElement>(
              '[data-mark-id]'
            )
            if (chip?.dataset.markId && ['Enter', ' '].includes(event.key)) {
              event.preventDefault()
              props.onOpenMark?.(chip.dataset.markId)
              return
            }
            if (
              event.key === 'Enter' &&
              !event.shiftKey &&
              !event.nativeEvent.isComposing
            ) {
              event.preventDefault()
              submit()
            }
          }}
          onPaste={(event) => {
            event.preventDefault()
            const files = [...event.clipboardData.files].filter((file) =>
              file.type.startsWith('image/')
            )
            if (files.length) {
              props.onUpload(files)
              return
            }
            rememberSelection()
            insertNode(
              document.createTextNode(event.clipboardData.getData('text/plain'))
            )
          }}
        />
        {props.compact && (
          <Button
            className='rounded-full'
            size='icon'
            aria-label={props.busy ? t('Stop') : t('Send')}
            disabled={!props.busy && (!hasText || tooLong)}
            onClick={props.busy ? props.onStop : submit}
          >
            {props.busy ? (
              <Square className='size-3.5 fill-current' />
            ) : (
              <ArrowUp className='size-4' />
            )}
          </Button>
        )}
      </div>
      {!props.compact && (
        <div className='mt-3 flex items-center justify-between gap-2'>
          <div className='flex min-w-0 items-center gap-1.5'>
            <Button
              variant='ghost'
              size='icon'
              aria-label={t('Upload reference image')}
              title={t('Upload reference image')}
              disabled={props.busy}
              onClick={() => fileInput.current?.click()}
            >
              <Plus className='size-5' />
            </Button>
            <Button
              variant='ghost'
              size='sm'
              className='text-muted-foreground min-w-0 rounded-full'
              onClick={props.onSettings}
            >
              <SlidersHorizontal className='size-3.5 shrink-0' />
              <span className='max-w-48 truncate text-xs'>
                {props.model || t('Choose models')}
              </span>
            </Button>
          </div>
          <Button
            className='rounded-full'
            size='icon'
            aria-label={props.busy ? t('Stop') : t('Send')}
            disabled={!props.busy && (!hasText || tooLong)}
            onClick={props.busy ? props.onStop : submit}
          >
            {props.busy ? (
              <Square className='size-3.5 fill-current' />
            ) : (
              <ArrowUp className='size-4' />
            )}
          </Button>
        </div>
      )}
      {tooLong && (
        <p role='alert' className='text-destructive mt-2 text-xs'>
          {t('Please keep your message under 16,000 characters.')}
        </p>
      )}
      <input
        ref={fileInput}
        type='file'
        accept='image/png,image/jpeg,image/webp'
        multiple
        className='hidden'
        aria-label={t('Upload reference image')}
        onChange={(event) => {
          props.onUpload([...(event.target.files ?? [])])
          event.target.value = ''
        }}
      />
    </div>
  )
}
