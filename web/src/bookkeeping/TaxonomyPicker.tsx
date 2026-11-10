import { useDeferredValue, useId } from 'react'
import { useTranslation } from 'react-i18next'
import { TextInput } from '../forms/Form'
import { type AccountType, useTaxonomySearch } from './queries'

type Props = {
  administrationId: string
  accountType?: AccountType
  value: string
  /** The name of the current code, shown until a search finds it. */
  currentName?: string
  label: string
  errors?: string[]
  onChange: (code: string) => void
}

/** A code field that suggests codes of the chart scheme as people type a code or a name. */
export function TaxonomyPicker({ administrationId, accountType, value, currentName, label, errors, onChange }: Props) {
  const { t } = useTranslation()
  const listId = useId()
  const query = useDeferredValue(value)
  const items = useTaxonomySearch(administrationId, query, accountType)
  const match = items.data?.find((i) => i.code === value)
  const help = match ? match.name : value && currentName ? currentName : t('bookkeeping.schemeCodeHelp')
  return (
    <>
      <TextInput label={label} list={listId} autoComplete="off" spellCheck={false} maxLength={32} value={value} help={help} errors={errors} onChange={(e) => onChange(e.target.value.trim())} />
      <datalist id={listId}>
        {items.data?.map((i) => <option key={i.code} value={i.code}>{i.name}</option>)}
      </datalist>
    </>
  )
}
