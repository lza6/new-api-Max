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
import { useTranslation } from 'react-i18next'
import { sanitizeImageSrc, type ImageNode } from 'stream-markdown-parser'

import { ChatImage } from '@/components/chat-image'
import { useUseAsReference } from '@/components/use-as-reference-context'

type ResponseImageProps = {
  node: ImageNode
}

export function ResponseImage(props: ResponseImageProps) {
  const { t } = useTranslation()
  // 游乐场通过 Context 声明「可把这张图送回输入框做二次改图」；其它调用方
  // 没有 Provider，读到 undefined → 不显示该按钮，行为零变化。
  const useAsReference = useUseAsReference()
  // 加载失败的降级交给 ChatImage（它内部同样渲染文本占位），这里只需判空 src
  // ——留一份 `hasError` 状态会让「谁负责降级」出现两个真相。
  const src = sanitizeImageSrc(props.node.src)

  if (!src) {
    return (
      <span className='border-border/70 text-muted-foreground my-4 inline-flex rounded-md border px-3 py-2 text-xs italic'>
        {props.node.alt || t('Image not available')}
      </span>
    )
  }

  // 复用统一的图片操作层（点击放大 + 下载），而不是只渲染一个裸 `<img>`：
  // 生成图在正文里以 markdown 形式返回，用户最常想做的就是「存下来」，
  // 原先整个应用没有任何图片下载入口。
  // 尺寸仍由本组件决定（`imgClassName`），保持既有正文排版不变。
  return (
    <span className='my-4 block'>
      <ChatImage
        src={src}
        alt={props.node.alt}
        imgClassName='max-h-96 max-w-full rounded-lg'
        onUseAsReference={useAsReference}
      />
    </span>
  )
}
