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
import { api } from '@/lib/api'
import type { CustomOAuthBinding } from '@/lib/oauth'
import { authRequestOptions, authResult } from '@/lib/secure-verification'
import type { LoginSession } from '@/stores/auth-store'

import { normalizeUserSettings } from './lib/user-settings'
import type {
  ApiResponse,
  UserProfile,
  UpdateUserRequest,
  UpdateUserSettingsRequest,
  CheckinStatusResponse,
  CheckinResponse,
  AccountSecurityResult,
  EmailBindingFlow,
  UserSkill,
  AgentPreset,
  TrainingDataConsentStatus,
} from './types'

// ============================================================================
// User Profile APIs
// ============================================================================

/**
 * Get current user profile
 */
export async function getUserProfile(): Promise<ApiResponse<UserProfile>> {
  const res = await api.get('/api/user/self')
  return res.data
}

/**
 * Update user profile
 */
export async function updateUserProfile(
  data: UpdateUserRequest
): Promise<ApiResponse> {
  const res = await api.put('/api/user/self', data, {
    acceptAuthRotation: Boolean(data.password),
  })
  return res.data
}

export function changeAccountPassword(
  data: UpdateUserRequest,
  proofToken: string,
  signal: AbortSignal
): Promise<AccountSecurityResult & { has_password: boolean }> {
  return authResult(
    api.put('/api/user/self', data, {
      ...authRequestOptions,
      headers: { 'X-Security-Proof': proofToken },
      acceptAuthRotation: true,
      singleUseAuthorization: true,
      signal,
    })
  )
}

/**
 * Update user settings
 */
export async function updateUserSettings(
  data: UpdateUserSettingsRequest
): Promise<ApiResponse> {
  const profile = await getUserProfile()
  if (!profile.success || !profile.data) {
    return { success: false, message: profile.message }
  }
  const settings = normalizeUserSettings(profile.data.setting)
  const res = await api.put('/api/user/setting', { ...settings, ...data })
  return res.data
}

/**
 * Update interface language preference
 */
export async function updateUserLanguage(
  language: string
): Promise<ApiResponse> {
  const res = await api.put('/api/user/self', { language })
  return res.data
}

/**
 * T8 更新用户记忆注入文本。
 *
 * 与 language 同构：走 PUT /api/user/self，后端以现有设置为基底只覆盖该字段。
 * 服务端按字符数限长（MaxMemoryInjectionRunes），超长返回失败。
 */
export async function updateUserMemoryInjection(
  memoryInjection: string
): Promise<ApiResponse> {
  const res = await api.put('/api/user/self', {
    memory_injection: memoryInjection,
  })
  return res.data
}

/**
 * T13 保存用户技能表（整表覆盖）。与 memory_injection 同构。
 * 服务端按数量/长度校验，超限返回失败；前端也应先拦。
 */
export async function updateUserSkills(
  skills: UserSkill[]
): Promise<ApiResponse> {
  const res = await api.put('/api/user/self', { skills })
  return res.data
}

/**
 * T13 保存 Agent 预设（整表覆盖）。
 */
export async function updateAgentPresets(
  presets: AgentPreset[]
): Promise<ApiResponse> {
  const res = await api.put('/api/user/self', { agent_presets: presets })
  return res.data
}

/**
 * Delete user account
 */
export function deleteUserAccount(
  proof: string,
  signal: AbortSignal
): Promise<AccountSecurityResult> {
  return authResult(
    api.delete('/api/user/self', {
      ...authRequestOptions,
      headers: { 'X-Security-Proof': proof },
      singleUseAuthorization: true,
      signal,
    }),
    'Failed to delete account'
  )
}

// ============================================================================
// Account Binding APIs
// ============================================================================

/**
 * Send email verification code
 */
export async function sendEmailVerification(
  email: string,
  turnstileToken?: string
): Promise<ApiResponse> {
  const params = new URLSearchParams({ email })
  if (turnstileToken) {
    params.append('turnstile', turnstileToken)
  }
  const res = await api.get(`/api/verification?${params}`)
  return res.data
}

/**
 * Bind email account
 */
export function startEmailBinding(
  email: string,
  proofToken: string,
  signal: AbortSignal
): Promise<EmailBindingFlow> {
  return authResult(
    api.post(
      '/api/oauth/email/bind/start',
      { email },
      {
        ...authRequestOptions,
        headers: { 'X-Security-Proof': proofToken },
        singleUseAuthorization: true,
        signal,
      }
    )
  )
}

export function resendEmailBinding(
  flowToken: string,
  signal: AbortSignal
): Promise<EmailBindingFlow> {
  return authResult(
    api.post(
      '/api/oauth/email/bind/resend',
      { flow_token: flowToken },
      {
        ...authRequestOptions,
        singleUseAuthorization: true,
        signal,
      }
    )
  )
}

export function bindEmail(
  flowToken: string,
  newCode: string,
  oldCode: string,
  signal: AbortSignal
): Promise<AccountSecurityResult> {
  return authResult(
    api.post(
      '/api/oauth/email/bind',
      { flow_token: flowToken, new_code: newCode, old_code: oldCode },
      {
        ...authRequestOptions,
        singleUseAuthorization: true,
        signal,
      }
    )
  )
}

/**
 * Bind WeChat account
 */
export function bindWeChat(
  code: string,
  proofToken: string,
  signal: AbortSignal
): Promise<AccountSecurityResult> {
  return authResult(
    api.post(
      '/api/oauth/wechat/bind',
      { code },
      {
        ...authRequestOptions,
        headers: { 'X-Security-Proof': proofToken },
        singleUseAuthorization: true,
        signal,
      }
    )
  )
}

export interface TelegramBindFlow {
  flow_token: string
  callback_url: string
  expires_at: number
}

export async function startTelegramBind(): Promise<
  ApiResponse<TelegramBindFlow>
> {
  const res = await api.post('/api/oauth/telegram/bind/start')
  return res.data
}

// ============================================================================
// Login Session APIs
// ============================================================================

export async function getLoginSessions(): Promise<ApiResponse<LoginSession[]>> {
  const res = await api.get('/api/user/sessions')
  return res.data
}

export async function revokeLoginSession(sid: string): Promise<ApiResponse> {
  const res = await api.delete(`/api/user/sessions/${encodeURIComponent(sid)}`)
  return res.data
}

export async function revokeOtherLoginSessions(): Promise<ApiResponse> {
  const res = await api.post('/api/user/sessions/revoke-others')
  return res.data
}

// ============================================================================
// Custom OAuth Binding APIs
// ============================================================================

/**
 * Get current user's custom OAuth bindings
 */
export async function getSelfOAuthBindings(): Promise<
  ApiResponse<CustomOAuthBinding[]>
> {
  const res = await api.get('/api/user/oauth/bindings')
  return res.data
}

/**
 * Unbind a custom OAuth provider for current user
 */
export function unbindCustomOAuth(
  providerId: number,
  proofToken: string,
  signal: AbortSignal
): Promise<AccountSecurityResult> {
  return authResult(
    api.delete(`/api/user/oauth/bindings/${providerId}`, {
      ...authRequestOptions,
      headers: { 'X-Security-Proof': proofToken },
      singleUseAuthorization: true,
      signal,
    })
  )
}

// ============================================================================
// Checkin APIs
// ============================================================================

/**
 * Get checkin status for a specific month
 */
export async function getCheckinStatus(
  month: string
): Promise<ApiResponse<CheckinStatusResponse>> {
  const res = await api.get(`/api/user/checkin?month=${month}`)
  return res.data
}

/**
 * Perform daily checkin
 */
export async function performCheckin(
  turnstileToken?: string
): Promise<ApiResponse<CheckinResponse>> {
  const url = turnstileToken
    ? `/api/user/checkin?turnstile=${encodeURIComponent(turnstileToken)}`
    : '/api/user/checkin'
  const res = await api.post(url)
  return res.data
}

// ============================================================================
// Training Data Collection APIs (Spec-008)
// ============================================================================

/**
 * 读取当前用户的训练数据采集授权状态。
 *
 * 服务端只认会话里的用户 ID，不接受任何客户端传入的用户标识。
 */
export async function getTrainingDataStatus(): Promise<
  ApiResponse<TrainingDataConsentStatus>
> {
  const res = await api.get('/api/user/training-data/status')
  return res.data
}

/**
 * 同意采集（必须在用户于免责声明弹窗中确认后调用）。
 *
 * 传 `policyVersion` 与服务端不一致时服务端会拒绝 —— 那说明免责声明
 * 已更新，前端应重新拉取文案让用户重新确认，而不是把旧同意当数。
 */
export async function grantTrainingDataConsent(
  policyVersion: string
): Promise<ApiResponse<TrainingDataConsentStatus>> {
  const res = await api.post('/api/user/training-data/grant', {
    policy_version: policyVersion,
  })
  return res.data
}

/**
 * 撤回同意（立即停止后续采集）。
 *
 * 注意语义边界：撤回**不删除**已上传的数据，只停止后续采集。
 */
export async function revokeTrainingDataConsent(): Promise<
  ApiResponse<TrainingDataConsentStatus>
> {
  const res = await api.post('/api/user/training-data/revoke')
  return res.data
}
