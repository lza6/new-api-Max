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
// P1-6 视频生成结果面板：进度 / 播放 / 失败原因 / 下载。
//
// 直接复用了平台约定的产物形态（视频 URL + 可下载），不新增存储层。
import { AlertCircle, Download, Loader2, Video } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'

import type { VideoGenerationState } from '../../hooks/use-video-generation'

type VideoResultPanelProps = {
  state: VideoGenerationState
  onReset: () => void
  onCancel: () => void
}

/** 把错误码/失败原因翻译成可展示文案（找不到对应键时原样展示上游原因）。 */
function describeFailure(
  state: VideoGenerationState,
  t: (key: string) => string
): string {
  if (state.error === 'no_task_id') {
    return t('The video service did not return a task id.')
  }
  if (state.error === 'submit_failed') {
    return t('Failed to submit the video generation request.')
  }
  if (state.error === 'poll_failed') {
    return t('Lost connection while waiting for the video.')
  }
  if (state.failReason === 'timeout') {
    return t('Timed out waiting for the video. Please try again later.')
  }
  return state.failReason ?? t('Video generation failed.')
}

export function VideoResultPanel(props: VideoResultPanelProps) {
  const { t } = useTranslation()
  const state = props.state

  if (state.status === 'idle') {
    return null
  }

  if (state.isGenerating) {
    return (
      <section
        aria-label={t('Video generation')}
        className='bg-card mx-auto w-full max-w-4xl rounded-2xl border p-4'
      >
        <div className='flex items-center gap-2 text-sm font-medium'>
          <Loader2 className='size-4 animate-spin' />
          {t('Generating video…')}
        </div>
        <p className='text-muted-foreground mt-1 text-xs'>
          {t('Video generation can take a few minutes. You can keep this tab open.')}
        </p>
        <Progress
          className='mt-3'
          value={state.progress ?? null}
          aria-label={t('Progress')}
        />
        <div className='mt-3 flex justify-end'>
          <Button variant='outline' size='sm' onClick={props.onCancel}>
            {t('Cancel')}
          </Button>
        </div>
      </section>
    )
  }

  if (state.status === 'failed') {
    return (
      <section
        role='alert'
        aria-label={t('Video generation')}
        className='border-destructive/40 bg-destructive/5 mx-auto w-full max-w-4xl rounded-2xl border p-4'
      >
        <div className='text-destructive flex items-center gap-2 text-sm font-medium'>
          <AlertCircle className='size-4' />
          {t('Video generation failed')}
        </div>
        <p className='text-muted-foreground mt-1 text-xs break-words'>
          {describeFailure(state, t)}
        </p>
        <div className='mt-3 flex justify-end'>
          <Button variant='outline' size='sm' onClick={props.onReset}>
            {t('Dismiss')}
          </Button>
        </div>
      </section>
    )
  }

  // 成功：有 URL 才可播放；无 URL 说明上游只回状态没回地址，如实说明。
  return (
    <section
      aria-label={t('Video generation')}
      className='bg-card mx-auto w-full max-w-4xl rounded-2xl border p-4'
    >
      <div className='flex items-center gap-2 text-sm font-medium'>
        <Video className='size-4' />
        {t('Video ready')}
      </div>
      {state.url ? (
        <>
          <video
            className='mt-3 w-full rounded-lg'
            src={state.url}
            controls
            preload='metadata'
            aria-label={t('Generated video')}
          />
          <div className='mt-3 flex justify-end gap-2'>
            <Button variant='outline' size='sm' render={<a href={state.url} download />}>
              <Download className='mr-1.5 size-3.5' />
              {t('Download')}
            </Button>
            <Button variant='ghost' size='sm' onClick={props.onReset}>
              {t('Dismiss')}
            </Button>
          </div>
        </>
      ) : (
        <>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t('The task finished but no video URL was returned.')}
          </p>
          <div className='mt-3 flex justify-end'>
            <Button variant='outline' size='sm' onClick={props.onReset}>
              {t('Dismiss')}
            </Button>
          </div>
        </>
      )}
    </section>
  )
}
