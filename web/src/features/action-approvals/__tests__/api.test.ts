import { beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import {
  approveActionApproval,
  denyActionApproval,
  getActionApprovals,
} from '../api'

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn() },
}))

const mockedGet = vi.mocked(api.get)
const mockedPost = vi.mocked(api.post)

describe('action approvals api', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('treats HTTP 200 + success:false as an error instead of empty data', async () => {
    mockedGet.mockResolvedValue({
      data: { success: false, message: 'database unavailable' },
    })
    await expect(getActionApprovals({ status: 'pending' })).rejects.toThrow(
      /database unavailable/i
    )
  })

  it('treats failed approve as an error (no false success)', async () => {
    mockedPost.mockResolvedValue({
      data: { success: false, message: 'approval is not pending' },
    })
    await expect(approveActionApproval(1)).rejects.toThrow(
      /approval is not pending/i
    )
  })

  it('treats failed deny as an error', async () => {
    mockedPost.mockResolvedValue({
      data: { success: false, message: 'invalid approval id' },
    })
    await expect(denyActionApproval(1)).rejects.toThrow(/invalid approval id/i)
  })
})
