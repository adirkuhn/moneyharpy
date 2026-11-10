import { useId } from 'react'

type Props = { label: string; help?: string; checked: boolean; disabled?: boolean; onChange: (checked: boolean) => void }

/** An on-off row that applies at once, like Moneybird's switches. */
export function SwitchRow({ label, help, checked, disabled, onChange }: Props) {
  const id = useId()
  return (
    <div className="switch-row">
      <div>
        <label htmlFor={id} className="switch-row__label">{label}</label>
        {help && <p className="hint" id={`${id}-help`}>{help}</p>}
      </div>
      <input
        id={id}
        type="checkbox"
        role="switch"
        className="switch"
        checked={checked}
        disabled={disabled}
        aria-describedby={help ? `${id}-help` : undefined}
        onChange={(e) => onChange(e.target.checked)}
      />
    </div>
  )
}
