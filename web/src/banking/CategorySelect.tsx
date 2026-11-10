import { useTranslation } from 'react-i18next'
import { SelectInput } from '../forms/Form'
import { useLedgerAccounts } from '../bookkeeping/queries'
import { bookable } from './queries'

type Props = { administrationId: string; label: string; value: string; errors?: string[]; onChange: (id: string) => void }

/** Picks a category transactions can be booked on, grouped by type. */
export function CategorySelect({ administrationId, label, value, errors, onChange }: Props) {
  const { t } = useTranslation()
  const accounts = useLedgerAccounts(administrationId)
  return (
    <SelectInput label={label} width="long" value={value} errors={errors} onChange={(e) => onChange(e.target.value)}>
      <option value="">{t('bank.chooseCategory')}</option>
      {bookable(accounts.data ?? []).map((g) => (
        <optgroup key={g.type} label={t(`bookkeeping.accountTypes.${g.type}`)}>
          {g.rows.map(({ account: a, depth }) => (
            <option key={a.id} value={a.id}>{`${' '.repeat(depth)}${a.account_id ? `${a.account_id} ` : ''}${a.name}`}</option>
          ))}
        </optgroup>
      ))}
    </SelectInput>
  )
}
