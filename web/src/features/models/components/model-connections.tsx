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
import { Pencil } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { GroupBadge } from '@/components/group-badge'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { requireServerSuccess } from '@/lib/server-error-message'
import { getGroups } from '@/features/users/api'

import type { Model } from '../types'
import { ModelSquareStatus } from './model-square-status'
import { ModelGroupsDialog } from './model-groups-dialog'

export function ModelConnections(props: { model: Model; onGroupsSaved?: () => void }) {
  const { t } = useTranslation()
  const [groupsDialogOpen, setGroupsDialogOpen] = useState(false)
  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: async () => requireServerSuccess(await getGroups()),
  })
  const availableGroups = Array.isArray(groupsData?.data)
    ? (groupsData.data as string[])
    : []
  const assignedGroups = props.model.groups
    ? props.model.groups.split(',').map((g) => g.trim()).filter(Boolean)
    : (props.model.enable_groups ?? [])

  return (
    <div className='min-h-0 flex-1 space-y-6 overflow-auto p-4'>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Channel availability and group access are derived from enabled channels. Importing metadata does not create a callable channel.'
        )}
      </p>
      <ModelSquareStatus model={props.model} detail />
      <section className='space-y-3'>
        <h3 className='font-medium'>{t('Bound Channels')}</h3>
        {props.model.bound_channels?.length ? (
          <StaticDataTable
            data={props.model.bound_channels}
            columns={[
              {
                id: 'name',
                header: t('Channel'),
                cell: (channel) => channel.name,
              },
              {
                id: 'type',
                header: t('Type'),
                cell: (channel) => channel.type,
              },
            ]}
          />
        ) : (
          <EmptyState title={t('No available channels')} />
        )}
      </section>
      <section className='space-y-3'>
        <div className='flex items-center justify-between'>
          <h3 className='font-medium'>{t('Assigned Groups')}</h3>
          <Button
            variant='ghost'
            size='sm'
            className='text-muted-foreground h-7 px-2 text-xs'
            onClick={() => setGroupsDialogOpen(true)}
          >
            <Pencil className='size-3.5' aria-hidden />
            {t('Edit groups')}
          </Button>
        </div>
        <div className='flex flex-wrap gap-2'>
          {assignedGroups.map((group) => (
            <GroupBadge key={group} group={group} />
          ))}
          {!assignedGroups.length && (
            <span className='text-muted-foreground text-sm'>
              {t('No enabled groups')}
            </span>
          )}
        </div>
        <p className='text-muted-foreground/70 text-xs'>
          {t(
            'Assign this model to groups (e.g. free for free models, vip for paid models). Saving syncs the groups to channels serving this model and makes it callable under them.'
          )}
        </p>
      </section>
      <section className='space-y-3'>
        <h3 className='font-medium'>{t('Supported endpoints')}</h3>
        <div className='flex flex-wrap gap-2'>
          {props.model.supported_endpoints?.map((endpoint) => (
            <StatusBadge key={endpoint} variant='neutral' label={endpoint} />
          ))}
          {!props.model.supported_endpoints?.length && (
            <span className='text-muted-foreground text-sm'>
              {t('No endpoints inferred from channels')}
            </span>
          )}
        </div>
      </section>
      {props.model.name_rule !== 0 && (
        <section className='space-y-3'>
          <h3 className='font-medium'>{t('Matched models')}</h3>
          <div className='flex flex-wrap gap-2'>
            {props.model.matched_models?.map((name) => (
              <StatusBadge key={name} variant='neutral' label={name} />
            ))}
          </div>
        </section>
      )}

      <ModelGroupsDialog
        open={groupsDialogOpen}
        onOpenChange={setGroupsDialogOpen}
        modelName={props.model.model_name}
        currentGroups={assignedGroups}
        availableGroups={availableGroups}
        onSaved={props.onGroupsSaved}
      />
    </div>
  )
}
