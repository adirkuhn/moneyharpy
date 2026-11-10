/**
 * The MoneyHarpy logo. Both colour variants are in the page and CSS shows the
 * one that suits the theme; the wrapper carries the name for screen readers.
 */
type Props = {
  variant?: 'full' | 'mark'
  className?: string
  /** Hides the logo from screen readers where another copy already names it. */
  decorative?: boolean
}

export function Logo({ variant = 'full', className = '', decorative = false }: Props) {
  const file = variant === 'full' ? 'harpy-logo' : 'harpy-mark'
  const name = decorative ? { 'aria-hidden': true } : { role: 'img', 'aria-label': 'MoneyHarpy' }
  return (
    <span className={`logo logo--${variant} ${className}`} {...name}>
      <img className="logo__light" src={`/brand/${file}.svg`} alt="" />
      <img className="logo__dark" src={`/brand/${file}-dark.svg`} alt="" />
    </span>
  )
}
