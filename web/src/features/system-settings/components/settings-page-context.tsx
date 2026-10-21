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
import { RotateCcw, Save } from 'lucide-react'
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useId,
  useMemo,
  useState,
  type ComponentProps,
  type ReactNode,
  type RefObject,
} from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { FormNavigationGuard } from './form-navigation-guard'

type SettingsPageContextValue = {
  actionsContainer: HTMLDivElement | null
  titleStatusContainer: HTMLSpanElement | null
  suppressSectionHeader: boolean
  /**
   * 各 section 的 `useSettingsForm` 通过它把 dirty 状态登记上来，页面级只渲染
   * 一个 `FormNavigationGuard`。这样新增 section 不会漏掉「未保存离开」保护——
   * 之前 42 处 `SettingsPageFormActions` 里只有 7 个文件手挂了 guard。
   */
  registerDirty: (id: string, dirty: boolean) => void
}

const SettingsPageContext = createContext<SettingsPageContextValue>({
  actionsContainer: null,
  titleStatusContainer: null,
  suppressSectionHeader: false,
  registerDirty: () => undefined,
})

/** 把某个表单的 dirty 状态登记到页面级 guard（在 Provider 外调用是 no-op）。 */
export function useSettingsDirtyRegistration(isDirty: boolean) {
  const { registerDirty } = useContext(SettingsPageContext)
  const id = useId()
  useEffect(() => {
    registerDirty(id, isDirty)
    return () => registerDirty(id, false)
  }, [id, isDirty, registerDirty])
}

type SettingsPageProviderProps = {
  actionsContainer: HTMLDivElement | null
  titleStatusContainer?: HTMLSpanElement | null
  children: ReactNode
  suppressSectionHeader?: boolean
}

export function SettingsPageProvider(props: SettingsPageProviderProps) {
  const [dirtyById, setDirtyById] = useState<Record<string, boolean>>({})
  const registerDirty = useCallback((id: string, dirty: boolean) => {
    setDirtyById((previous) =>
      previous[id] === dirty ? previous : { ...previous, [id]: dirty }
    )
  }, [])
  const anyDirty = useMemo(
    () => Object.values(dirtyById).some(Boolean),
    [dirtyById]
  )
  const value = useMemo(
    () => ({
      actionsContainer: props.actionsContainer,
      titleStatusContainer: props.titleStatusContainer ?? null,
      suppressSectionHeader: props.suppressSectionHeader ?? true,
      registerDirty,
    }),
    [
      props.actionsContainer,
      props.titleStatusContainer,
      props.suppressSectionHeader,
      registerDirty,
    ]
  )

  return (
    <SettingsPageContext.Provider value={value}>
      {/* 页面级唯一的未保存离开保护：覆盖所有使用 useSettingsForm 的 section。 */}
      <FormNavigationGuard when={anyDirty} />
      {props.children}
    </SettingsPageContext.Provider>
  )
}

export function useSuppressSettingsSectionHeader() {
  return useContext(SettingsPageContext).suppressSectionHeader
}

type SettingsPageTitleStatusPortalProps = {
  children: ReactNode
}

export function SettingsPageTitleStatusPortal(
  props: SettingsPageTitleStatusPortalProps
) {
  const { titleStatusContainer } = useContext(SettingsPageContext)

  if (!titleStatusContainer) {return null}

  return createPortal(props.children, titleStatusContainer)
}

type SettingsPageActionsPortalProps = {
  children: ReactNode
}

export function SettingsPageActionsPortal(
  props: SettingsPageActionsPortalProps
) {
  const { actionsContainer } = useContext(SettingsPageContext)

  if (!actionsContainer) {return null}

  return createPortal(
    <div className='flex flex-wrap items-center justify-end gap-2'>
      {props.children}
    </div>,
    actionsContainer
  )
}

type SettingsPageFormActionsProps = {
  onSave: () => void
  onReset?: () => void
  isSaving?: boolean
  isSaveDisabled?: boolean
  isResetDisabled?: boolean
  saveLabel?: string
  savingLabel?: string
  resetLabel?: string
  resetVariant?: ComponentProps<typeof Button>['variant']
  saveButtonRef?: RefObject<HTMLButtonElement | null>
}

export function SettingsPageFormActions(props: SettingsPageFormActionsProps) {
  const { t } = useTranslation()
  const saveLabel = props.isSaving
    ? (props.savingLabel ?? 'Saving...')
    : (props.saveLabel ?? 'Save Changes')

  return (
    <SettingsPageActionsPortal>
      {props.onReset && (
        <Button
          type='button'
          size='sm'
          variant={props.resetVariant ?? 'outline'}
          onClick={props.onReset}
          disabled={props.isResetDisabled || props.isSaving}
        >
          <RotateCcw data-icon='inline-start' />
          <span>{t(props.resetLabel ?? 'Reset')}</span>
        </Button>
      )}
      <Button
        ref={props.saveButtonRef}
        type='button'
        size='sm'
        onClick={props.onSave}
        disabled={props.isSaving || props.isSaveDisabled}
      >
        <Save data-icon='inline-start' />
        <span>{t(saveLabel)}</span>
      </Button>
    </SettingsPageActionsPortal>
  )
}
