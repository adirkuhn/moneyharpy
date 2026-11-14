import { useId } from 'react'

/** How a due date is given: as a date, or as a number of days after the document's date. */
export type DueMode = 'date' | 'days'

type Props = {
  mode: DueMode
  onMode: (mode: DueMode) => void
  /** What the choice is called, such as "Due date as". */
  label: string
  /** The words of each choice: "Date" and "Days". */
  words: Record<DueMode, string>
}

/** Moneybird's "Date | Days" switch at the right of a due date, as a segmented radio group. */
export function DueModeToggle({ mode, onMode, label, words }: Props) {
  const name = useId()
  return (
    <fieldset className="segmented-control" role="radiogroup" aria-label={label}>
      {(['date', 'days'] as const).map((m) => (
        <label key={m} className={`segmented-control__option${mode === m ? ' is-checked' : ''}`}>
          <input type="radio" name={name} value={m} checked={mode === m} onChange={() => onMode(m)} />
          {words[m]}
        </label>
      ))}
    </fieldset>
  )
}
