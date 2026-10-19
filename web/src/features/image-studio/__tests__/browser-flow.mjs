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
import { mkdir } from 'node:fs/promises'

/** Run against the real app in an isolated browser context; all model/auth APIs are synthetic. */
export async function runImageStudioBrowserFlow(
  browser,
  baseURL,
  outputDirectory
) {
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
  })
  const page = await context.newPage()
  const errors = [],
    requests = []
  let chatCalls = 0,
    refreshCalls = 0
  let separateImageKey = false
  page.on('pageerror', (error) => errors.push(error.message))
  await mkdir(outputDirectory, { recursive: true })
  try {
    const png = await page.evaluate(() => {
      const canvas = document.createElement('canvas')
      canvas.width = 640
      canvas.height = 480
      const ctx = canvas.getContext('2d')
      ctx.fillStyle = '#e9f1ea'
      ctx.fillRect(0, 0, 640, 480)
      ctx.fillStyle = '#526c57'
      ctx.fillRect(80, 90, 200, 180)
      ctx.fillStyle = '#d9b481'
      ctx.fillRect(350, 220, 220, 180)
      ctx.fillStyle = '#222'
      ctx.font = '24px sans-serif'
      ctx.fillText('UI test image', 30, 40)
      return canvas.toDataURL('image/png').split(',')[1]
    })
    const user = {
      id: 7001,
      username: 'studio-test',
      display_name: 'Studio test',
      role: 1,
      group: 'default',
    }
    const session = {
      sid: 'studio-isolated-test',
      current: true,
      login_method: 'password',
      ip: '127.0.0.1',
      user_agent: 'test',
      created_at: 1,
      last_active_at: 1,
      expires_at: 4102444800,
    }
    await context.addInitScript(() => {
      localStorage.setItem('i18nextLng', 'en')
      localStorage.setItem('setup_status_checked', 'true')
    })
    await context.route('**/api/**', async (route) => {
      const path = new URL(route.request().url()).pathname
      let data = {}
      if (path === '/api/user/auth/refresh') {
        refreshCalls += 1
        data = {
          access_token: 'dashboard-test-jwt',
          token_type: 'Bearer',
          access_expires_at: 4102444800,
          user,
          session,
        }
      } else if (path === '/api/status') {
        data = {
          system_name: 'X-LLM',
          logo: '',
          HeaderNavModules: JSON.stringify({
            home: true,
            console: true,
            pricing: { enabled: true, requireAuth: false },
            rankings: false,
            docs: true,
            about: false,
          }),
        }
      } else if (path === '/api/setup') data = { status: true }
      else if (path === '/api/user/self') data = user
      else if (path === '/api/token/') {
        data = {
          items: [
            {
              id: 17,
              name: 'Studio test key',
              status: 1,
              expired_time: -1,
              unlimited_quota: true,
              group: 'default',
            },
            ...(separateImageKey
              ? [
                  {
                    id: 18,
                    name: 'Image channel key',
                    status: 1,
                    expired_time: -1,
                    unlimited_quota: true,
                    group: 'default',
                  },
                ]
              : []),
          ],
          total: separateImageKey ? 2 : 1,
        }
      } else if (path === '/api/token/17/key') {
        assert.equal(
          route.request().headers().authorization,
          'Bearer dashboard-test-jwt'
        )
        data = { key: 'selected-model-key' }
      } else if (path === '/api/token/18/key') {
        data = { key: 'separate-image-key' }
      } else if (/notice/.test(path)) data = ''
      else if (/announcement/.test(path)) data = []
      await route.fulfill({ json: { success: true, data } })
    })
    await context.route('**/v1/**', async (route) => {
      const request = route.request(),
        path = new URL(request.url()).pathname
      requests.push(path)
      try {
        if (
          path === '/v1/models' &&
          request.headers().authorization === 'Bearer sk-separate-image-key'
        ) {
          return await route.fulfill({
            json: { data: [{ id: 'gpt-image-special' }] },
          })
        }
        assert.equal(
          request.headers().authorization,
          'Bearer sk-selected-model-key'
        )
        if (path === '/v1/models') {
          return await route.fulfill({
            json: {
              data: [{ id: 'test-vision-chat' }, { id: 'gpt-image-2.5' }],
            },
          })
        }
        if (path === '/v1/chat/completions') {
          chatCalls += 1
          const payload = request.postDataJSON()
          assert.equal(payload.model, 'test-vision-chat')
          if (chatCalls === 5) {
            return await route.fulfill({
              status: 401,
              json: { error: { message: 'Token expired in test' } },
            })
          }
          if (chatCalls % 2 === 0) {
            return await route.fulfill({
              json: {
                choices: [{ message: { content: 'Your image is ready.' } }],
              },
            })
          }
          let tool = {
            name: 'generate_image',
            arguments: JSON.stringify({
              prompt: 'A quiet garden',
              size: '1024x1024',
            }),
          }
          if (chatCalls === 3) {
            const visualContext = payload.messages.at(-1).content
            const id = visualContext[0].text.match(
              /Current image: ([\w-]+)/
            )?.[1]
            assert.ok(id)
            assert.match(
              visualContext.map((part) => part.text ?? '').join('\n'),
              /Mark \[1\]/
            )
            assert.ok(visualContext.some((part) => part.type === 'image_url'))
            tool = {
              name: 'edit_image',
              arguments: JSON.stringify({
                prompt: 'Make the marked area blue',
                source_image_id: id,
              }),
            }
          }
          return await route.fulfill({
            json: {
              choices: [
                {
                  message: {
                    content: null,
                    tool_calls: [
                      {
                        id: `call-${chatCalls}`,
                        type: 'function',
                        function: tool,
                      },
                    ],
                  },
                },
              ],
            },
          })
        }
        if (path === '/v1/images/edits') {
          const body = request.postDataBuffer()
          assert.match(
            body.toString('latin1'),
            /name="size"\r\n\r\n640x480\r\n/,
            'a detail edit must keep the original dimensions when the model omits size'
          )
          assert.match(body.toString('latin1'), /Preserve.*640x480/)
          assert.ok(
            body.includes(Buffer.from(png, 'base64')),
            'edit must contain the exact original bytes'
          )
          assert.equal(
            (body.toString('latin1').match(/name="image\[\]"/g) ?? []).length,
            2,
            'edit must include original and location guide'
          )
          assert.match(body.toString('latin1'), /location guide/)
        }
        return await route.fulfill({ json: { data: [{ b64_json: png }] } })
      } catch (error) {
        errors.push(error.message)
        return await route.fulfill({
          status: 500,
          json: { error: { message: error.message } },
        })
      }
    })
    await page.goto(`${baseURL}/image-studio`)
    const editor = page.getByRole('textbox', { name: 'Describe your image' })
    await editor.waitFor()
    await page.waitForFunction(() =>
      document.querySelector('main')?.textContent?.includes('gpt-image-2.5')
    )
    assert.equal(
      await page.getByRole('region', { name: 'Image preview' }).count(),
      0
    )
    const nav = await page.locator('header nav a').allTextContents()
    assert.equal(
      await page
        .locator('main > header')
        .getByText('Image creation', { exact: true })
        .count(),
      0
    )
    assert.ok(nav.indexOf('Image creation') > nav.indexOf('Model Square'))
    assert.ok(nav.indexOf('Image creation') < nav.indexOf('Docs'))
    await page.screenshot({ path: `${outputDirectory}/empty.png` })
    await editor.fill('Draw a quiet garden')
    await page.getByRole('button', { name: 'Send', exact: true }).click()
    await page.getByText('Your image is ready.', { exact: true }).waitFor()
    const preview = page.getByRole('region', { name: 'Image preview' })
    await preview.waitFor()
    const image = preview.locator('img').first()
    await image.waitFor()
    await editor.fill('Make this area blue')
    await editor.press('Home')
    await editor.press('ArrowRight')
    const surface = preview.locator('[data-dazi-context-menu="none"]')
    const box = await surface.boundingBox()
    assert.ok(box)
    const mainBox = await page.locator('main').boundingBox()
    assert.ok(
      (await preview.boundingBox()).width >= mainBox.width * 0.66,
      'image panel should occupy at least two thirds of the desktop workspace'
    )
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
    await page.mouse.wheel(0, -120)
    await page.waitForFunction(
      () =>
        document.querySelector('[title="Fit image"]')?.textContent !== '100%'
    )
    let imageBox = await image.boundingBox()
    assert.ok(imageBox)
    const target = {
      x: imageBox.x + imageBox.width * 0.62,
      y: imageBox.y + imageBox.height * 0.58,
    }
    await page.mouse.click(target.x, target.y)
    await page
      .getByRole('button', { name: 'Remove mark 1', exact: true })
      .waitFor()
    const markButton = page.getByRole('button', {
      name: 'Remove mark 1',
      exact: true,
    })
    const markBox = await markButton.boundingBox()
    const badgeBox = await markButton
      .locator('..')
      .locator('span')
      .boundingBox()
    assert.ok(
      markBox.x >= badgeBox.x + badgeBox.width ||
        markBox.y + markBox.height <= badgeBox.y,
      'delete control must not cover the numbered mark'
    )
    const draft = await editor.textContent()
    assert.ok(draft.includes('1'))
    await page
      .getByRole('button', { name: 'Collapse conversation', exact: true })
      .click()
    assert.equal(await editor.textContent(), draft)
    await page
      .getByRole('button', { name: 'Expand conversation', exact: true })
      .first()
      .click()
    assert.equal(await editor.textContent(), draft)
    await page.waitForFunction(
      ({ x, width }) => {
        const bounds = document
          .querySelector('[aria-label="Image preview"] img')
          ?.getBoundingClientRect()
        return (
          bounds &&
          Math.abs(bounds.x - x) < 1 &&
          Math.abs(bounds.width - width) < 1
        )
      },
      { x: imageBox.x, width: imageBox.width }
    )
    imageBox = await image.boundingBox()
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
    await page.mouse.down({ button: 'right' })
    await page.mouse.move(
      box.x + box.width / 2 + 40,
      box.y + box.height / 2 + 20
    )
    await page.mouse.up({ button: 'right' })
    const movedBox = await image.boundingBox()
    assert.ok(Math.abs(movedBox.x - imageBox.x - 40) < 2)
    const draftReference = editor.getByRole('button', {
      name: 'Image reference 1',
      exact: true,
    })
    await draftReference.click()
    await page.waitForFunction(
      () =>
        document.querySelector('[title="Fit image"]')?.textContent === '100%'
    )
    assert.equal(await editor.textContent(), draft)
    assert.equal(
      await page.getByRole('button', { name: /^Remove mark/ }).count(),
      1
    )
    await page.screenshot({ path: `${outputDirectory}/marked.png` })
    await page.getByRole('button', { name: 'Send', exact: true }).click()
    await page.waitForFunction(
      () =>
        [...document.querySelectorAll('article')].filter(
          (node) => node.textContent === 'Your image is ready.'
        ).length >= 1 && !document.querySelector('main [role="status"]')
    )
    await page.waitForFunction(
      () =>
        document.querySelector('[aria-label="Images and versions"]')?.children
          .length === 2
    )
    assert.equal(
      await page.getByRole('button', { name: /^Remove mark/ }).count(),
      0
    )
    assert.equal(
      requests.filter((path) => path === '/v1/images/generations').length,
      1
    )
    assert.equal(
      requests.filter((path) => path === '/v1/images/edits').length,
      1
    )
    await page.screenshot({ path: `${outputDirectory}/result.png` })
    // Historical references open their original version without entering the draft.
    await editor.fill('Keep my draft [99]')
    const sentReference = page
      .getByRole('log')
      .getByRole('button', { name: 'Image reference 1', exact: true })
    await sentReference.focus()
    await sentReference.press('Enter')
    await page
      .getByRole('button', { name: 'Hide mark 1', exact: true })
      .waitFor()
    const versions = page.getByLabel('Images and versions')
    assert.equal(
      await versions.getByRole('button').first().getAttribute('aria-pressed'),
      'true'
    )
    assert.equal(await editor.textContent(), 'Keep my draft [99]')
    assert.equal(await editor.locator('[data-mark-id]').count(), 0)
    assert.equal(
      await page.getByRole('button', { name: /^Remove mark/ }).count(),
      0
    )
    await page.screenshot({ path: `${outputDirectory}/reference.png` })
    await page.getByRole('button', { name: 'Hide mark 1', exact: true }).click()
    await sentReference.click()
    await page
      .getByRole('button', { name: 'Hide mark 1', exact: true })
      .waitFor()
    await versions.getByRole('button').last().click()
    assert.equal(
      await page
        .getByRole('button', { name: 'Hide mark 1', exact: true })
        .count(),
      0
    )
    const previousRefreshes = refreshCalls
    await editor.fill('Discuss the composition [99]')
    await page.getByRole('button', { name: 'Send', exact: true }).click()
    await page.getByText('Token expired in test', { exact: true }).waitFor()
    await page
      .getByRole('log')
      .getByText('Discuss the composition [99]', { exact: true })
      .waitFor()
    assert.equal(
      await page
        .getByRole('button', { name: 'Image reference 99', exact: true })
        .count(),
      0
    )
    assert.equal(refreshCalls, previousRefreshes)
    assert.ok(page.url().includes('/image-studio'))
    // Settings must present the same full-width field layout for keys and models.
    await page
      .getByRole('button', { name: 'Creation settings', exact: true })
      .click()
    const dialog = page.getByRole('dialog', { name: 'Creation settings' })
    await dialog.evaluate(async (element) => {
      await Promise.all(
        element
          .getAnimations({ subtree: true })
          .map((animation) => animation.finished)
      )
    })
    assert.ok(
      !(await dialog.textContent()).includes('default'),
      'legacy group identifiers must not be presented as routing information'
    )
    for (const name of ['Chat model', 'Image model']) {
      const field = dialog.getByRole('group', { name, exact: true })
      const labelBox = await field.locator('span').first().boundingBox()
      const selectBox = await field.getByRole('combobox').boundingBox()
      const fieldBox = await field.boundingBox()
      assert.ok(
        selectBox.y >= labelBox.y + labelBox.height,
        'model control must be below its label'
      )
      assert.ok(
        Math.abs(selectBox.width - fieldBox.width) < 2,
        'model control must use full field width'
      )
    }
    separateImageKey = true
    await dialog.getByRole('button', { name: 'Refresh models' }).click()
    await dialog.getByLabel('Image API key', { exact: true }).selectOption('18')
    await dialog
      .getByRole('group', { name: 'Image model', exact: true })
      .getByRole('combobox')
      .getByText('gpt-image-special', { exact: true })
      .waitFor()
    assert.ok(
      (
        await dialog
          .getByRole('group', { name: 'Chat model', exact: true })
          .textContent()
      ).includes('test-vision-chat')
    )
    await page.setViewportSize({ width: 390, height: 844 })
    await dialog
      .getByRole('group', { name: 'Image model', exact: true })
      .getByRole('combobox')
      .locator('span')
      .first()
      .waitFor({ state: 'visible' })
    await page.screenshot({ path: `${outputDirectory}/settings-mobile.png` })
    await page.setViewportSize({ width: 1440, height: 1000 })
    await page.screenshot({ path: `${outputDirectory}/settings.png` })
    await dialog.getByRole('button', { name: 'Done', exact: true }).click()
    await page
      .getByRole('button', { name: 'New creation', exact: true })
      .click()
    await page
      .getByRole('button', { name: 'Creation history', exact: true })
      .click()
    await page.getByRole('button', { name: /Draw a quiet garden/ }).click()
    await page.waitForFunction(
      () =>
        document.querySelector('[aria-label="Images and versions"]')?.children
          .length === 2
    )
    const stored = await page.evaluate(async () => {
      const db = await new Promise((resolve, reject) => {
        const request = indexedDB.open('xllm-image-studio')
        request.onsuccess = () => resolve(request.result)
        request.onerror = () => reject(request.error)
      })
      const records = await new Promise((resolve, reject) => {
        const request = db.transaction('images').objectStore('images').getAll()
        request.onsuccess = () => resolve(request.result)
        request.onerror = () => reject(request.error)
      })
      const conversations = await new Promise((resolve, reject) => {
        const request = db
          .transaction('conversations')
          .objectStore('conversations')
          .getAll()
        request.onsuccess = () => resolve(request.result)
        request.onerror = () => reject(request.error)
      })
      const mark = conversations[0].messages.find(
        (message) => message.marks?.length
      )?.marks[0]
      db.close()
      return {
        mark,
        count: records.length,
        blobs: records.every((image) => image.blob instanceof Blob),
        scopes: records.map((image) => image.scope),
        parents: records.map((image) => image.parentId),
        storage: JSON.stringify(localStorage),
      }
    })
    assert.ok(
      Math.abs(stored.mark.region.x - 0.62) < 0.005 &&
        Math.abs(stored.mark.region.y - 0.58) < 0.005,
      'mark coordinates must match the zoomed original'
    )
    assert.equal(stored.count, 2)
    assert.equal(stored.blobs, true)
    assert.ok(stored.scopes.every((scope) => scope.endsWith(':7001')))
    assert.ok(stored.parents.some(Boolean))
    assert.ok(!stored.storage.includes('selected-model-key'))
    // A provider may ignore size. Keep its result, surface the mismatch and stop.
    const squarePng = await page.evaluate(() => {
      const canvas = document.createElement('canvas')
      canvas.width = canvas.height = 512
      return canvas.toDataURL('image/png').split(',')[1]
    })
    let warningChatCalls = 0
    await context.route('**/v1/chat/completions', async (route) => {
      warningChatCalls += 1
      const content = route.request().postDataJSON().messages.at(-1).content
      const sourceId = content[0].text.match(/Current image: ([\w-]+)/)?.[1]
      await route.fulfill({
        json: {
          choices: [
            {
              message: {
                content: null,
                tool_calls: [
                  {
                    id: 'aspect-check',
                    type: 'function',
                    function: {
                      name: 'edit_image',
                      arguments: JSON.stringify({
                        prompt: 'Add another flower',
                        source_image_id: sourceId,
                      }),
                    },
                  },
                ],
              },
            },
          ],
        },
      })
    })
    await context.route('**/v1/images/edits', (route) =>
      route.fulfill({ json: { data: [{ b64_json: squarePng }] } })
    )
    await editor.fill('Add another flower')
    await page.getByRole('button', { name: 'Send', exact: true }).click()
    const warning = page
      .getByRole('log')
      .getByRole('status')
      .filter({ hasText: 'aspect ratio' })
    await warning.waitFor()
    assert.match(await warning.textContent(), /512×512.*640×480/)
    await page
      .getByRole('button', { name: 'Stop', exact: true })
      .waitFor({ state: 'hidden' })
    assert.equal(warningChatCalls, 1)
    assert.equal(
      await page.getByLabel('Images and versions').getByRole('button').count(),
      3
    )
    await page.screenshot({ path: `${outputDirectory}/aspect-warning.png` })
    await page
      .getByRole('button', { name: 'New creation', exact: true })
      .click()
    await page
      .getByText('Saved in this browser', { exact: true })
      .waitFor({ state: 'hidden' })
    await page.setViewportSize({ width: 390, height: 844 })
    assert.ok(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth
      )
    )
    await page.screenshot({ path: `${outputDirectory}/mobile.png` })
    // Reload without a session hint must still recover the authenticated user.
    user.id = 7002
    await page.reload()
    await editor.waitFor()
    assert.equal(await page.getByLabel('Images and versions').count(), 0)
    await page
      .getByRole('button', { name: 'Creation history', exact: true })
      .click()
    assert.equal(
      await page.getByRole('button', { name: /Draw a quiet garden/ }).count(),
      0
    )
    user.id = 7001
    await page.reload()
    await editor.waitFor()
    await page
      .getByRole('button', { name: 'Creation history', exact: true })
      .click()
    await page.getByRole('button', { name: /Draw a quiet garden/ }).waitFor()

    const anonymous = await browser.newContext()
    try {
      await anonymous.route('**/api/**', (route) => {
        const pathname = new URL(route.request().url()).pathname
        if (pathname === '/api/user/auth/refresh') {
          return route.fulfill({ status: 401, json: { success: false } })
        }
        return route.fulfill({
          json: {
            success: true,
            data: pathname === '/api/setup' ? { status: true } : {},
          },
        })
      })
      const anonymousPage = await anonymous.newPage()
      await anonymousPage.goto(baseURL + '/image-studio')
      await anonymousPage.waitForURL('**/sign-in?redirect=*')
      assert.equal(
        await anonymousPage
          .getByRole('textbox', { name: 'Describe your image' })
          .count(),
        0
      )
    } finally {
      await anonymous.close()
    }
    assert.deepEqual(errors, [])
    return {
      passed: [
        'navigation order',
        'session recovery without a hint cookie',
        'anonymous access redirects to sign-in',
        'account-isolated history',
        'chat-only entry',
        'selected-key auth',
        'generation',
        'zoom and mark',
        'draft survives collapse',
        'right-button pan',
        'original plus guide edit',
        'consumed markers',
        'clickable draft and historical references',
        '401 keeps login',
        'local Blob history and versions',
        'original edit dimensions and visible aspect mismatch',
        'mobile layout',
      ],
      screenshots: outputDirectory,
    }
  } finally {
    await context.close()
  }
}
