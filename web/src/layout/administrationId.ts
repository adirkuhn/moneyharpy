import { useMatch } from 'react-router-dom'

/** The open administration's id, if the address is inside one. */
export function useAdministrationId() {
  const nested = useMatch('/:id/*')
  const home = useMatch('/:id')
  const id = home?.params.id ?? nested?.params.id ?? ''
  return /^\d{1,19}$/.test(id) ? id : ''
}
