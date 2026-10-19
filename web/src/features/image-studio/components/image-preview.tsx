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
import { Download, Maximize2, Minus, Plus, X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import { useImageUrl } from '../hooks/use-image-url'
import { regionFromPoints } from '../lib/images'
import type { ImageMark, ImageRegion, StudioImage } from '../types'

interface PreviewProps {
  image: StudioImage
  marks: ImageMark[]
  reference?: ImageMark | null
  disabled: boolean
  onMark(region: ImageRegion): void
  onRemoveMark(id: string): void
  onClose(): void
}

export function ImagePreview(props: PreviewProps) {
  const { t } = useTranslation()
  const url = useImageUrl(props.image.blob)
  const viewport = useRef<HTMLDivElement>(null)
  const imageElement = useRef<HTMLDivElement>(null)
  const [bounds, setBounds] = useState({ width: 600, height: 500 })
  const [view, setView] = useState({ zoom: 1, x: 0, y: 0 })
  const [draft, setDraft] = useState<ImageRegion | null>(null)
  const [panning, setPanning] = useState(false)
  const gesture = useRef<{
    pointerId: number
    pan: boolean
    x: number
    y: number
    viewX: number
    viewY: number
    start: { x: number; y: number }
  } | null>(null)
  const fit = Math.min(
    (bounds.width - 24) / props.image.width,
    (bounds.height - 24) / props.image.height
  )
  const width = props.image.width * Math.max(0.01, fit) * view.zoom
  const height = props.image.height * Math.max(0.01, fit) * view.zoom
  const historicalMark =
    props.reference?.imageId === props.image.id &&
    !props.marks.some((mark) => mark.id === props.reference?.id)
      ? props.reference
      : null
  const visibleMarks = historicalMark
    ? [...props.marks, historicalMark]
    : props.marks

  useEffect(() => {
    if (props.reference) setView({ zoom: 1, x: 0, y: 0 })
  }, [props.reference])

  useEffect(() => {
    const element = viewport.current
    if (!element) return
    const observer = new ResizeObserver(() =>
      setBounds({ width: element.clientWidth, height: element.clientHeight })
    )
    observer.observe(element)
    const wheel = (event: WheelEvent) => {
      event.preventDefault()
      if (gesture.current) return
      const rect = element.getBoundingClientRect(),
        x = event.clientX - rect.left - rect.width / 2,
        y = event.clientY - rect.top - rect.height / 2
      setView((current) => {
        const zoom = Math.max(
          0.25,
          Math.min(6, current.zoom * Math.exp(-event.deltaY * 0.0015))
        )
        return {
          zoom,
          x: x - ((x - current.x) * zoom) / current.zoom,
          y: y - ((y - current.y) * zoom) / current.zoom,
        }
      })
    }
    element.addEventListener('wheel', wheel, { passive: false })
    return () => {
      observer.disconnect()
      element.removeEventListener('wheel', wheel)
    }
  }, [])

  const point = (x: number, y: number) => {
    const rect = imageElement.current?.getBoundingClientRect()
    if (!rect?.width || !rect.height) return { x: 0, y: 0 }
    return { x: (x - rect.left) / rect.width, y: (y - rect.top) / rect.height }
  }

  return (
    <div className='flex h-full min-h-0 flex-col'>
      <div className='flex h-9 shrink-0 items-center justify-between px-2'>
        <span className='text-muted-foreground truncate text-xs'>
          {t('Current image')}
        </span>
        <div className='flex items-center gap-1'>
          <Button
            variant='ghost'
            size='icon-sm'
            aria-label={t('Zoom out')}
            onClick={() =>
              setView((current) => ({
                ...current,
                zoom: Math.max(0.25, current.zoom / 1.2),
              }))
            }
          >
            <Minus className='size-4' />
          </Button>
          <Button
            variant='ghost'
            size='sm'
            type='button'
            className='text-muted-foreground min-w-12 text-xs tabular-nums'
            title={t('Fit image')}
            onClick={() => setView({ zoom: 1, x: 0, y: 0 })}
          >
            {Math.round(view.zoom * 100)}%
          </Button>
          <Button
            variant='ghost'
            size='icon-sm'
            aria-label={t('Zoom in')}
            onClick={() =>
              setView((current) => ({
                ...current,
                zoom: Math.min(6, current.zoom * 1.2),
              }))
            }
          >
            <Plus className='size-4' />
          </Button>
          <Button
            variant='ghost'
            size='icon-sm'
            aria-label={t('Fit image')}
            onClick={() => setView({ zoom: 1, x: 0, y: 0 })}
          >
            <Maximize2 className='size-4' />
          </Button>
          <a
            href={url}
            download={`image-${props.image.id}.${props.image.blob.type === 'image/jpeg' ? 'jpg' : 'png'}`}
            aria-label={t('Download image')}
            title={t('Download image')}
            className='hover:bg-accent rounded-md p-2'
          >
            <Download className='size-4' />
          </a>
          <Button
            variant='ghost'
            size='icon-sm'
            aria-label={t('Close preview')}
            onClick={props.onClose}
          >
            <X className='size-4' />
          </Button>
        </div>
      </div>
      <div
        ref={viewport}
        className='bg-muted/40 relative min-h-48 flex-1 touch-none overflow-hidden rounded-2xl select-none'
        data-dazi-context-menu='none'
        style={{ cursor: panning ? 'grabbing' : 'crosshair' }}
        onContextMenu={(event) => event.preventDefault()}
        onPointerDown={(event) => {
          if (
            (event.target as HTMLElement).closest('button') ||
            ![0, 2].includes(event.button)
          ) {
            return
          }
          if (
            event.button === 0 &&
            (props.disabled ||
              !imageElement.current?.contains(event.target as Node))
          ) {
            return
          }
          event.preventDefault()
          event.currentTarget.setPointerCapture(event.pointerId)
          gesture.current = {
            pointerId: event.pointerId,
            pan: event.button === 2,
            x: event.clientX,
            y: event.clientY,
            viewX: view.x,
            viewY: view.y,
            start: point(event.clientX, event.clientY),
          }
          setPanning(event.button === 2)
        }}
        onPointerMove={(event) => {
          const active = gesture.current
          if (!active || active.pointerId !== event.pointerId) return
          if (active.pan) {
            setView((current) => ({
              ...current,
              x: active.viewX + event.clientX - active.x,
              y: active.viewY + event.clientY - active.y,
            }))
          } else {
            setDraft(
              regionFromPoints(
                active.start,
                point(event.clientX, event.clientY),
                false
              )
            )
          }
        }}
        onPointerUp={(event) => {
          const active = gesture.current
          if (!active || active.pointerId !== event.pointerId) return
          gesture.current = null
          setPanning(false)
          setDraft(null)
          if (event.currentTarget.hasPointerCapture(event.pointerId)) {
            event.currentTarget.releasePointerCapture(event.pointerId)
          }
          if (!active.pan) {
            props.onMark(
              regionFromPoints(
                active.start,
                point(event.clientX, event.clientY),
                Math.hypot(event.clientX - active.x, event.clientY - active.y) <
                  5
              )
            )
          }
        }}
        onPointerCancel={() => {
          gesture.current = null
          setPanning(false)
          setDraft(null)
        }}
      >
        <div
          ref={imageElement}
          className='absolute shadow-lg'
          style={{
            width,
            height,
            left: (bounds.width - width) / 2 + view.x,
            top: (bounds.height - height) / 2 + view.y,
          }}
        >
          <img
            src={url}
            alt={props.image.name}
            draggable={false}
            className='pointer-events-none block size-full'
          />
          {visibleMarks
            .filter((mark) => mark.imageId === props.image.id)
            .map((mark) => (
              <div
                key={mark.id}
                className='pointer-events-none absolute border-2 border-blue-500'
                style={{
                  left: `${mark.region.x * 100}%`,
                  top: `${mark.region.y * 100}%`,
                  width: `${mark.region.w * 100}%`,
                  height: `${mark.region.h * 100}%`,
                }}
              >
                <span className='absolute -top-3 -left-3 flex size-6 items-center justify-center rounded-full bg-blue-600 text-xs font-semibold text-white shadow ring-2 ring-white'>
                  {mark.number}
                </span>
                <Button
                  variant='ghost'
                  size='icon-xs'
                  type='button'
                  title={
                    historicalMark?.id === mark.id
                      ? t('Hide mark')
                      : t('Remove mark')
                  }
                  aria-label={`${historicalMark?.id === mark.id ? t('Hide mark') : t('Remove mark')} ${mark.number}`}
                  className={cn(
                    'pointer-events-auto absolute flex size-5 p-0 items-center justify-center rounded-full bg-white text-slate-700 shadow ring-1 ring-slate-200 hover:bg-slate-100 hover:text-slate-950',
                    mark.region.w === 0 ? '-top-6 left-3' : '-top-3 -right-3'
                  )}
                  onClick={(event) => {
                    event.stopPropagation()
                    props.onRemoveMark(mark.id)
                  }}
                >
                  <X className='size-3' />
                </Button>
              </div>
            ))}
          {draft && (
            <div
              className='pointer-events-none absolute border-2 border-blue-500 bg-blue-500/10'
              style={{
                left: `${draft.x * 100}%`,
                top: `${draft.y * 100}%`,
                width: `${draft.w * 100}%`,
                height: `${draft.h * 100}%`,
              }}
            />
          )}
        </div>
      </div>
    </div>
  )
}
