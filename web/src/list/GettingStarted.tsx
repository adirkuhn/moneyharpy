import { type ReactNode, useId } from 'react'
import { ListIcon, type ListIconKind } from '../layout/icons'

export type Step = { icon: ListIconKind; title: string; text: string }

type Props = {
  id?: string
  title: string
  text: string
  steps: Step[]
  /** Actions under the text, such as the list's primary button. */
  children?: ReactNode
}

/** The introduction to a list, after Moneybird's: what it is for and the steps to get going. */
export function GettingStarted({ id, title, text, steps, children }: Props) {
  const heading = useId()
  return (
    <section id={id} className="getting-started" aria-labelledby={heading}>
      <div className="getting-started__intro">
        <h2 id={heading} className="getting-started__title">{title}</h2>
        <p className="getting-started__text">{text}</p>
        {children && <div className="getting-started__actions">{children}</div>}
      </div>
      <ol className="getting-started__steps">
        {steps.map((step) => (
          <li key={step.title} className="getting-started__step">
            <span className="getting-started__icon"><ListIcon kind={step.icon} /></span>
            <div>
              <h3 className="getting-started__step-title">{step.title}</h3>
              <p className="getting-started__step-text">{step.text}</p>
            </div>
          </li>
        ))}
      </ol>
    </section>
  )
}
