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
export interface StudioModel {
  id: string
  supported_endpoint_types?: string[]
}

export interface StudioSettings {
  chatTokenId: number | null
  imageTokenId: number | null
  chatModel: string
  imageModel: string
}

export interface ImageRegion {
  x: number
  y: number
  w: number
  h: number
}
export interface ImageMark {
  id: string
  number: number
  imageId: string
  region: ImageRegion
}

export interface StudioImage {
  id: string
  blob: Blob
  width: number
  height: number
  name: string
  createdAt: number
  parentId?: string
}

export interface StudioMessage {
  id: string
  role: 'user' | 'assistant'
  text: string
  imageIds: string[]
  marks: ImageMark[]
  error?: string
  warning?: string
}

export interface StudioConversation {
  id: string
  title: string
  updatedAt: number
  messages: StudioMessage[]
  imageIds: string[]
  selectedImageId?: string
}

export interface ChatToolCall {
  id: string
  type: 'function'
  function: { name: string; arguments: string }
}

export type ChatPart =
  | { type: 'text'; text: string }
  | {
      type: 'image_url'
      image_url: { url: string }
    }

export interface ChatMessage {
  role: 'system' | 'user' | 'assistant' | 'tool'
  content: string | ChatPart[] | null
  tool_calls?: ChatToolCall[]
  tool_call_id?: string
}

export type StudioStage = 'thinking' | 'generating' | 'editing'
