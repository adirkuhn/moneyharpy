import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { type ReactNode, useState } from 'react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import { DocumentTable } from './DocumentTable'
import { AdvancedFilter, FilterPanel, RadioFilter } from './FilterPanel'
import { GettingStarted } from './GettingStarted'
import { ListActions } from './ListActions'
import { monthOf, type MonthRange } from './period'
import { PeriodPicker } from './PeriodPicker'

function Where() {
  const { pathname, search } = useLocation()
  return <div data-testid="where">{pathname + search}</div>
}

function show(ui: ReactNode, path = '/list') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/list" element={ui} />
          <Route path="/list/:id" element={<p>Opened</p>} />
        </Routes>
        <Where />
        <button type="button">Elsewhere</button>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('the actions of a list', () => {
  const actions = (
    <ListActions
      exports={['Excel per invoice', 'Excel per row', 'Compose export']}
      primary={{ to: '/list/new', label: 'New invoice' }}
      more={[{ to: '/settings', label: 'Invoice settings' }]}
    />
  )

  it('offers the primary action as a link', () => {
    show(actions)
    expect(screen.getByRole('link', { name: 'New invoice' })).toHaveAttribute('href', '/list/new')
  })

  it('shows statistics as not available yet', () => {
    show(actions)
    const stats = screen.getByRole('button', { name: 'Show statistics' })
    expect(stats).toHaveAttribute('aria-disabled', 'true')
    expect(stats).toHaveAccessibleDescription('Not available yet.')
  })

  it('opens the export menu, whose entries are not available yet, and closes it with Escape', async () => {
    show(actions)
    const exportButton = screen.getByRole('button', { name: 'Export' })
    expect(exportButton).toHaveAttribute('aria-expanded', 'false')
    await userEvent.click(exportButton)
    expect(exportButton).toHaveAttribute('aria-expanded', 'true')
    const entry = screen.getByText('Excel per invoice').closest('[aria-disabled]')
    expect(entry).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByText('Excel per row')).toBeInTheDocument()
    expect(screen.getByText('Compose export')).toBeInTheDocument()
    expect(screen.getAllByText('Not available yet.').length).toBeGreaterThan(0)
    await userEvent.keyboard('{Escape}')
    expect(exportButton).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('Excel per invoice')).toBeNull()
    expect(exportButton).toHaveFocus()
  })

  it('opens more actions and closes them on a click elsewhere', async () => {
    show(actions)
    const more = screen.getByRole('button', { name: 'More actions' })
    await userEvent.click(more)
    expect(screen.getByRole('link', { name: 'Invoice settings' })).toHaveAttribute('href', '/settings')
    await userEvent.click(screen.getByRole('button', { name: 'Elsewhere' }))
    expect(more).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByRole('link', { name: 'Invoice settings' })).toBeNull()
  })

  it('toggles the introduction with more information', async () => {
    function WithInfo() {
      const [shown, setShown] = useState(false)
      return (
        <>
          <ListActions exports={[]} primary={{ to: '/list/new', label: 'Add' }} more={[]} info={{ shown, onToggle: () => setShown(!shown), controls: 'intro' }} />
          {shown && <p id="intro">Intro</p>}
        </>
      )
    }
    show(<WithInfo />)
    const info = screen.getByRole('button', { name: 'More information' })
    expect(info).toHaveAttribute('aria-expanded', 'false')
    await userEvent.click(info)
    expect(info).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('Intro')).toBeInTheDocument()
  })
})

describe('the filter panel', () => {
  function Panel({ onApply = () => {} }: { onApply?: (state: string) => void }) {
    const [state, setState] = useState('all')
    return (
      <FilterPanel
        label="Invoices from this year"
        title="Invoice filters"
        quick={[
          { label: "This year's invoices", to: '/list', current: true },
          { label: 'Draft invoices', to: '/list?filter=state:draft', current: false },
        ]}
      >
        <AdvancedFilter onApply={() => onApply(state)}>
          <RadioFilter legend="Status" value={state} onChange={setState} options={[{ value: 'all', label: 'All' }, { value: 'paid', label: 'Paid' }]} />
        </AdvancedFilter>
      </FilterPanel>
    )
  }

  it('opens from a button named after the current filter and offers quick filters as links', async () => {
    show(<Panel />)
    const toggle = screen.getByRole('button', { name: 'Invoices from this year' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByRole('search', { name: 'Invoice filters' })).toBeNull()
    await userEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    const panel = screen.getByRole('search', { name: 'Invoice filters' })
    expect(within(panel).getByRole('link', { name: "This year's invoices" })).toHaveAttribute('aria-current', 'true')
    await userEvent.click(within(panel).getByRole('link', { name: 'Draft invoices' }))
    expect(screen.getByTestId('where')).toHaveTextContent('/list?filter=state:draft')
    expect(screen.queryByRole('search', { name: 'Invoice filters' })).toBeNull()
  })

  it('applies the advanced filter', async () => {
    let applied = ''
    show(<Panel onApply={(s) => { applied = s }} />)
    await userEvent.click(screen.getByRole('button', { name: 'Invoices from this year' }))
    const panel = screen.getByRole('search', { name: 'Invoice filters' })
    await userEvent.click(within(panel).getByRole('radio', { name: 'Paid' }))
    await userEvent.click(within(panel).getByRole('button', { name: 'Filter' }))
    expect(applied).toBe('paid')
  })

  it('closes with Escape, returning focus, and on a click elsewhere', async () => {
    show(<Panel />)
    const toggle = screen.getByRole('button', { name: 'Invoices from this year' })
    await userEvent.click(toggle)
    await userEvent.keyboard('{Escape}')
    expect(screen.queryByRole('search', { name: 'Invoice filters' })).toBeNull()
    expect(toggle).toHaveFocus()
    await userEvent.click(toggle)
    await userEvent.click(screen.getByRole('button', { name: 'Elsewhere' }))
    expect(screen.queryByRole('search', { name: 'Invoice filters' })).toBeNull()
  })
})

describe('the period picker', () => {
  function Picker({ initial }: { initial: MonthRange | null }) {
    const [range, setRange] = useState(initial)
    return <PeriodPicker value={range} onChange={setRange} today={new Date(2026, 10, 11)} />
  }

  it('picks a month, then widens it to a range with a second choice', async () => {
    show(<Picker initial={null} />)
    const picker = screen.getByRole('group', { name: 'Period' })
    expect(within(picker).getByRole('button', { name: 'All periods' })).toHaveAttribute('aria-pressed', 'true')
    await userEvent.click(within(picker).getByRole('button', { name: 'March 2026' }))
    expect(within(picker).getByRole('button', { name: 'March 2026' })).toHaveAttribute('aria-pressed', 'true')
    expect(within(picker).getByRole('status')).toHaveTextContent('Mar 2026')
    await userEvent.click(within(picker).getByRole('button', { name: 'Q3 2026' }))
    expect(within(picker).getByRole('status')).toHaveTextContent('Mar 2026 – Sep 2026')
    expect(within(picker).getByRole('button', { name: 'June 2026' })).toHaveAttribute('aria-pressed', 'true')
    expect(within(picker).getByRole('button', { name: 'October 2026' })).toHaveAttribute('aria-pressed', 'false')
  })

  it('picks a whole year, moves to earlier years and clears the period', async () => {
    show(<Picker initial={{ from: monthOf(2026, 0), to: monthOf(2026, 11) }} />)
    const picker = screen.getByRole('group', { name: 'Period' })
    expect(within(picker).getByRole('button', { name: '2026' })).toHaveAttribute('aria-pressed', 'true')
    expect(within(picker).getByRole('button', { name: 'Later years' })).toBeDisabled()
    await userEvent.click(within(picker).getByRole('button', { name: 'Earlier years' }))
    expect(within(picker).getByRole('button', { name: '2024' })).toBeInTheDocument()
    await userEvent.click(within(picker).getByRole('button', { name: '2024' }))
    expect(within(picker).getByRole('status')).toHaveTextContent('2024')
    await userEvent.click(within(picker).getByRole('button', { name: 'All periods' }))
    expect(within(picker).getByRole('status')).toHaveTextContent('All periods')
  })
})

type Row = { id: string; name: string; amount: string }

describe('the document table', () => {
  const rows: Row[] = [{ id: '1', name: 'First', amount: '€10.00' }, { id: '2', name: 'Second', amount: '€20.00' }]

  function Table() {
    const [selected, setSelected] = useState<Set<string>>(new Set())
    return (
      <>
        <DocumentTable
          label="Documents"
          rows={rows}
          rowKey={(r) => r.id}
          href={(r) => `/list/${r.id}`}
          columns={[
            { key: 'name', header: 'Name', link: true, cell: (r) => r.name },
            { key: 'amount', header: 'Amount', kind: 'amount', cell: (r) => r.amount },
          ]}
          selection={{ selected, onChange: setSelected, selectLabel: (r) => `Select ${r.name}`, allLabel: 'Select all documents' }}
          total={2}
          perPage={50}
        />
        <p>{selected.size} chosen</p>
      </>
    )
  }

  it('links every row to its document and opens it on a click anywhere in the row', async () => {
    show(<Table />)
    const table = screen.getByRole('table', { name: 'Documents' })
    expect(within(table).getByRole('link', { name: 'First' })).toHaveAttribute('href', '/list/1')
    expect(within(table).getByRole('columnheader', { name: 'Amount' })).toHaveClass('document-table__amount')
    await userEvent.click(within(table).getByText('€20.00'))
    expect(screen.getByTestId('where')).toHaveTextContent('/list/2')
  })

  it('selects rows without opening them', async () => {
    show(<Table />)
    const table = screen.getByRole('table', { name: 'Documents' })
    await userEvent.click(within(table).getByRole('checkbox', { name: 'Select First' }))
    expect(screen.getByText('1 chosen')).toBeInTheDocument()
    expect(screen.getByTestId('where')).toHaveTextContent('/list')
    await userEvent.click(within(table).getByRole('checkbox', { name: 'Select all documents' }))
    expect(screen.getByText('2 chosen')).toBeInTheDocument()
    await userEvent.click(within(table).getByRole('checkbox', { name: 'Select all documents' }))
    expect(screen.getByText('0 chosen')).toBeInTheDocument()
  })
})

describe('getting started', () => {
  it('introduces a list with its steps', () => {
    show(
      <GettingStarted
        title="Send your invoice"
        text="Create and send a sales invoice."
        steps={[
          { icon: 'file', title: 'Create and send', text: 'Make the invoice.' },
          { icon: 'palette', title: 'Your house style', text: 'Add your logo.' },
          { icon: 'flow', title: 'A workflow', text: 'Set reminders.' },
        ]}
      />,
    )
    const intro = screen.getByRole('region', { name: 'Send your invoice' })
    expect(within(intro).getByText('Create and send a sales invoice.')).toBeInTheDocument()
    expect(within(intro).getAllByRole('heading', { level: 3 }).map((h) => h.textContent)).toEqual(['Create and send', 'Your house style', 'A workflow'])
  })
})
