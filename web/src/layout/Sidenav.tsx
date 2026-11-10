import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, Outlet, useLocation } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { useAdministrationId } from './administrationId'
import { CaretIcon, NavIcon } from './icons'
import { contains, isAt, visibleItems, type NavItem, type NavLink } from './sidenavItems'

const sidenavId = 'sidenav'
const laterId = 'sidenav-later'
const storageKey = 'moneyharpy.sidenav'

type Drawer = { open: boolean; setOpen: (open: boolean) => void; present: boolean; setPresent: (present: boolean) => void }

const DrawerContext = createContext<Drawer>({ open: false, setOpen: () => {}, present: false, setPresent: () => {} })

/** Shares whether the side menu is on the page and, on small screens, open as a drawer. */
export function SidenavProvider({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false)
  const [present, setPresent] = useState(false)
  return <DrawerContext.Provider value={{ open, setOpen, present, setPresent }}>{children}</DrawerContext.Provider>
}

/** The top bar button that opens the side menu on small screens. */
export function MenuToggle() {
  const { t } = useTranslation()
  const { open, setOpen, present } = useContext(DrawerContext)
  if (!present) {
    return null
  }
  return (
    <button
      type="button"
      className="topnav__btn topnav__menu"
      aria-label={t('sidenav.menu')}
      aria-expanded={open}
      aria-controls={sidenavId}
      onClick={() => setOpen(!open)}
    >
      <NavIcon kind="menu" />
    </button>
  )
}

/** The pages of an administration that share the side menu. Settings bring their own. */
export function AdministrationLayout() {
  return (
    <div className="workspace">
      <Sidenav />
      <div className="workspace__page">
        <Outlet />
      </div>
    </div>
  )
}

function storedCollapsed() {
  try {
    return localStorage.getItem(storageKey) === 'collapsed'
  } catch {
    return false
  }
}

function storeCollapsed(collapsed: boolean) {
  try {
    localStorage.setItem(storageKey, collapsed ? 'collapsed' : 'expanded')
  } catch {
    // Without storage the choice lasts until the page reloads.
  }
}

function Sidenav() {
  const { t } = useTranslation()
  const id = useAdministrationId()
  const administration = useAdministration(id)
  const { pathname } = useLocation()
  const path = pathname.slice(id.length + 2)
  const drawer = useContext(DrawerContext)
  const { setOpen, setPresent } = drawer
  const [collapsed, setCollapsed] = useState(storedCollapsed)
  const items = visibleItems(administration.data?.permissions ?? [])
  const [expanded, setExpanded] = useState<Set<string>>(new Set())

  useEffect(() => {
    setPresent(true)
    return () => setPresent(false)
  }, [setPresent])

  // Following a link closes the drawer and opens the section it leads to.
  useEffect(() => {
    setOpen(false)
    const current = visibleItems(administration.data?.permissions ?? []).find((item) => item.children && contains(item, path))
    if (current) {
      setExpanded((open) => (open.has(current.key) ? open : new Set(open).add(current.key)))
    }
  }, [path, administration.data, setOpen])

  useEffect(() => {
    if (!drawer.open) {
      return
    }
    const close = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    document.addEventListener('keydown', close)
    return () => document.removeEventListener('keydown', close)
  }, [drawer.open, setOpen])

  if (!administration.data) {
    return <div className="sidenav" aria-hidden="true" />
  }

  const collapse = (next: boolean) => {
    setCollapsed(next)
    storeCollapsed(next)
  }

  const toggle = (item: NavItem) => {
    if (collapsed) {
      collapse(false)
      setExpanded((open) => new Set(open).add(item.key))
      return
    }
    setExpanded((open) => {
      const next = new Set(open)
      if (!next.delete(item.key)) {
        next.add(item.key)
      }
      return next
    })
  }

  const entry = (link: NavLink, label: string, icon?: ReactNode) => {
    const title = collapsed && icon ? label : undefined
    const content = <>{icon}<span className="sidenav__text">{label}</span></>
    if (!link.ready) {
      return (
        <span className="sidenav__link is-later" role="link" aria-disabled="true" aria-describedby={laterId} title={title}>
          {content}
        </span>
      )
    }
    const current = isAt(path, link.to)
    return (
      <Link className={`sidenav__link${current ? ' is-active' : ''}`} to={`/${id}/${link.to}`} aria-current={current ? 'page' : undefined} title={title}>
        {content}
      </Link>
    )
  }

  const classes = ['sidenav', collapsed && 'is-collapsed', drawer.open && 'is-open'].filter(Boolean).join(' ')

  return (
    <>
      <nav id={sidenavId} className={classes} aria-label={t('sidenav.label')}>
        <ul className="sidenav__items">
          {items.map((item) => {
            const label = t(`sidenav.items.${item.key}`)
            const icon = <NavIcon kind={item.kind} />
            if (!item.children) {
              return <li key={item.key}>{entry(item, label, icon)}</li>
            }
            const open = !collapsed && expanded.has(item.key)
            const groupId = `sidenav-${item.key}`
            return (
              <li key={item.key}>
                <button
                  type="button"
                  className={`sidenav__link${contains(item, path) ? ' is-within' : ''}`}
                  aria-expanded={open}
                  aria-controls={groupId}
                  title={collapsed ? label : undefined}
                  onClick={() => toggle(item)}
                >
                  {icon}
                  <span className="sidenav__text">{label}</span>
                  <span className="sidenav__arrow"><CaretIcon /></span>
                </button>
                <ul id={groupId} className="sidenav__subitems" hidden={!open}>
                  {open && item.children.map((child) => <li key={child.key}>{entry(child, t(`sidenav.items.${child.key}`))}</li>)}
                </ul>
              </li>
            )
          })}
        </ul>
        <button type="button" className="sidenav__link sidenav__collapse" aria-label={t(collapsed ? 'sidenav.expand' : 'sidenav.collapse')} onClick={() => collapse(!collapsed)}>
          <NavIcon kind={collapsed ? 'expand' : 'collapse'} />
          <span className="sidenav__text" aria-hidden="true">{t('sidenav.collapseShort')}</span>
        </button>
        <span id={laterId} className="sr-only">{t('sidenav.later')}</span>
      </nav>
      {drawer.open && <div className="sidenav__backdrop" onClick={() => setOpen(false)} />}
    </>
  )
}
