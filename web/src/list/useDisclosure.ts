import { useEffect, useRef, useState } from 'react'
import { useLocation } from 'react-router-dom'

/**
 * A button that shows a panel: Escape closes it and gives focus back to the
 * button, a click outside closes it, and so does going somewhere else.
 */
export function useDisclosure() {
  const [open, setOpen] = useState(false)
  const root = useRef<HTMLDivElement>(null)
  const button = useRef<HTMLButtonElement>(null)
  const { pathname, search } = useLocation()

  useEffect(() => setOpen(false), [pathname, search])
  useEffect(() => {
    if (!open) {
      return
    }
    // A control inside the panel that handles Escape itself prevents the default.
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !e.defaultPrevented) {
        setOpen(false)
        button.current?.focus()
      }
    }
    // The path is taken at dispatch, so a click on something the panel removed meanwhile still counts as inside.
    const onClick = (e: MouseEvent) => {
      if (root.current && !e.composedPath().includes(root.current)) {
        setOpen(false)
      }
    }
    document.addEventListener('keydown', onKey)
    document.addEventListener('click', onClick)
    return () => {
      document.removeEventListener('keydown', onKey)
      document.removeEventListener('click', onClick)
    }
  }, [open])

  return { open, setOpen, toggle: () => setOpen((o) => !o), close: () => setOpen(false), root, button }
}
