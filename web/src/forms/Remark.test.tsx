import { render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import '../i18n'
import { remarksOf } from '../incoming/recognition'
import { Remark } from './Remark'

describe('Remark', () => {
  it('marks a value read from the attachment with a bolt', () => {
    const { container } = render(<Remark confidence="certain" />)
    expect(screen.getByRole('img', { name: 'We filled this in for you from the attachment.' })).toHaveClass('remark--certain')
    expect(container.querySelectorAll('path')).toHaveLength(1)
  })

  it('marks a value to check with a warning triangle, as Moneybird does', () => {
    const { container } = render(<Remark confidence="uncertain" />)
    expect(screen.getByRole('img', { name: 'Please check: we were not sure about this.' })).toHaveClass('remark--uncertain')
    expect(container.querySelectorAll('path')).toHaveLength(2)
  })

  it('shows nothing for a value typed by hand', () => {
    const { container } = render(<Remark confidence={undefined} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('says which earlier document a remembered value came from, with a link to it, as Moneybird does', () => {
    render(
      <MemoryRouter>
        <Remark confidence="uncertain" source={{ to: '/11/documents/41', reference: 'EX-2026-1' }} />
      </MemoryRouter>,
    )
    const mark = screen.getByRole('img', { name: 'We filled this in based on EX-2026-1.' })
    expect(mark.closest('.remark')).toHaveClass('remark--uncertain')
    const tip = screen.getByRole('tooltip')
    expect(tip).toHaveTextContent('We filled this in based on EX-2026-1.')
    expect(within(tip).getByRole('link', { name: 'EX-2026-1' })).toHaveAttribute('href', '/11/documents/41')
  })

  it('names an earlier document without a reference', () => {
    render(
      <MemoryRouter>
        <Remark confidence="certain" source={{ to: '/11/documents/41', reference: null }} />
      </MemoryRouter>,
    )
    expect(within(screen.getByRole('tooltip')).getByRole('link', { name: 'an earlier document' })).toHaveAttribute('href', '/11/documents/41')
  })
})

describe('remarksOf', () => {
  const recognition = {
    state: 'done' as const, source: null,
    fields: [
      { field: 'currency', confidence: 'certain' as const, reason: 'previous_document' as const, source_document_id: '41', source_document_reference: 'EX-41' },
      { field: 'reference', confidence: 'certain' as const, reason: 'attachment' as const },
      { field: 'details.0.price', confidence: 'certain' as const, reason: 'attachment' as const },
      { field: 'details.0.ledger_account_id', confidence: 'uncertain' as const, reason: 'previous_document' as const, source_document_id: '42', source_document_reference: 'EX-42' },
    ],
  }
  const details = [{ id: 'a', row_order: 1 }]

  it('keeps where remembered values came from', () => {
    const remarks = remarksOf(recognition, details, { administrationId: '11' })
    expect(remarks.source('currency')).toEqual({ to: '/11/documents/41', reference: 'EX-41' })
    expect(remarks.source('reference')).toBeUndefined()
    expect(remarks.lineSources.a).toEqual({ to: '/11/documents/42', reference: 'EX-42' })
    expect(remarks.lines.a).toEqual({ price: 'certain', ledger_account_id: 'uncertain' })
  })

  it('keeps only the remembered values when asked', () => {
    const remarks = remarksOf(recognition, details, { administrationId: '11', rememberedOnly: true })
    expect(remarks.field('reference')).toBeUndefined()
    expect(remarks.field('currency')).toBe('certain')
    expect(remarks.lines.a).toEqual({ ledger_account_id: 'uncertain' })
  })
})
