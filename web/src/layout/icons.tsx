type IconProps = { className?: string }

const stroke = {
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 1.75,
  strokeLinecap: 'round',
  strokeLinejoin: 'round',
} as const

export function Mark({ className }: IconProps) {
  return (
    <svg className={className} viewBox="0 0 32 32" aria-hidden="true">
      <path d="M3 8c8 0 11.500 2 13 8 1.500-6 5-8 13-8-1 9-5 15-13 18C8 23 4 17 3 8Z" fill="#FFBD38" />
      <path d="M16 16c1.200-4 3.500-6 7-7.200C22 15 19.500 20 16 23.500Z" fill="#13BDB5" />
    </svg>
  )
}

export function StarIcon({ filled }: { filled: boolean }) {
  return (
    <svg viewBox="0 0 24 24" width="1.25em" height="1.25em" aria-hidden="true" {...stroke} fill={filled ? 'currentColor' : 'none'}>
      <path d="m12 3 2.800 5.800 6.200.9-4.500 4.400 1.100 6.300L12 17.400l-5.600 3 1.100-6.300L3 9.700l6.200-.9Z" />
    </svg>
  )
}

export function CogIcon() {
  return (
    <svg viewBox="0 0 24 24" width="1.25em" height="1.25em" aria-hidden="true" {...stroke}>
      <circle cx="12" cy="12" r="3" />
      <path d="M19 12a7 7 0 0 0-.1-1.200l2-1.600-2-3.400-2.400 1a7 7 0 0 0-2-1.200L14 3h-4l-.5 2.600a7 7 0 0 0-2 1.200l-2.400-1-2 3.400 2 1.600a7 7 0 0 0 0 2.400l-2 1.600 2 3.400 2.400-1a7 7 0 0 0 2 1.200L10 21h4l.5-2.600a7 7 0 0 0 2-1.200l2.400 1 2-3.400-2-1.600c.1-.4.100-.8.100-1.200Z" />
    </svg>
  )
}

export function ChevronIcon({ className }: IconProps) {
  return (
    <svg className={className} viewBox="0 0 24 24" width="1.125em" height="1.125em" aria-hidden="true" {...stroke}>
      <path d="m9 5 7 7-7 7" />
    </svg>
  )
}

export function CaretIcon() {
  return (
    <svg viewBox="0 0 24 24" width="0.875em" height="0.875em" aria-hidden="true" {...stroke}>
      <path d="m6 9 6 6 6-6" />
    </svg>
  )
}

export function BellIcon() {
  return (
    <svg viewBox="0 0 24 24" width="1.25em" height="1.25em" aria-hidden="true" {...stroke}>
      <path d="M6 16V11a6 6 0 1 1 12 0v5l2 2H4Z" />
      <path d="M10 21h4" />
    </svg>
  )
}

export function SearchIcon() {
  return (
    <svg viewBox="0 0 24 24" width="1.125em" height="1.125em" aria-hidden="true" {...stroke}>
      <circle cx="11" cy="11" r="6.5" />
      <path d="m16 16 4.5 4.5" />
    </svg>
  )
}

export function ArrowLeftIcon() {
  return (
    <svg viewBox="0 0 24 24" width="1.25em" height="1.25em" aria-hidden="true" {...stroke}>
      <path d="M19 12H5m6-6-6 6 6 6" />
    </svg>
  )
}

export function InfoIcon() {
  return (
    <svg viewBox="0 0 24 24" width="1.125em" height="1.125em" aria-hidden="true" {...stroke}>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 11v5m0-8h.01" />
    </svg>
  )
}

/** Line icons for the settings sections, drawn for MoneyHarpy. */
export function SectionIcon({ section }: { section: string }) {
  const shapes: Record<string, React.ReactNode> = {
    company: <><rect x="5" y="3.5" width="14" height="17" rx="1.5" /><path d="M9 8h1.5m3 0H15M9 12h1.5m3 0H15m-4.5 8.5V16h3v4.5" /></>,
    invoices_and_estimates: <><path d="M6.5 3.5h8l3.5 3.5v13.5h-11.5Z" /><path d="M14.5 3.5V7H18M9.5 12h5m-5 3.5h5" /></>,
    bookkeeping: <><rect x="3.5" y="4.5" width="17" height="12.5" rx="1.5" /><path d="m7 13 3-3 2.5 2.5L17 8M9 20.5h6" /></>,
    bank: <><path d="M3.5 9 12 4l8.5 5Z" /><path d="M5.5 9.5v8m4.3-8v8m4.4-8v8m4.3-8v8M3.5 20.5h17" /></>,
    administration: <><rect x="5" y="3.5" width="14" height="17" rx="1.5" /><path d="M8.5 8h7m-7 4h7m-7 4h4" /></>,
    connections: <><circle cx="6" cy="12" r="2.5" /><circle cx="18" cy="6" r="2.5" /><circle cx="18" cy="18" r="2.5" /><path d="m8.2 10.8 7.6-3.6m-7.6 6 7.6 3.6" /></>,
  }
  return (
    <svg viewBox="0 0 24 24" width="1.25em" height="1.25em" aria-hidden="true" {...stroke}>
      {shapes[section]}
    </svg>
  )
}

/** Line icons for the parts of an administration in the side menu. */
export function NavIcon({ kind }: { kind: string }) {
  const shapes: Record<string, React.ReactNode> = {
    overview: <><path d="M4 10.5 12 4l8 6.5" /><path d="M6 9v11h12V9M10 20v-5.5h4V20" /></>,
    invoices: <><path d="M6.5 3.5h8l3.5 3.5v13.5h-11.5Z" /><path d="M14.5 3.5V7H18M9.5 12h5m-5 3.5h5" /></>,
    incoming: <><path d="M3.5 13.5 6 5h12l2.5 8.5v6h-17Z" /><path d="M3.5 13.5h5l1 2h5l1-2h5" /></>,
    quotes: <><path d="M4 20c2-3 3.5-4 5-4 1.500 0 1.500 2 3 2s3-2 4.500-2" /><path d="m14 4.5 3 3L9.500 15H6.500v-3Z" /></>,
    bank: <><path d="M3.5 9 12 4l8.5 5Z" /><path d="M5.5 9.5v8m4.3-8v8m4.4-8v8m4.3-8v8M3.5 20.5h17" /></>,
    timesheets: <><circle cx="12" cy="12" r="8.5" /><path d="M12 7.5V12l3 2" /></>,
    contacts: <><rect x="5" y="3.5" width="14" height="17" rx="1.5" /><circle cx="12" cy="10" r="2.5" /><path d="M8 16.5c.8-1.800 2.200-2.500 4-2.500s3.200.7 4 2.500M3.5 8h1.5m-1.5 4h1.5m-1.5 4h1.5" /></>,
    vat: <><rect x="5" y="3.5" width="14" height="17" rx="1.5" /><path d="M8 7h8v3H8ZM8.5 13.5h.01m3.490 0h.01m3.490 0h.01M8.5 17h.01m3.490 0h.01m3.490 0h.01" /></>,
    reports: <><path d="M4 20.5h16" /><path d="M6.5 17v-5m4 5V7m4 10v-7.5m4 7.5V4.5" /></>,
    products: <><path d="M4 9h16l-1.500 10.500h-13Z" /><path d="m8.500 9 2-5m5 5-2-5" /></>,
    assets: <><path d="M3.5 8 12 4l8.5 4v8L12 20l-8.500-4Z" /><path d="m3.5 8 8.500 4 8.500-4M12 12v8" /></>,
    projects: <><rect x="3.5" y="7.5" width="17" height="12" rx="1.5" /><path d="M9 7.5V5.5a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2M3.5 12.5h17" /></>,
    tasks: <><path d="m4 6.5 1.500 1.500L8 5.500m-4 7 1.500 1.500L8 11.500m-4 7 1.500 1.500L8 17.500" /><path d="M11 7h9m-9 6h9m-9 6h9" /></>,
    settings: <><circle cx="12" cy="12" r="3" /><path d="M19 12a7 7 0 0 0-.1-1.200l2-1.600-2-3.400-2.400 1a7 7 0 0 0-2-1.200L14 3h-4l-.5 2.600a7 7 0 0 0-2 1.200l-2.400-1-2 3.400 2 1.600a7 7 0 0 0 0 2.400l-2 1.600 2 3.400 2.400-1a7 7 0 0 0 2 1.200L10 21h4l.5-2.600a7 7 0 0 0 2-1.200l2.400 1 2-3.400-2-1.600c.1-.4.100-.8.100-1.200Z" /></>,
    collapse: <><rect x="3.5" y="4.5" width="17" height="15" rx="2" /><path d="M9 4.5v15M15.500 10 13.500 12l2 2" /></>,
    expand: <><rect x="3.5" y="4.5" width="17" height="15" rx="2" /><path d="M9 4.5v15M13.500 10l2 2-2 2" /></>,
    menu: <path d="M4 6.5h16M4 12h16M4 17.5h16" />,
  }
  return (
    <svg viewBox="0 0 24 24" width="1.25em" height="1.25em" aria-hidden="true" {...stroke}>
      {shapes[kind]}
    </svg>
  )
}

/** Icons for the overview tiles: a balance, work waiting, money in and money out. */
export function TileIcon({ kind }: { kind: 'balance' | 'attention' | 'in' | 'out' }) {
  const shapes = {
    balance: <><rect x="3.5" y="6.5" width="17" height="12" rx="2" /><path d="M3.5 10.5h17M15.5 14.5h2" /></>,
    attention: <><circle cx="12" cy="12" r="8.5" /><path d="M12 7.5v5.5m0 3v.01" /></>,
    in: <><circle cx="12" cy="12" r="8.5" /><path d="M12 8v8m-4-4h8" /></>,
    out: <><circle cx="12" cy="12" r="8.5" /><path d="M8 12h8" /></>,
  }
  return (
    <svg viewBox="0 0 24 24" width="1.25em" height="1.25em" aria-hidden="true" {...stroke}>
      {shapes[kind]}
    </svg>
  )
}
