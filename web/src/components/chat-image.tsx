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
// 对话内图片的操作层（T3/T5）：点击放大 + 下载 + 用作参考图。
//
// ## 为什么需要新增（能力缺口说明）
//
// `web/AGENTS.md` 要求「新增替代实现前必须说明候选组件缺少的具体能力」。
// 已检索 `src/components/` 与 `src/features/`：
//   - `response-renderer-image.tsx` 只渲染 `<img>` + `onError` 降级，
//     **没有**放大、下载、复用为参考图的能力；
//   - 全仓 grep `lightbox|ImageLightbox|image-viewer` **零命中**；
//   - `Dialog`（`@/components/ui/dialog`）是通用弹层，不含图片查看器的
//     缩放/下载/复用语义，单独用它仍要自己写这一整套交互。
// 因此这里新增一个**通用**图片操作组件（不绑定游乐场业务），
// 内部复用既有 `Dialog` 与 `Button`，不手写弹层。
import { useState } from 'react'
import { Download, ImagePlus } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from '@/components/ui/dialog'
import { guessImageFileName } from '@/lib/image-file-name'
import { cn } from '@/lib/utils'

export type ChatImageProps = {
  src: string
  alt?: string
  /** 外层容器类（控制布局位置）。 */
  className?: string
  /** 图片本身的尺寸类。不同位置差异很大：附件条要小、markdown 正文要大，
   *  因此不写死尺寸，交给调用方（否则会把正文图片压成缩略图大小）。 */
  imgClassName?: string
  /**
   * 「用作参考图」回调。不传则不显示该按钮——文本模型没有图生图能力，
   * 给它显示一个点了没用的按钮是更糟的体验。
   */
  onUseAsReference?: (src: string) => void
  /** 下载文件名（省略时用 URL 末段）。 */
  downloadName?: string
}


export function ChatImage(props: ChatImageProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [failed, setFailed] = useState(false)

  // 图片加载失败时给文本占位而不是碎图——与 `response-renderer-image.tsx`
  // 的既有降级策略保持一致。
  if (failed) {
    return (
      <span className='border-border/70 text-muted-foreground inline-flex rounded-md border px-3 py-2 text-xs italic'>
        {props.alt || t('Image not available')}
      </span>
    )
  }

  return (
    <>
      <div className={cn('group/img relative', props.className)}>
        <button
          type='button'
          // 整张图可点放大：用 button 而不是给 img 挂 onClick，
          // 保证键盘可达（Enter/Space）且有可访问名称。
          aria-label={t('View image')}
          onClick={() => setOpen(true)}
          className='block w-full cursor-zoom-in focus-visible:ring-ring rounded-md focus-visible:ring-2 focus-visible:outline-none'
        >
          <img
            src={props.src}
            alt={props.alt || t('Image')}
            loading='lazy'
            onError={() => setFailed(true)}
            className={cn(
              'border-border/70 w-auto rounded-md border object-contain',
              props.imgClassName ?? 'max-h-40'
            )}
          />
        </button>
        {/* 操作按钮 hover / 键盘聚焦出现，不遮挡图片本身。 */}
        <div className='absolute top-1 right-1 flex gap-1 opacity-0 transition-opacity group-hover/img:opacity-100 group-focus-within/img:opacity-100'>
          <Button
            size='icon'
            variant='secondary'
            className='size-7'
            aria-label={t('Download image')}
            render={
              <a
                href={props.src}
                download={props.downloadName || guessImageFileName(props.src)}
                target='_blank'
                rel='noreferrer'
              />
            }
          >
            <Download className='size-3.5' />
          </Button>
          {props.onUseAsReference && (
            <Button
              size='icon'
              variant='secondary'
              className='size-7'
              aria-label={t('Use as reference image')}
              onClick={() => props.onUseAsReference?.(props.src)}
            >
              <ImagePlus className='size-3.5' />
            </Button>
          )}
        </div>
      </div>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className='max-w-[min(92vw,1200px)] p-2 sm:max-w-[min(92vw,1200px)]'>
          {/* Radix/Base UI 的 Dialog 要求标题存在（可访问性）；视觉上隐藏，
              因为图片本身就是内容。 */}
          <DialogTitle className='sr-only'>{props.alt || t('Image')}</DialogTitle>
          <DialogDescription className='sr-only'>
            {t('Enlarged image preview')}
          </DialogDescription>
          <img
            src={props.src}
            alt={props.alt || t('Image')}
            className='mx-auto max-h-[80vh] w-auto rounded-md object-contain'
          />
          <div className='flex justify-center gap-2 pt-1'>
            <Button
              variant='outline'
              size='sm'
              render={
                <a
                  href={props.src}
                  download={props.downloadName || guessImageFileName(props.src)}
                  target='_blank'
                  rel='noreferrer'
                />
              }
            >
              <Download className='mr-1.5 size-3.5' />
              {t('Download')}
            </Button>
            {props.onUseAsReference && (
              <Button
                variant='outline'
                size='sm'
                onClick={() => {
                  props.onUseAsReference?.(props.src)
                  setOpen(false)
                }}
              >
                <ImagePlus className='mr-1.5 size-3.5' />
                {t('Use as reference image')}
              </Button>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}

