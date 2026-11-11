/** Icons for the settings, onboarding and overview pages. Outline style like icons.tsx. */

const stroke = {
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 1.75,
  strokeLinecap: 'round',
  strokeLinejoin: 'round',
} as const

export type PageIconKind = 'close' | 'trash' | 'invoice' | 'receipt' | 'card' | 'cabinet' | 'leave' | 'delete' | 'lock' | 'download' | 'user'

export function PageIcon({ kind, className }: { kind: PageIconKind; className?: string }) {
  const shapes: Record<PageIconKind, React.ReactNode> = {
    close: <path d="M6 6l12 12M18 6 6 18" />,
    trash: <><path d="M4.5 7h15M9.5 7V4.5h5V7" /><path d="M6.5 7l1 12.5h9l1-12.5M10.5 11v5m3-5v5" /></>,
    invoice: <><path d="M6.5 3.5h8l3 3v14h-11z" /><path d="M9.5 10.5h5M9.5 13.5h5M9.5 16.5h3" /></>,
    receipt: <><path d="M6.5 3.5h11v17l-2-1.300-1.800 1.300-1.700-1.300-1.700 1.300-1.800-1.300-2 1.300z" /><path d="M9.5 8.5h5M9.5 11.5h5M9.5 14.5h3" /></>,
    card: <><rect x="3.5" y="6" width="17" height="12" rx="2" /><path d="M3.5 10h17M7 14.5h3" /></>,
    cabinet: <><rect x="5" y="3.5" width="14" height="17" rx="1.500" /><path d="M5 12h14M10.500 7.500h3M10.500 16h3" /></>,
    leave: <><path d="M14 4.500H6.500v15H14" /><path d="M10.500 12h10m-3-3 3 3-3 3" /></>,
    delete: <><circle cx="12" cy="12" r="8.500" /><path d="m9 9 6 6m0-6-6 6" /></>,
    lock: <><rect x="5.500" y="10.500" width="13" height="9.500" rx="1.500" /><path d="M8.500 10.500V8a3.500 3.500 0 0 1 7 0v2.500" /></>,
    download: <path d="M12 4v11m-4.500-4.500L12 15l4.500-4.500M5 19.500h14" />,
    user: <><circle cx="12" cy="9" r="3.500" /><path d="M5.500 19.500c1-3.200 3.500-5 6.500-5s5.500 1.800 6.500 5" /></>,
  }
  return (
    <svg className={className} viewBox="0 0 24 24" width="1.25em" height="1.25em" aria-hidden="true" {...stroke}>
      {shapes[kind]}
    </svg>
  )
}
