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
import { Loader2, Play, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import type { KeyTestResult, MultiKeyConfirmAction } from '../../types'

type MultiKeyTableRowActionsProps = {
  keyIndex: number
  status: number
  canDelete: boolean
  onAction: (action: MultiKeyConfirmAction) => void
  /** 测试该 key 的回调；传 undefined 表示不展示测试按钮。 */
  onTest?: (keyIndex: number) => void
  /** 该 key 的最近一次测试结果（用于展示绿/红点）。 */
  testResult?: KeyTestResult
  /** 是否正在测试该 key。 */
  testing?: boolean
}

export function MultiKeyTableRowActions({
  keyIndex,
  status,
  canDelete,
  onAction,
  onTest,
  testResult,
  testing,
}: MultiKeyTableRowActionsProps) {
  const { t } = useTranslation()
  const isEnabled = status === 1

  // 状态标签：未测试 → "Test"，已测试 → OK/FAIL
  let testLabel = t('Test')
  let testTone: string | undefined
  if (testResult) {
    testLabel = testResult.ok ? t('OK') : t('FAIL')
    testTone = testResult.ok ? 'text-success' : 'text-destructive'
  }

  return (
    <div className='flex justify-end gap-2'>
      {onTest && (
        <Button
          variant='outline'
          size='sm'
          onClick={() => onTest(keyIndex)}
          disabled={testing}
          title={t('Test this key')}
        >
          {testing ? (
            <Loader2 className='h-4 w-4 animate-spin' />
          ) : (
            <Play className='h-4 w-4' />
          )}
          <span className={testTone}>{testLabel}</span>
        </Button>
      )}
      {isEnabled ? (
        <Button
          variant='outline'
          size='sm'
          onClick={() => onAction({ type: 'disable', keyIndex })}
        >
          {t('Disable')}
        </Button>
      ) : (
        <Button
          variant='outline'
          size='sm'
          onClick={() => onAction({ type: 'enable', keyIndex })}
        >
          {t('Enable')}
        </Button>
      )}
      <Button
        variant='destructive'
        size='sm'
        onClick={() => {
          if (!canDelete) {return}
          onAction({ type: 'delete', keyIndex })
        }}
        disabled={!canDelete}
        title={
          canDelete ? undefined : t('No permission to perform this action')
        }
      >
        {t('Delete')}
      </Button>
    </div>
  )
}

/** 批量测试入口按钮（放在对话框工具栏）。 */
export function TestAllKeysButton(props: {
  onTest: () => void
  testing: boolean
  disabled?: boolean
}) {
  const { t } = useTranslation()
  return (
    <Button
      variant='outline'
      size='sm'
      onClick={props.onTest}
      disabled={props.testing || props.disabled}
    >
      {props.testing ? (
        <Loader2 className='mr-2 h-4 w-4 animate-spin' />
      ) : (
        <Zap className='mr-2 h-4 w-4' />
      )}
      {t('Test All Keys')}
    </Button>
  )
}
