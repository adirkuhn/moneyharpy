const stroke = { fill: 'none', stroke: 'currentColor', strokeWidth: 1.6, strokeLinecap: 'round', strokeLinejoin: 'round' } as const

export type ContactIconKind = 'merge' | 'import' | 'card' | 'pin' | 'send' | 'wallet' | 'flow' | 'fields' | 'plus' | 'info'

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
  }
  return (
    <svg viewBox="0 0 24 24" width="1.125em" height="1.125em" aria-hidden="true" {...stroke}>
      {shapes[kind]}
    </svg>
  )
}
