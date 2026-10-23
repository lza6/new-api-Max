import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

type ActionApprovalStatus =
  | 'pending'
  | 'approved'
  | 'denied'
  | 'consumed'
  | 'expired'

export interface ActionApproval {
  id: number
  user_id: number
  action: string
  resource: string
  status: ActionApprovalStatus
  reason: string
  admin_id: number
  created_at: number
  updated_at: number
  expires_at: number
  consumed_at: number
}

export interface ActionApprovalList {
  success: boolean
  data: ActionApproval[]
  total: number
  page: number
  size: number
}

export async function getActionApprovals(params: {
  status?: string
  page?: number
  size?: number
}): Promise<ActionApprovalList> {
  const res = await api.get<ActionApprovalList>('/api/action-approvals/', { params })
  return requireServerSuccess(res.data)
}

export async function approveActionApproval(id: number, reason = '') {
  const res = await api.post(`/api/action-approvals/${id}/approve`, { reason })
  requireServerSuccess(res.data)
}

export async function denyActionApproval(id: number, reason = '') {
  const res = await api.post(`/api/action-approvals/${id}/deny`, { reason })
  requireServerSuccess(res.data)
}
