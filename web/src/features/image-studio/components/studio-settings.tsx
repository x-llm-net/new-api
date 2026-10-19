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
import { RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { ModelSelector } from '@/components/model-group-selector'
import { Button } from '@/components/ui/button'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import type { ApiKey } from '@/features/keys/types'

import { isImageModel } from '../hooks/use-studio-settings'
import type { StudioModel, StudioSettings } from '../types'

interface SettingsProps {
  open: boolean
  onOpenChange(open: boolean): void
  settings: StudioSettings
  onChange(settings: Partial<StudioSettings>): void
  tokens: ApiKey[]
  chatModels: StudioModel[]
  imageModels: StudioModel[]
  loading: boolean
  error: Error | null
  busy: boolean
  onRefresh(): void
}

export function StudioSettingsPanel(props: SettingsProps) {
  const { t } = useTranslation()
  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      contentClassName='sm:max-w-lg'
      bodyClassName='space-y-4'
      title={t('Creation settings')}
      description={t(
        'Use your existing API keys. Models follow the selected key’s permissions.'
      )}
    >
      <fieldset disabled={props.busy} className='space-y-6 disabled:opacity-60'>
        <div className='space-y-3'>
          <label className='grid gap-2 text-sm font-medium'>
            {t('Chat API key')}
            <NativeSelect
              aria-label={t('Chat API key')}
              className='w-full [&_select]:h-10'
              value={props.settings.chatTokenId ?? ''}
              onChange={(event) =>
                props.onChange({
                  chatTokenId: Number(event.target.value) || null,
                  chatModel: '',
                  imageModel: props.settings.imageTokenId
                    ? props.settings.imageModel
                    : '',
                })
              }
            >
              <NativeSelectOption value=''>
                {t('Select an API key')}
              </NativeSelectOption>
              {props.tokens.map((token) => (
                <NativeSelectOption key={token.id} value={token.id}>
                  {token.name}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </label>
          <div role='group' aria-label={t('Chat model')} className='grid gap-2'>
            <span className='text-sm font-medium'>{t('Chat model')}</span>
            <ModelSelector
              className='h-10 w-full justify-between rounded-lg px-3 sm:w-full sm:justify-between [&>span]:block [&>span]:text-sm [&>svg:first-child]:hidden [&>svg:last-child]:block'
              selectedModel={props.settings.chatModel}
              models={props.chatModels.map((model) => ({
                label: model.id,
                value: model.id,
              }))}
              onModelChange={(chatModel) => props.onChange({ chatModel })}
              disabled={props.loading || props.busy}
            />
          </div>
        </div>
        <div className='border-border space-y-3 border-t pt-5'>
          <label className='grid gap-2 text-sm font-medium'>
            {t('Image API key')}
            <NativeSelect
              aria-label={t('Image API key')}
              className='w-full [&_select]:h-10'
              value={props.settings.imageTokenId ?? ''}
              onChange={(event) =>
                props.onChange({
                  imageTokenId: Number(event.target.value) || null,
                  imageModel: '',
                })
              }
            >
              <NativeSelectOption value=''>
                {t('Same as chat')}
              </NativeSelectOption>
              {props.tokens.map((token) => (
                <NativeSelectOption key={token.id} value={token.id}>
                  {token.name}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </label>
          <div
            role='group'
            aria-label={t('Image model')}
            className='grid gap-2'
          >
            <span className='text-sm font-medium'>{t('Image model')}</span>
            <ModelSelector
              className='h-10 w-full justify-between rounded-lg px-3 sm:w-full sm:justify-between [&>span]:block [&>span]:text-sm [&>svg:first-child]:hidden [&>svg:last-child]:block'
              selectedModel={props.settings.imageModel}
              models={[...props.imageModels]
                .sort(
                  (a, b) => Number(isImageModel(b)) - Number(isImageModel(a))
                )
                .map((model) => ({ label: model.id, value: model.id }))}
              onModelChange={(imageModel) => props.onChange({ imageModel })}
              disabled={props.loading || props.busy}
            />
          </div>
        </div>
      </fieldset>
      {props.error && (
        <p role='alert' className='text-destructive text-sm'>
          {props.error.message}
        </p>
      )}
      {!props.loading && !props.tokens.length && (
        <p className='text-muted-foreground text-sm'>
          {t('Create or enable an API key to start creating.')}
        </p>
      )}
      <div className='flex items-center justify-between gap-2'>
        <Link
          to='/keys'
          className='text-primary text-sm underline-offset-4 hover:underline'
        >
          {t('Manage API keys')}
        </Link>
        <div className='flex gap-2'>
          <Button
            variant='ghost'
            size='icon'
            aria-label={t('Refresh models')}
            title={t('Refresh models')}
            disabled={props.loading || props.busy}
            onClick={props.onRefresh}
          >
            <RefreshCw
              className={props.loading ? 'size-4 animate-spin' : 'size-4'}
            />
          </Button>
          <Button onClick={() => props.onOpenChange(false)}>{t('Done')}</Button>
        </div>
      </div>
    </Dialog>
  )
}
