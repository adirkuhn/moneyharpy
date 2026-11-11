const stroke = { fill: 'none', stroke: 'currentColor', strokeWidth: 1.6, strokeLinecap: 'round', strokeLinejoin: 'round' } as const

export type ContactIconKind =
  | 'merge' | 'import' | 'card' | 'pin' | 'send' | 'wallet' | 'flow' | 'fields' | 'plus' | 'info'
  | 'phone' | 'mail' | 'map' | 'landmark' | 'pen' | 'invoice' | 'external' | 'quote' | 'purchase' | 'receipt'

/** Line icons of the contacts pages, drawn for MoneyHarpy. */
export function ContactIcon({ kind }: { kind: ContactIconKind }) {
  const shapes: Record<ContactIconKind, React.ReactNode> = {
    merge: <><path d="M6 4v4.500a4 4 0 0 0 4 4h4a4 4 0 0 1 4 4V20" /><path d="M18 4v4.500a4 4 0 0 1-4 4" /><path d="m15.500 17.500 2.500 2.500 2.500-2.500" /></>,
    import: <><path d="M12 4v11m-4-4 4 4 4-4" /><path d="M5 12.500v7h14v-7" /></>,
    card: <><rect x="3.500" y="5" width="17" height="14" rx="2" /><circle cx="9" cy="11" r="2" /><path d="M6 16c.6-1.300 1.700-2 3-2s2.400.7 3 2M14.500 10h3.500m-3.500 3.500h2.500" /></>,
    pin: <><path d="M12 20.500s6-5.500 6-10.500a6 6 0 0 0-12 0c0 5 6 10.500 6 10.500Z" /><circle cx="12" cy="10" r="2.200" /></>,
    send: <><path d="M20.500 3.500 10 14" /><path d="m20.500 3.500-6.500 17-4-6.500-6.500-4Z" /></>,
    wallet: <><path d="M4 7.500A2.500 2.500 0 0 1 6.500 5H17v3" /><rect x="4" y="8" width="16" height="11" rx="2" /><path d="M16 13.500h.01" strokeWidth={2.500} /></>,
    flow: <><rect x="3.500" y="4" width="6" height="5" rx="1" /><rect x="14.500" y="15" width="6" height="5" rx="1" /><path d="M6.500 9v4.500a2 2 0 0 0 2 2h6" /></>,
    fields: <><rect x="3.500" y="4" width="13" height="13" rx="2" /><path d="M7 20h11a2 2 0 0 0 2-2V7M10 7.500v6M7 10.500h6" /></>,
    plus: <path d="M12 5v14M5 12h14" />,
    info: <><circle cx="12" cy="12" r="8.500" /><path d="M12 11v5M12 8h.01" /></>,
    phone: <path d="M8.200 4H5.500A1.500 1.500 0 0 0 4 5.600 14.500 14.500 0 0 0 18.400 20 1.500 1.500 0 0 0 20 18.500v-2.700l-3.700-1.600-1.900 1.900a11 11 0 0 1-4.900-4.900l1.900-1.900Z" />,
    mail: <><rect x="3.500" y="5.500" width="17" height="13" rx="2" /><path d="m4 7 8 6 8-6" /></>,
    map: <><path d="M9 5 3.500 7v12.500L9 17.500l6 2 5.500-2V5L15 7Z" /><path d="M9 5v12.500M15 7v12.500" /></>,
    landmark: <><path d="M3.500 9.500 12 4.500l8.500 5Z" /><path d="M5.500 10v7m4.333-7v7m4.334-7v7M18.500 10v7M3.500 20h17" /></>,
    pen: <><path d="M4 20h4L19 9a2.800 2.800 0 0 0-4-4L4 16Z" /><path d="m13.500 6.500 4 4" /></>,
    invoice: <><path d="M6.500 3.500h8l3.500 3.500v13.500h-11.500Z" /><path d="M14.500 3.500V7H18M9.500 11h5m-5 3h5m-5 3h3" /></>,
    external: <><path d="M6.500 3.500h8l3.500 3.500v13.500h-11.500Z" /><path d="M14.500 3.500V7H18M10 16l4.500-4.500M11 11.500h3.500V15" /></>,
    quote: <><path d="M6.500 3.500h8l3.500 3.500v13.500h-11.500Z" /><path d="M14.500 3.500V7H18M9.500 12.500h5m-5 3h3" /></>,
    purchase: <><path d="M6.500 3.500h11v17l-2.200-1.500-2.200 1.500-2.200-1.500-2.200 1.500-2.200-1.500Z" transform="translate(-1 0)" /><path d="M8.500 9h6m-6 3.500h6" /></>,
    receipt: <><rect x="3.500" y="7" width="17" height="12.500" rx="2" /><path d="m4 9 8 5 8-5M7.500 7V4.500h9V7" /></>,
  }
  return (
    <svg viewBox="0 0 24 24" width="1.125em" height="1.125em" aria-hidden="true" {...stroke}>
      {shapes[kind]}
    </svg>
  )
}
