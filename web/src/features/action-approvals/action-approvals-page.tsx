import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ClipboardCheck, RefreshCw } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { SectionPageLayout } from '@/components/layout'
import { handleServerError } from '@/lib/handle-server-error'

import {
  approveActionApproval,
  denyActionApproval,
  getActionApprovals,
  type ActionApproval,
} from './api'

const STATUS_FILTERS = [
  'pending',
  'approved',
  'consumed',
  'denied',
  'expired',
  '',
] as const

function statusVariant(status: ActionApproval['status']) {
  switch (status) {
    case 'approved':
      return 'default' as const
    case 'pending':
      return 'secondary' as const
    case 'denied':
    case 'expired':
      return 'destructive' as const
    default:
      return 'outline' as const
  }
}

export function ActionApprovalsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [status, setStatus] = useState<string>('pending')
  const [page, setPage] = useState(1)
  const size = 50

  const query = useQuery({
    queryKey: ['action-approvals', status, page],
    queryFn: () => getActionApprovals({ status, page, size }),
    refetchInterval: 15_000,
  })

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['action-approvals'] })
  }

  const approve = useMutation({
    mutationFn: (id: number) => approveActionApproval(id),
    onSuccess: () => {
      toast.success(t('Approved'))
      invalidate()
    },
    onError: (error) => handleServerError(error, t('Operation failed')),
  })

  const deny = useMutation({
    mutationFn: (id: number) => denyActionApproval(id),
    onSuccess: () => {
      toast.success(t('Denied'))
      invalidate()
    },
    onError: (error) => handleServerError(error, t('Operation failed')),
  })

  const rows = query.data?.data ?? []
  const total = query.data?.total ?? 0
  const pageCount = Math.max(1, Math.ceil(total / size))

  // 审批后总数可能收缩；页码越界时自动回到最后一页，避免显示空表。
  useEffect(() => {
    if (page > pageCount) {setPage(pageCount)}
  }, [page, pageCount])

  const renderTableBody = () => {
    if (query.isLoading) {
      return (
        <TableRow>
          <TableCell colSpan={7} className='text-muted-foreground py-8 text-center'>
            {t('Loading...')}
          </TableCell>
        </TableRow>
      )
    }
    if (query.isError) {
      return (
        <TableRow>
          <TableCell colSpan={7} className='text-destructive py-8 text-center'>
            {t('Failed to load')}
          </TableCell>
        </TableRow>
      )
    }
    if (rows.length === 0) {
      return (
        <TableRow>
          <TableCell colSpan={7} className='text-muted-foreground py-8 text-center'>
            {t('No approvals')}
          </TableCell>
        </TableRow>
      )
    }
    return rows.map((row) => (
      <TableRow key={row.id}>
        <TableCell className='font-mono text-xs'>{row.id}</TableCell>
        <TableCell>{row.user_id}</TableCell>
        <TableCell className='font-mono text-xs'>{row.action}</TableCell>
        <TableCell className='max-w-[240px] truncate' title={row.resource}>
          {row.resource}
        </TableCell>
        <TableCell>
          <Badge variant={statusVariant(row.status)}>{t(row.status)}</Badge>
        </TableCell>
        <TableCell className='text-xs'>
          {row.expires_at ? new Date(row.expires_at * 1000).toLocaleString() : '—'}
        </TableCell>
        <TableCell className='text-end'>
          {row.status === 'pending' ? (
            <div className='flex justify-end gap-2'>
              <Button
                size='sm'
                onClick={() => approve.mutate(row.id)}
                disabled={(approve.isPending && approve.variables === row.id) || (deny.isPending && deny.variables === row.id)}
              >
                {t('Approve')}
              </Button>
              <Button
                size='sm'
                variant='destructive'
                onClick={() => deny.mutate(row.id)}
                disabled={(approve.isPending && approve.variables === row.id) || (deny.isPending && deny.variables === row.id)}
              >
                {t('Deny')}
              </Button>
            </div>
          ) : (
            '—'
          )}
        </TableCell>
      </TableRow>
    ))
  }

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Action Approvals')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <Card>
          <CardHeader>
            <CardTitle className='flex items-center gap-2'>
              <ClipboardCheck className='size-4' />
              {t('MCP paid actions')}
            </CardTitle>
            <CardDescription>
              {t(
                'Paid MCP tools are denied by default. Approving authorizes exactly one call; the approval expires automatically.'
              )}
            </CardDescription>
          </CardHeader>
          <CardContent className='space-y-3'>
            <div className='flex flex-wrap items-center gap-2'>
              {STATUS_FILTERS.map((filter) => (
                <Button
                  key={filter || 'all'}
                  size='sm'
                  variant={status === filter ? 'default' : 'outline'}
                  onClick={() => {
                    setStatus(filter)
                    setPage(1)
                  }}
                >
                  {filter ? t(filter) : t('All')}
                </Button>
              ))}
              <Button
                size='sm'
                variant='ghost'
                className='ms-auto'
                onClick={() => void query.refetch()}
                disabled={query.isFetching}
              >
                <RefreshCw className='me-1 size-3.5' />
                {t('Refresh')}
              </Button>
            </div>

            <div className='rounded-md border'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>#</TableHead>
                    <TableHead>{t('User')}</TableHead>
                    <TableHead>{t('Action')}</TableHead>
                    <TableHead>{t('Resource')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                    <TableHead>{t('Expires')}</TableHead>
                    <TableHead className='text-end'>{t('Actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {renderTableBody()}
                </TableBody>
              </Table>
            </div>

            <div className='flex items-center justify-between text-sm'>
              <span className='text-muted-foreground'>
                {t('Total: {{total}}', { total })}
              </span>
              <div className='flex items-center gap-2'>
                <Button
                  size='sm'
                  variant='outline'
                  disabled={page <= 1}
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                >
                  {t('Previous')}
                </Button>
                <span>
                  {page} / {pageCount}
                </span>
                <Button
                  size='sm'
                  variant='outline'
                  disabled={page >= pageCount}
                  onClick={() => setPage((p) => Math.min(pageCount, p + 1))}
                >
                  {t('Next')}
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
