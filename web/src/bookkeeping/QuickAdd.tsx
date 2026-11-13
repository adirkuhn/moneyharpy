import { type FormEvent, type ReactNode, useId, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { formErrors, SelectInput, TextInput } from '../forms/Form'
import { Modal } from '../layout/Modal'
import { TaxonomyPicker } from './TaxonomyPicker'
import {
  type AccountType, categoryOption, type DocumentType, type LedgerAccount, tree, useBookkeepingSettings, useSaveLedgerAccount, useSaveTaxRate,
} from './queries'

type Added = { onAdded: (id: string) => void; onClose: () => void }

/** A popup with a short form, its Save in the footer. */
function QuickAddDialog({ title, busy, onSubmit, onClose, children }: { title: string; busy: boolean; onSubmit: () => void; onClose: () => void; children: ReactNode }) {
  const { t } = useTranslation()
  const formId = useId()
  const submit = (e: FormEvent) => {
    e.preventDefault()
    // The form sits inside the details editor's form; only this one saves.
    e.stopPropagation()
    onSubmit()
  }
  // Laid over the page outside the form the lines are in, as forms cannot nest.
  return createPortal(
    <Modal
      title={title}
      onClose={onClose}
      className="quick-add"
      footer={
        <>
          <button type="submit" form={formId} className="btn btn--primary" disabled={busy}>{t('settings.save')}</button>
          <button type="button" className="btn btn--secondary" onClick={onClose}>{t('common.cancel')}</button>
        </>
      }
    >
      <form id={formId} className="form-sheet form-sheet--compact" onSubmit={submit} noValidate>
        {children}
      </form>
    </Modal>,
    document.body,
  )
}

/**
 * "Add VAT rate..." of a line: a name and percentage, for the documents the
 * line is on. The other settings of a rate are on the VAT rates settings page.
 */
export function AddTaxRateDialog({ administrationId, documentType, onAdded, onClose }: Added & { administrationId: string; documentType: DocumentType }) {
  const { t } = useTranslation()
  const save = useSaveTaxRate(administrationId)
  const [form, setForm] = useState({ name: '', percentage: '' })
  const labels = { name: t('bookkeeping.rateName'), percentage: t('bookkeeping.percentage') }
  const errors = formErrors(save.error, labels)
  const type = documentType === 'sales_invoice' ? 'sales_invoice' : 'purchase_invoice'
  return (
    <QuickAddDialog
      title={t('invoices.details.newRate')}
      busy={save.isPending}
      onClose={onClose}
      onSubmit={() => save.mutate({ ...form, tax_rate_type: type, show_tax: true, active: true }, { onSuccess: (r) => onAdded(String(r.id)) })}
    >
      <Problems messages={errors.general} />
      <TextInput label={labels.name} required maxLength={255} autoComplete="off" data-autofocus value={form.name} errors={errors.field('name')} onChange={(e) => setForm({ ...form, name: e.target.value })} />
      <TextInput label={labels.percentage} required inputMode="decimal" width="small" maxLength={6} help="%" value={form.percentage} errors={errors.field('percentage')} onChange={(e) => setForm({ ...form, percentage: e.target.value })} />
    </QuickAddDialog>
  )
}

/** The kinds of category a line of these documents can be booked on, as the groups of its category select. */
export function lineAccountTypes(documentType: DocumentType): AccountType[] {
  return documentType === 'sales_invoice' ? ['revenue', 'other_income_expenses'] : ['direct_costs', 'expenses', 'non_current_assets', 'other_income_expenses']
}

/** "Add Category...": a name, type, parent and code, and the chart scheme code when the books keep one. */
export function AddCategoryDialog({ administrationId, documentType, accounts, onAdded, onClose }: Added & { administrationId: string; documentType: DocumentType; accounts: LedgerAccount[] }) {
  const { t } = useTranslation()
  const save = useSaveLedgerAccount(administrationId)
  const settings = useBookkeepingSettings(administrationId)
  const hasScheme = Boolean(settings.data?.chart_scheme)
  const types = lineAccountTypes(documentType)
  const [form, setForm] = useState({ name: '', account_type: types[0] as AccountType, parent_id: '', account_id: '', rgs_code: '' })
  const labels = {
    name: t('bookkeeping.categoryName'), account_type: t('bookkeeping.accountType'), parent_id: t('bookkeeping.parent'),
    account_id: t('invoices.details.categoryCode'), rgs_code: t('bookkeeping.schemeCode'),
  }
  const errors = formErrors(save.error, labels)
  const parents = tree(accounts.filter((a) => a.active && a.account_type === form.account_type))
  const submit = () => {
    const { rgs_code, ...ledger_account } = form
    save.mutate({ ledger_account, ...(hasScheme ? { rgs_code } : {}) }, { onSuccess: (a) => onAdded(String(a.id)) })
  }
  return (
    <QuickAddDialog title={t('invoices.details.newCategory')} busy={save.isPending} onClose={onClose} onSubmit={submit}>
      <Problems messages={errors.general} />
      <TextInput label={labels.name} required maxLength={255} autoComplete="off" data-autofocus value={form.name} errors={errors.field('name')} onChange={(e) => setForm({ ...form, name: e.target.value })} />
      <SelectInput label={labels.account_type} value={form.account_type} errors={errors.field('account_type')} onChange={(e) => setForm({ ...form, account_type: e.target.value as AccountType, parent_id: '' })}>
        {types.map((type) => <option key={type} value={type}>{t(`bookkeeping.accountTypes.${type}`)}</option>)}
      </SelectInput>
      <SelectInput label={labels.parent_id} value={form.parent_id} errors={errors.field('parent_id')} onChange={(e) => setForm({ ...form, parent_id: e.target.value })}>
        <option value="">{t('bookkeeping.noParent')}</option>
        {parents.map(({ account: a, depth }) => <option key={a.id} value={a.id}>{categoryOption({ account: a, depth })}</option>)}
      </SelectInput>
      <TextInput label={labels.account_id} width="small" maxLength={20} autoComplete="off" value={form.account_id} errors={errors.field('account_id')} onChange={(e) => setForm({ ...form, account_id: e.target.value })} />
      {hasScheme && (
        <TaxonomyPicker administrationId={administrationId} accountType={form.account_type} label={labels.rgs_code} value={form.rgs_code} errors={errors.field('rgs_code')} onChange={(rgs_code) => setForm({ ...form, rgs_code })} />
      )}
    </QuickAddDialog>
  )
}
