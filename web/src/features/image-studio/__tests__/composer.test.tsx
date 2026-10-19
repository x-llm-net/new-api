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
import assert from 'node:assert/strict'

import { describe, test } from 'vitest'

const { act, createRef } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { StudioComposer } = await import('../components/studio-composer')
const { readComposerDocument } = await import('../lib/composer-document')
const i18n = createInstance()
await i18n
  .use(initReactI18next)
  .init({ lng: 'en', resources: { en: { translation: {} } } })

describe('image creation composer', () => {
  test('an oversized message stays editable and cannot be submitted until shortened', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const sent: string[] = []
    try {
      await act(async () =>
        root.render(
          <I18nextProvider i18n={i18n}>
            <StudioComposer
              compact={false}
              busy={false}
              model='image-model'
              attachments={[]}
              onSubmit={(text) => {
                sent.push(text)
                return true
              }}
              onUpload={() => {}}
              onRemoveAttachment={() => {}}
              onSettings={() => {}}
              onStop={() => {}}
              onExpand={() => {}}
              onMarksChange={() => {}}
            />
          </I18nextProvider>
        )
      )
      const editor = container.querySelector<HTMLElement>('[role="textbox"]')
      const send = container.querySelector<HTMLButtonElement>(
        '[aria-label="Send"]'
      )
      assert.ok(editor && send)
      editor.textContent = 'a'.repeat(16001)
      await act(async () =>
        editor.dispatchEvent(new Event('input', { bubbles: true }))
      )
      assert.equal(send.disabled, true)
      assert.ok(container.querySelector('[role="alert"]'))
      await act(async () =>
        editor.dispatchEvent(
          new KeyboardEvent('keydown', {
            key: 'Enter',
            bubbles: true,
            cancelable: true,
          })
        )
      )
      assert.deepEqual(sent, [])
      assert.equal(editor.textContent.length, 16001)
      editor.textContent = 'Draw a small garden'
      await act(async () =>
        editor.dispatchEvent(new Event('input', { bubbles: true }))
      )
      assert.equal(send.disabled, false)
      assert.equal(container.querySelector('[role="alert"]'), null)
      await act(async () => send.click())
      assert.deepEqual(sent, ['Draw a small garden'])
    } finally {
      await act(async () => root.unmount())
      container.remove()
    }
  })

  test('inserting a mark keeps text on both sides and submits the exact reference', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const ref =
      createRef<import('../components/studio-composer').ComposerHandle>()
    let sent: { text: string; ids: string[] } | undefined
    const opened: string[] = []
    const render = (compact: boolean) => (
      <I18nextProvider i18n={i18n}>
        <StudioComposer
          ref={ref}
          compact={compact}
          busy={false}
          model='image-model'
          attachments={[]}
          onSubmit={(text, ids) => {
            sent = { text, ids }
            return true
          }}
          onUpload={() => {}}
          onRemoveAttachment={() => {}}
          onSettings={() => {}}
          onStop={() => {}}
          onExpand={() => {}}
          onMarksChange={() => {}}
          onOpenMark={(id) => opened.push(id)}
        />
      </I18nextProvider>
    )
    try {
      await act(async () => root.render(render(false)))
      const editor = container.querySelector<HTMLElement>('[role="textbox"]')
      assert.ok(editor)
      editor.textContent = 'Make this area green'
      const range = document.createRange()
      assert.ok(editor.firstChild)
      range.setStart(editor.firstChild, 5)
      range.collapse(true)
      window.getSelection()?.removeAllRanges()
      window.getSelection()?.addRange(range)
      await act(async () =>
        editor.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }))
      )
      await act(async () =>
        ref.current?.insertMark({
          id: 'mark-1',
          imageId: 'original-1',
          number: 1,
          region: { x: 0.5, y: 0.4, w: 0.2, h: 0.2 },
        })
      )
      assert.deepEqual(readComposerDocument(editor), {
        text: 'Make [1]this area green',
        markIds: ['mark-1'],
      })
      const chip = editor.querySelector<HTMLElement>('[role="button"]')
      assert.ok(chip)
      await act(async () => chip.click())
      await act(async () =>
        chip.dispatchEvent(
          new KeyboardEvent('keydown', {
            key: ' ',
            bubbles: true,
            cancelable: true,
          })
        )
      )
      assert.deepEqual(opened, ['mark-1', 'mark-1'])
      assert.equal(sent, undefined)
      await act(async () => root.render(render(true)))
      assert.equal(container.querySelector('[role="textbox"]'), editor)
      assert.equal(readComposerDocument(editor).text, 'Make [1]this area green')
      await act(async () =>
        editor.dispatchEvent(
          new KeyboardEvent('keydown', {
            key: 'Enter',
            bubbles: true,
            cancelable: true,
          })
        )
      )
      assert.deepEqual(sent, {
        text: 'Make [1]this area green',
        ids: ['mark-1'],
      })
      assert.equal(editor.textContent, '')
    } finally {
      await act(async () => root.unmount())
      container.remove()
    }
  })

  test('removing a mark preserves ordinary text and removes its reference from submission', async () => {
    const editor = document.createElement('div')
    editor.innerHTML =
      'Move <span contenteditable="false" data-mark-id="old" data-mark-number="2">2</span> to the left'
    assert.deepEqual(readComposerDocument(editor), {
      text: 'Move [2] to the left',
      markIds: ['old'],
    })
    editor.querySelector('span')?.remove()
    assert.deepEqual(readComposerDocument(editor), {
      text: 'Move  to the left',
      markIds: [],
    })
  })
})
