import { type DragEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { formErrors } from '../forms/Form'
import { ListIcon } from '../layout/icons'
import { useImportStatementFile } from './queries'
import './accounts.css'

type AddProps = { id: string; administrationId: string; manualTo: string; onImported: (accountId: string) => void }

/** Moneybird's Add transactions: a field to drop a CAMT.053 or MT940 file on, or a way to add them by hand. */
export function AddTransactions({ id, administrationId, manualTo, onImported }: AddProps) {
  const { t } = useTranslation()
  const upload = useImportStatementFile(administrationId)
  const [over, setOver] = useState(false)
  const take = (files: FileList | null) => {
    const file = files?.[0]
    if (file) {
      upload.mutate(file, { onSuccess: (res) => onImported(res.financial_account_id) })
    }
  }
  const drop = (e: DragEvent) => {
    e.preventDefault()
    setOver(false)
    take(e.dataTransfer.files)
  }
  const errors = formErrors(upload.error, {})
  return (
    <section id={id} className="add-transactions" aria-label={t('bank.accounts.addTransactions')}>
      <label
        className={`add-transactions__drop${over ? ' is-over' : ''}`}
        onDragOver={(e) => {
          e.preventDefault()
          setOver(true)
        }}
        onDragLeave={() => setOver(false)}
        onDrop={drop}
      >
        <span className="add-transactions__icon" aria-hidden="true"><ListIcon kind="upload" /></span>
        <span className="add-transactions__title">{upload.isPending ? t('bank.accounts.importing') : t('bank.accounts.dropTitle')}</span>
        <span className="add-transactions__subtitle">{t('bank.accounts.dropSubtitle')}</span>
        <input
          className="sr-only"
          type="file"
          accept=".xml,.sta,.940,.mt940,.txt,.camt,application/xml,text/xml,text/plain"
          aria-label={t('bank.accounts.dropTitle')}
          disabled={upload.isPending}
          onChange={(e) => {
            take(e.target.files)
            e.target.value = ''
          }}
        />
      </label>
      <Problems messages={[...errors.general, ...(errors.field('file') ?? [])]} />
      <p className="add-transactions__manual">
        {t('bank.accounts.manualText')} <Link to={manualTo}>{t('bank.accounts.manualLink')}</Link>
      </p>
    </section>
  )
}
