import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { useLedgerAccounts, categoryOption } from '../bookkeeping/queries'
import { bookable, formatAmount } from '../banking/queries'
import { ConfirmPanel } from '../detail/Detail'
import { DateInput } from '../forms/DatePicker'
import { FormSection, formErrors, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { MoreMenu } from '../list/ListActions'
import { type JournalDocument, parseAmount, useDeleteJournalDocument, useJournalDocument, useSaveJournalDocument } from './queries'

type Row = { id?: string; ledger_account_id: string; debit: string; credit: string; description: string }

const blank = (): Row => ({ ledger_account_id: '', debit: '', credit: '', description: '' })
const amountText = (v: string | undefined) => (Number(v ?? 0) === 0 ? '' : String(Number(v)))

/** Books a journal entry by hand (/documents/general_journal_documents/new) or changes one (/…/:id). */
export function JournalDocumentPage() {
  const { administrationId = '', documentId } = useParams()
  const doc = useJournalDocument(administrationId, documentId)
  if (documentId && doc.error instanceof ApiError && doc.error.status < 500) {
    return <NotFoundPage />
  }
  if (documentId && !doc.data) {
    return <Loading />
  }
  return <JournalForm key={doc.data?.id ?? 'new'} administrationId={administrationId} doc={doc.data ?? null} />
}

function JournalForm({ administrationId, doc }: { administrationId: string; doc: JournalDocument | null }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const remove = useDeleteJournalDocument(administrationId, doc?.id ?? '')
  const [confirming, setConfirming] = useState(false)
  const list = `/${administrationId}/documents/general_journal_documents`
  const actions = doc ? <MoreMenu items={[{ label: t('journal.delete'), danger: true, onSelect: () => setConfirming(true) }]} /> : undefined

  return (
    <Page title={doc?.reference || t('journal.newEntry')} subtitle={t('journal.entrySubtitle')} back={{ to: list, label: t('journal.backToEntries') }} actions={actions}>
      <Problems messages={formErrors(remove.error, {}).general} />
      {confirming && (
        <ConfirmPanel
          message={t('journal.confirmDeleteText')}
          confirmLabel={t('journal.confirmDelete')}
          disabled={remove.isPending}
          onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(list) })}
          onCancel={() => setConfirming(false)}
        />
      )}
      <JournalEntryForm administrationId={administrationId} doc={doc} cancelTo={list} onSaved={(saved) => navigate(`${list}/${saved.id}`)} />
    </Page>
  )
}

type FormProps = {
  administrationId: string
  doc: JournalDocument | null
  /** Where Cancel leads. */
  cancelTo: string
  onSaved: (saved: JournalDocument) => void
  /** The form sits in another card, such as the right column of an incoming document, without a sheet of its own. */
  embedded?: boolean
}

/** The reference, date and balanced rows of a journal entry, saved as one. */
export function JournalEntryForm({ administrationId, doc, cancelTo, onSaved, embedded = false }: FormProps) {
  const { t, i18n } = useTranslation()
  const administration = useAdministration(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  const save = useSaveJournalDocument(administrationId, doc?.id)
  const [reference, setReference] = useState(doc?.reference ?? '')
  const [date, setDate] = useState(doc?.date ?? new Date().toISOString().slice(0, 10))
  const [rows, setRows] = useState<Row[]>(() =>
    doc
      ? (doc.general_journal_document_entries ?? []).map((r) => ({ id: r.id ?? '', ledger_account_id: r.ledger_account_id ?? '', debit: amountText(r.debit), credit: amountText(r.credit), description: r.description ?? '' }))
      : [blank(), blank()],
  )
  const [removed, setRemoved] = useState<string[]>([])
  const currency = administration.data?.currency ?? 'EUR'
  const money = (n: number) => formatAmount(n, currency, i18n.language)
  const labels = { reference: t('journal.reference'), date: t('journal.date') }
  const errors = formErrors(save.error, labels)
  const groups = bookable(accounts.data ?? [], 'general_journal_document')

  const amounts = rows.map((r) => ({ debit: parseAmount(r.debit), credit: parseAmount(r.credit) }))
  const valid = amounts.every((a) => a.debit !== null && a.credit !== null)
  const debit = amounts.reduce((s, a) => s + (a.debit ?? 0), 0)
  const credit = amounts.reduce((s, a) => s + (a.credit ?? 0), 0)
  const difference = Math.round((debit - credit) * 100) / 100
  const used = rows.filter((r) => r.ledger_account_id || r.debit || r.credit)
  const ready = valid && difference === 0 && debit > 0 && used.length >= 2 && used.every((r) => r.ledger_account_id)

  const set = (i: number, change: Partial<Row>) => setRows(rows.map((r, j) => (j === i ? { ...r, ...change } : r)))
  const drop = (i: number) => {
    const row = rows[i]
    if (row?.id) {
      setRemoved([...removed, row.id])
    }
    setRows(rows.filter((_, j) => j !== i))
  }
  const submit = (e: FormEvent) => {
    e.preventDefault()
    const entries = [
      ...used.map((r) => ({ ...(r.id ? { id: r.id } : {}), ledger_account_id: r.ledger_account_id, debit: r.debit.trim(), credit: r.credit.trim(), description: r.description.trim() })),
      ...removed.map((id) => ({ id, _destroy: true })),
    ]
    save.mutate({ reference, date, general_journal_document_entries_attributes: entries }, { onSuccess: onSaved })
  }

  return (
    <form className={embedded ? 'journal-entry-form' : 'form-sheet'} onSubmit={submit} noValidate aria-label={t('journal.entryTitle')}>
      <Problems messages={errors.general} />
      <FormSection>
        <TextInput label={labels.reference} required maxLength={255} value={reference} errors={errors.field('reference')} onChange={(e) => setReference(e.target.value)} />
        <DateInput label={labels.date} width="medium" required value={date} errors={errors.field('date')} onChange={setDate} />
      </FormSection>
      <FormSection title={t('journal.rows')}>
      <table className="data-table journal-rows" aria-label={t('journal.rows')}>
        <thead>
          <tr>
            <th scope="col">{t('journal.category')}</th>
            <th scope="col" className="data-table__extra">{t('journal.description')}</th>
            <th scope="col" className="journal-rows__amount">{t('journal.debit')}</th>
            <th scope="col" className="journal-rows__amount">{t('journal.credit')}</th>
            <th scope="col"><span className="sr-only">{t('journal.actions')}</span></th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={r.id ?? `new-${i}`}>
              <td>
                <select aria-label={t('journal.category')} value={r.ledger_account_id} onChange={(e) => set(i, { ledger_account_id: e.target.value })}>
                  <option value="">{t('bank.chooseCategory')}</option>
                  {groups.map((g) => (
                    <optgroup key={g.type} label={t(`bookkeeping.accountTypes.${g.type}`)}>
                      {g.rows.map(({ account: a, depth }) => (
                        <option key={a.id} value={a.id}>{categoryOption({ account: a, depth })}</option>
                      ))}
                    </optgroup>
                  ))}
                </select>
              </td>
              <td className="data-table__extra">
                <input type="text" aria-label={t('journal.description')} maxLength={1000} value={r.description} onChange={(e) => set(i, { description: e.target.value })} />
              </td>
              <td className="journal-rows__amount">
                <input type="text" inputMode="decimal" aria-label={t('journal.debit')} aria-invalid={amounts[i]?.debit === null} value={r.debit} onChange={(e) => set(i, { debit: e.target.value, credit: e.target.value ? '' : r.credit })} />
              </td>
              <td className="journal-rows__amount">
                <input type="text" inputMode="decimal" aria-label={t('journal.credit')} aria-invalid={amounts[i]?.credit === null} value={r.credit} onChange={(e) => set(i, { credit: e.target.value, debit: e.target.value ? '' : r.debit })} />
              </td>
              <td>
                {rows.length > 2 && (
                  <button type="button" className="btn btn--secondary btn--small" aria-label={t('journal.removeRow', { n: i + 1 })} onClick={() => drop(i)}>
                    {t('journal.remove')}
                  </button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
        <tfoot>
          <tr className="ledger-report__total">
            <th scope="row">
              <button type="button" className="icon-link icon-link--button" onClick={() => setRows([...rows, blank()])}>{t('journal.addRow')}</button>
            </th>
            <td className="data-table__extra" />
            <td className="journal-rows__amount">{money(debit)}</td>
            <td className="journal-rows__amount">{money(credit)}</td>
            <td />
          </tr>
        </tfoot>
      </table>
      <p className={difference === 0 ? 'hint' : 'journal-rows__difference'} role="status">
        {difference === 0 ? t('journal.balanced') : t('journal.difference', { amount: money(Math.abs(difference)) })}
      </p>
      </FormSection>
      <div className="form-sheet__actions">
        <button className="btn btn--primary" disabled={save.isPending || !ready}>{t('journal.save')}</button>
        <Link className="btn btn--secondary" to={cancelTo}>{t('common.cancel')}</Link>
      </div>
    </form>
  )
}
