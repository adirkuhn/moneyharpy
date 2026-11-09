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
    <svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true" {...stroke} fill={filled ? 'currentColor' : 'none'}>
      <path d="m12 3 2.800 5.800 6.200.9-4.500 4.400 1.100 6.300L12 17.400l-5.600 3 1.100-6.300L3 9.700l6.200-.9Z" />
    </svg>
  )
}

export function CogIcon() {
  return (
    <svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true" {...stroke}>
      <circle cx="12" cy="12" r="3" />
      <path d="M19 12a7 7 0 0 0-.1-1.200l2-1.600-2-3.400-2.400 1a7 7 0 0 0-2-1.200L14 3h-4l-.5 2.600a7 7 0 0 0-2 1.200l-2.400-1-2 3.400 2 1.600a7 7 0 0 0 0 2.400l-2 1.600 2 3.400 2.400-1a7 7 0 0 0 2 1.200L10 21h4l.5-2.600a7 7 0 0 0 2-1.200l2.400 1 2-3.400-2-1.600c.1-.4.100-.8.100-1.200Z" />
    </svg>
  )
}

export function ChevronIcon({ className }: IconProps) {
  return (
    <svg className={className} viewBox="0 0 24 24" width="18" height="18" aria-hidden="true" {...stroke}>
      <path d="m9 5 7 7-7 7" />
    </svg>
  )
}

export function CaretIcon() {
  return (
    <svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true" {...stroke}>
      <path d="m6 9 6 6 6-6" />
    </svg>
  )
}

export function BellIcon() {
  return (
    <svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true" {...stroke}>
      <path d="M6 16V11a6 6 0 1 1 12 0v5l2 2H4Z" />
      <path d="M10 21h4" />
    </svg>
  )
}
