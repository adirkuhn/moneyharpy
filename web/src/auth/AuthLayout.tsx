import type { ReactNode } from 'react'
import { LanguageSelect } from '../layout/LanguageSelect'
import { Mark } from '../layout/icons'
import { Logo } from '../layout/Logo'

export function AuthLayout({ title, children }: { title: string; children: ReactNode }) {
  return (
    <main className="wizard auth">
      <Mark className="wizard__wing wizard__wing--left" />
      <Mark className="wizard__wing wizard__wing--right" />
      <LanguageSelect className="wizard__language auth__language" />
      <p className="auth__logo"><Logo /></p>
      <div className="auth__panel">
        <h1>{title}</h1>
        {children}
      </div>
    </main>
  )
}

/** A compact input named by its placeholder, for short sign-in forms. */
export function AuthInput({ label, ...input }: { label: string } & React.InputHTMLAttributes<HTMLInputElement>) {
  return <input className="auth__input" aria-label={label} placeholder={label} {...input} />
}

export function Problems({ messages }: { messages: string[] }) {
  if (messages.length === 0) {
    return null
  }
  return (
    <div role="alert" className="problems">
      {messages.map((m) => <p key={m}>{m}</p>)}
    </div>
  )
}
