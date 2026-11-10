import type { FormEventHandler, ReactNode } from 'react'
import { Mark } from '../layout/icons'
import { LanguageSelect } from '../layout/LanguageSelect'

type Props = {
  title: string
  subtitle: string
  art?: boolean
  languageSwitch?: boolean
  onSubmit?: FormEventHandler
  footer: ReactNode
  children?: ReactNode
}

/** A full-screen step of a guided flow, outside the normal app navigation. */
export function Wizard({ title, subtitle, art, languageSwitch = true, onSubmit, footer, children }: Props) {
  return (
    <main className="wizard">
      <Mark className="wizard__wing wizard__wing--left" />
      <Mark className="wizard__wing wizard__wing--right" />
      {languageSwitch && <LanguageSelect className="wizard__language" />}
      <form className="wizard__card" onSubmit={onSubmit}>
        {art && <WizardArt />}
        <h1>{title}</h1>
        <p className="wizard__subtitle">{subtitle}</p>
        {children && <div className="wizard__body">{children}</div>}
        <footer className="wizard__footer">{footer}</footer>
      </form>
      <p className="wizard__brand"><Mark className="brand__mark" />MoneyHarpy</p>
    </main>
  )
}

function WizardArt() {
  return (
    <svg className="wizard__art" viewBox="0 0 240 120" aria-hidden="true">
      <path d="M24 34c42 0 62 10 72 44 10-34 30-44 72-44-6 44-28 72-72 88C52 106 30 78 24 34Z" transform="translate(24 -8)" fill="#FFBD38" />
      <rect x="82" y="18" width="76" height="96" rx="6" fill="#FFFFFF" stroke="#092653" strokeWidth="3" />
      <path d="M94 38h52M94 54h52M94 70h30" stroke="#1678D3" strokeWidth="4" strokeLinecap="round" />
      <path d="m126 92 9 9 17-20" fill="none" stroke="#13BDB5" strokeWidth="6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}
