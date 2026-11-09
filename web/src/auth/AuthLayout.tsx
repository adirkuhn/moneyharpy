import type { ReactNode } from 'react'
import { Mark } from '../layout/icons'

export function AuthLayout({ title, children }: { title: string; children: ReactNode }) {
  return (
    <main className="auth">
      <div className="auth__panel">
        <Mark className="auth__mark" />
        <h1>{title}</h1>
        {children}
      </div>
    </main>
  )
}

export function Field({
  label,
  ...input
}: { label: string } & React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <label className="field">
      <span>{label}</span>
      <input {...input} />
    </label>
  )
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
