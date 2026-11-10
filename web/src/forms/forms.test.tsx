import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ApiError } from '../api/client'
import { CheckboxInput, formErrors, TextInput } from './Form'

describe('form rows', () => {
  it('names the input by its label and describes it by its error', () => {
    render(<TextInput label="Account number" errors={['Account number is not a valid IBAN']} value="" onChange={() => {}} />)
    const input = screen.getByLabelText('Account number')
    expect(input).toHaveAttribute('aria-invalid', 'true')
    expect(input).toHaveAccessibleDescription('Account number is not a valid IBAN')
  })

  it('describes the input by its help text when there is no error', () => {
    render(<TextInput label="Website" help="Shown on your invoices" value="" onChange={() => {}} />)
    expect(screen.getByLabelText('Website')).toHaveAccessibleDescription('Shown on your invoices')
    expect(screen.getByLabelText('Website')).not.toHaveAttribute('aria-invalid')
  })

  it('names a checkbox by its own text', () => {
    render(<CheckboxInput label="Company does not have a website" checked={false} onChange={() => {}} />)
    expect(screen.getByRole('checkbox', { name: 'Company does not have a website' })).toBeInTheDocument()
  })
})

describe('formErrors', () => {
  const labels = { name: 'Name', currency: 'Currency' }

  it('puts problems of shown fields beside them and the rest on top', () => {
    const error = new ApiError(422, { error: { name: ["can't be blank"], base: ['Something else'] } })
    const errors = formErrors(error, labels)
    expect(errors.field('name')).toEqual(["Name can't be blank"])
    expect(errors.field('currency')).toBeUndefined()
    expect(errors.general).toEqual(['Something else'])
  })

  it('keeps a plain message on top', () => {
    const errors = formErrors(new ApiError(404, { error: 'Not found' }), labels)
    expect(errors.general).toEqual(['Not found'])
    expect(errors.field('name')).toBeUndefined()
  })

  it('is empty without an error', () => {
    expect(formErrors(null, labels).general).toEqual([])
  })
})
