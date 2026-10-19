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
export function readComposerDocument(root: HTMLElement): {
  text: string
  markIds: string[]
} {
  const markIds: string[] = []
  const read = (node: Node): string => {
    if (node.nodeType === Node.TEXT_NODE) return node.textContent ?? ''
    if (!(node instanceof HTMLElement)) return ''
    if (node.dataset.markId) {
      markIds.push(node.dataset.markId)
      return `[${node.dataset.markNumber}]`
    }
    if (node.tagName === 'BR') return '\n'
    const content = [...node.childNodes].map(read).join('')
    return node !== root && ['DIV', 'P'].includes(node.tagName)
      ? `${content}\n`
      : content
  }
  return { text: read(root).trim(), markIds: [...new Set(markIds)] }
}
