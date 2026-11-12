import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it } from 'vitest'
import '../i18n'
import { TextileEditor } from './TextileEditor'

/** Lets the editor put its selection back after a change, as it does on the next frame. */
const frame = () => new Promise((resolve) => requestAnimationFrame(resolve))

function Harness({ initial = '' }: { initial?: string }) {
  const [value, setValue] = useState(initial)
  return <TextileEditor label="Payment conditions" value={value} onChange={setValue} extraTags={[{ group: 'company', name: 'custom_field_5', label: 'Slogan' }]} />
}

describe('the textile editor', () => {
  it('wraps the selection in bold, italic and underline markers', async () => {
    render(<Harness initial="Pay now" />)
    const area = screen.getByLabelText('Payment conditions') as HTMLTextAreaElement
    area.setSelectionRange(4, 7)
    await userEvent.click(screen.getByRole('button', { name: 'Bold' }))
    expect(area).toHaveValue('Pay *now*')
    await frame()
    expect([area.selectionStart, area.selectionEnd]).toEqual([5, 8])
    area.setSelectionRange(0, 3)
    await userEvent.click(screen.getByRole('button', { name: 'Italic' }))
    expect(area).toHaveValue('_Pay_ *now*')
    await frame()
    area.setSelectionRange(0, 0)
    await userEvent.click(screen.getByRole('button', { name: 'Underline' }))
    expect(area).toHaveValue('++_Pay_ *now*')
  })

  it("puts a tag at the cursor from Moneybird's tag groups", async () => {
    render(<Harness initial="Pay  please" />)
    const area = screen.getByLabelText('Payment conditions') as HTMLTextAreaElement
    area.setSelectionRange(4, 4)
    const tags = screen.getByRole('combobox', { name: 'Insert tag' })
    expect(within(tags).getAllByRole('group').map((g) => g.getAttribute('label'))).toEqual(['Document', 'Recipient', 'Contact person', 'Sender'])
    expect(within(tags).getByRole('option', { name: 'Slogan' })).toHaveValue('{company.custom_field_5}')
    await userEvent.selectOptions(tags, 'Payment reference')
    expect(area).toHaveValue('Pay {document.payment_reference} please')
    expect(tags).toHaveValue('')
  })
})
