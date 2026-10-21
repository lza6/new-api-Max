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
import { Loader2 } from 'lucide-react'
import * as React from 'react'
import { useTranslation } from 'react-i18next'

import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

type ConfirmDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: React.ReactNode
  disabled?: boolean
  desc: React.JSX.Element | string
  cancelBtnText?: string
  confirmText?: React.ReactNode
  destructive?: boolean
  handleConfirm: () => void
  isLoading?: boolean
  className?: string
  children?: React.ReactNode
  /**
   * L3 破坏性操作：要求用户逐字输入这段文本才能确认（计费/价格类改动）。
   * 未输入或输入不符时确认按钮禁用。
   */
  requireTypedText?: string
}

export function ConfirmDialog(props: ConfirmDialogProps) {
  const { t } = useTranslation()
  const {
    title,
    desc,
    children,
    className,
    confirmText,
    cancelBtnText,
    destructive,
    isLoading,
    disabled = false,
    requireTypedText,
    handleConfirm,
    ...actions
  } = props
  const [typed, setTyped] = React.useState('')
  React.useEffect(() => {
    if (!props.open) {setTyped('')}
  }, [props.open])
  const typedOk = !requireTypedText || typed === requireTypedText
  return (
    <AlertDialog {...actions}>
      <AlertDialogContent className={cn(className)}>
        <AlertDialogHeader className='text-start'>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription render={<div />}>
            {desc}
          </AlertDialogDescription>
        </AlertDialogHeader>
        {requireTypedText && (
          <div className='grid gap-1.5'>
            <label className='text-muted-foreground text-xs' htmlFor='confirm-typed'>
              {t('Type {{text}} to confirm', { text: requireTypedText })}
            </label>
            <Input
              id='confirm-typed'
              autoComplete='off'
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              disabled={isLoading}
            />
          </div>
        )}
        {children}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isLoading}>
            {cancelBtnText ?? t('Cancel')}
          </AlertDialogCancel>
          <Button
            variant={destructive ? 'destructive' : 'default'}
            onClick={handleConfirm}
            disabled={disabled || isLoading || !typedOk}
          >
            {isLoading && (
              <Loader2 className='mr-1.5 size-4 animate-spin' aria-hidden='true' />
            )}
            {confirmText ?? t('Continue')}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
