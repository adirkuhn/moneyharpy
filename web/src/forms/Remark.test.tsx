import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import '../i18n'
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
})
