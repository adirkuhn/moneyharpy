import type { ReactNode } from 'react'
import { FormRow, type Width } from '../forms/Form'
import { type SearchOption, SearchSelect } from '../forms/SearchSelect'

export type SelectOption = SearchOption

type Props = {
  label: string
  searchLabel: string
  emptyLabel: string
  value: string
  options: SelectOption[]
  onChange: (value: string) => void
  errors?: string[]
  help?: ReactNode
  width?: Width
}

/** A select whose long list can be narrowed by typing, in a form row. */
export function SearchableSelect({ label, searchLabel, emptyLabel, value, options, onChange, errors, help, width }: Props) {
  return (
    <FormRow label={label} errors={errors} help={help} width={width}>
      {({ id, labelId, ...described }) => (
        <SearchSelect
          id={id}
          labelledBy={labelId}
          searchLabel={searchLabel}
          emptyLabel={emptyLabel}
          value={value}
          options={options}
          onChange={onChange}
          aria-invalid={described['aria-invalid']}
          aria-describedby={described['aria-describedby']}
        />
      )}
    </FormRow>
  )
}
