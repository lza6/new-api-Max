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
import {
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
} from '@tanstack/react-table'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { DataTableView } from '../data-table-view'
import { DataTableCardGrid } from '../../layout/card-grid'
import { MobileCardList } from '../../layout/mobile-card-list'

type Row = { id: number; name: string }

const rows: Row[] = [
  { id: 1, name: 'alpha' },
  { id: 2, name: 'beta' },
]
const columns: ColumnDef<Row>[] = [
  { accessorKey: 'id', header: 'ID' },
  { accessorKey: 'name', header: 'Name' },
]

function Fixture(props: {
  isError?: boolean
  data?: Row[]
  onRetry?: () => void
  errorTitle?: string
  errorDescription?: string
}) {
  const table = useReactTable({
    data: props.data ?? [],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })
  return (
    <DataTableView
      table={table}
      isError={props.isError}
      onRetry={props.onRetry}
      errorTitle={props.errorTitle}
      errorDescription={props.errorDescription}
    />
  )
}

describe('DataTableView error state', () => {
  it('renders a retryable error state instead of an empty list when isError is set', () => {
    const onRetry = vi.fn()
    render(<Fixture isError onRetry={onRetry} />)

    // Error copy is shown, not the "no data" copy.
    expect(screen.getByText('Oops! Something went wrong')).toBeInTheDocument()

    // Retry is wired to the caller's refetch.
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(onRetry).toHaveBeenCalledTimes(1)
  })

  it('shows the error copy passed by the caller', () => {
    render(
      <Fixture
        isError
        errorTitle='Failed to load API keys'
        errorDescription='Please try again.'
      />
    )
    expect(screen.getByText('Failed to load API keys')).toBeInTheDocument()
    expect(screen.getByText('Please try again.')).toBeInTheDocument()
  })

  it('shows rows (not the error state) when data is present and isError is false', () => {
    render(<Fixture data={rows} isError={false} />)
    expect(screen.getByText('alpha')).toBeVisible()
    expect(screen.getByText('beta')).toBeVisible()
    expect(
      screen.queryByText('Oops! Something went wrong')
    ).not.toBeInTheDocument()
  })

  it('error state wins over a non-empty row model (error branch first)', () => {
    // Guards the "error wins" ordering: even with rows present, isError shows
    // the retryable error, never the stale rows.
    render(<Fixture data={rows} isError />)
    expect(screen.getByText('Oops! Something went wrong')).toBeInTheDocument()
    expect(screen.queryByText('alpha')).not.toBeInTheDocument()
  })
})

describe('DataTableCardGrid error state (mobile/card path)', () => {
  function CardFixture(props: { isError?: boolean; onRetry?: () => void }) {
    const table = useReactTable({
      data: [],
      columns,
      getCoreRowModel: getCoreRowModel(),
    })
    return (
      <DataTableCardGrid
        table={table}
        isError={props.isError}
        errorTitle='Failed in card view'
        onRetry={props.onRetry}
      />
    )
  }

  it('renders the error state (not the empty card grid) when isError is set', () => {
    render(<CardFixture isError />)
    expect(screen.getByText('Failed in card view')).toBeInTheDocument()
    expect(screen.queryByText('No Data')).not.toBeInTheDocument()
  })
})

describe('MobileCardList error state (mobile path)', () => {
  function MobileFixture(props: { isError?: boolean; onRetry?: () => void }) {
    const table = useReactTable({
      data: [],
      columns,
      getCoreRowModel: getCoreRowModel(),
    })
    return (
      <MobileCardList
        table={table}
        isError={props.isError}
        errorTitle='Failed on mobile'
        onRetry={props.onRetry}
      />
    )
  }

  it('renders the error state (not the empty list) when isError is set', () => {
    render(<MobileFixture isError />)
    expect(screen.getByText('Failed on mobile')).toBeInTheDocument()
  })
})
