import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

/** Opens the ⋯ menu of the page's action bar and chooses one of its entries, an action or a link. */
export async function chooseMore(name: string) {
  await userEvent.click(await screen.findByRole('button', { name: 'More actions' }))
  const entry = screen.queryByRole('button', { name }) ?? screen.getByRole('link', { name })
  await userEvent.click(entry)
}
