import { type FormEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { type Administration, errorMessages } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { CheckboxInput, FormSection, formErrors, SelectInput, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { Block } from '../settings/Block'
import { type AccountType, accountTypes, type FinancialAccount, type ImportResult, useAddTransactions, useFinancialAccounts, useImportStatement, useSaveFinancialAccount } from './queries'

/** Adds a financial account (/financial_accounts/new) or shows one, with import and manual entry. */
export function FinancialAccountPage() {
  const { administrationId = '', accountId } = useParams()
  const administration = useAdministration(administrationId)
  const accounts = useFinancialAccounts(administrationId)
  if (!administration.data || !accounts.data) {
    return <Loading />
  }
  const account = accountId ? accounts.data.find((a) => a.id === accountId) : undefined
  if (accountId && !account) {
    return <NotFoundPage />
  }
  return <AccountView key={account?.id ?? 'new'} administration={administration.data} account={account ?? null} />
}

function AccountView({ administration, account }: { administration: Administration; account: FinancialAccount | null }) {
  const { t } = useTranslation()
  const back = `/${administration.id}/settings/bank#accounts`
  return (
    <Page title={account?.name ?? t('bank.newAccount')} back={{ to: back, label: t('bank.back') }}>
      <AccountForm administration={administration} account={account} back={back} />
      {account && (
        <>
          <ImportBlock administrationId={administration.id} account={account} />
          <ManualBlock administrationId={administration.id} account={account} />
        </>
      )}
    </Page>
  )
}

function AccountForm({ administration, account, back }: { administration: Administration; account: FinancialAccount | null; back: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const save = useSaveFinancialAccount(administration.id, account?.id)
  const [form, setForm] = useState({
    name: account?.name ?? '',
    type: (account?.type ?? 'BankAccount') as AccountType,
    identifier: account?.identifier ?? '',
    currency: account?.currency ?? administration.currency,
    active: account?.active ?? true,
  })
  const labels = { name: t('bank.accountName'), type: t('bank.accountType'), identifier: t('bank.identifier'), currency: t('workflow.currency') }
  const errors = formErrors(save.error, labels)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate(form, { onSuccess: (saved) => navigate(account ? back : `/${administration.id}/financial_accounts/${saved.id}`) })
  }
  return (
    <form className="form-sheet" onSubmit={submit} noValidate aria-label={t('bank.accountDetails')}>
      <Problems messages={errors.general} />
      <FormSection>
        <TextInput label={labels.name} required maxLength={255} autoComplete="off" value={form.name} errors={errors.field('name')} onChange={(e) => setForm({ ...form, name: e.target.value })} />
        <SelectInput label={labels.type} width="medium" value={form.type} errors={errors.field('type')} onChange={(e) => setForm({ ...form, type: e.target.value as AccountType })}>
          {accountTypes.map((type) => <option key={type} value={type}>{t(`bank.types.${type}`)}</option>)}
        </SelectInput>
        <TextInput
          label={labels.identifier}
          width="medium"
          maxLength={64}
          autoComplete="off"
          spellCheck={false}
          help={form.type === 'BankAccount' ? t('bank.ibanHelp') : t('bank.identifierHelp')}
          value={form.identifier}
          errors={errors.field('identifier')}
          onChange={(e) => setForm({ ...form, identifier: e.target.value })}
        />
        <TextInput label={labels.currency} width="small" maxLength={3} autoComplete="off" value={form.currency} errors={errors.field('currency')} onChange={(e) => setForm({ ...form, currency: e.target.value.toUpperCase() })} />
        {account && <CheckboxInput label={t('bookkeeping.active')} checked={form.active} onChange={(e) => setForm({ ...form, active: e.target.checked })} />}
      </FormSection>
      <div className="form-sheet__actions">
        <button className="btn btn--primary" disabled={save.isPending}>{t('settings.save')}</button>
        <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
      </div>
    </form>
  )
}

function ImportBlock({ administrationId, account }: { administrationId: string; account: FinancialAccount }) {
  const { t } = useTranslation()
  const upload = useImportStatement(administrationId, account.id ?? '')
  const inputId = useId()
  const [result, setResult] = useState<ImportResult | null>(null)
  return (
    <Block id="import" title={t('bank.import')} subtitle={t('bank.importSubtitle')}>
      <div className="card card-stack">
        <div className="file-field">
          <input
            id={inputId}
            type="file"
            className="sr-only"
            accept=".xml,.camt,.053,.052,.054,.sta,.940,.mt940,.txt"
            disabled={upload.isPending}
            onChange={(e) => {
              const file = e.target.files?.[0]
              e.target.value = ''
              if (file) {
                setResult(null)
                upload.mutate(file, { onSuccess: setResult })
              }
            }}
          />
          <label htmlFor={inputId} className="btn btn--accent file-field__pick">{upload.isPending ? t('bank.importing') : t('bank.chooseFile')}</label>
        </div>
        <Problems messages={errorMessages(upload.error, { file: t('bank.file') })} />
        {result && (
          <p role="status" className="card__footer">
            {t('bank.imported', { count: result.imported })}
            {result.duplicates > 0 && ` ${t('bank.duplicates', { count: result.duplicates })}`}
            {result.locked > 0 && ` ${t('bank.lockedSkipped', { count: result.locked })}`}
            {result.auto_booked > 0 && ` ${t('bank.autoBooked', { count: result.auto_booked })}`}{' '}
            <Link to={`/${administrationId}/financial_mutations?account=${account.id}`}>{t('bank.viewTransactions')}</Link>
          </p>
        )}
      </div>
    </Block>
  )
}

function ManualBlock({ administrationId, account }: { administrationId: string; account: FinancialAccount }) {
  const { t } = useTranslation()
  const add = useAddTransactions(administrationId)
  const today = new Date().toISOString().slice(0, 10)
  const empty = { date: today, direction: 'out', amount: '', message: '', contra_account_name: '', contra_account_number: '' }
  const [form, setForm] = useState(empty)
  const [added, setAdded] = useState(false)
  const labels = {
    'financial_mutations_attributes.0.date': t('bank.date'), 'financial_mutations_attributes.0.amount': t('bank.amount'),
    'financial_mutations_attributes.0.message': t('bank.description'), 'financial_mutations_attributes.0.contra_account_name': t('bank.contraName'),
    'financial_mutations_attributes.0.contra_account_number': t('bank.contraAccount'),
  }
  const errors = formErrors(add.error, labels)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    setAdded(false)
    const { direction, amount, ...rest } = form
    const value = amount.trim().replace(/^-/, '')
    add.mutate(
      { financial_account_id: account.id ?? '', reference: t('bank.manualReference', { date: form.date }), financial_mutations_attributes: [{ ...rest, amount: direction === 'out' ? `-${value}` : value }] },
      { onSuccess: () => { setForm(empty); setAdded(true) } },
    )
  }
  return (
    <Block id="add-transaction" title={t('bank.addTransaction')} subtitle={t('bank.addTransactionSubtitle')}>
      <form className="form-sheet form-sheet--block" onSubmit={submit} noValidate aria-label={t('bank.addTransaction')}>
        <Problems messages={errors.general} />
        <FormSection>
          <TextInput label={t('bank.date')} type="date" width="medium" required value={form.date} errors={errors.field('financial_mutations_attributes.0.date')} onChange={(e) => setForm({ ...form, date: e.target.value })} />
          <SelectInput label={t('bank.direction')} width="medium" value={form.direction} onChange={(e) => setForm({ ...form, direction: e.target.value })}>
            <option value="out">{t('bank.moneyOut')}</option>
            <option value="in">{t('bank.moneyIn')}</option>
          </SelectInput>
          <TextInput label={t('bank.amount')} inputMode="decimal" width="small" required maxLength={16} value={form.amount} errors={errors.field('financial_mutations_attributes.0.amount')} onChange={(e) => setForm({ ...form, amount: e.target.value })} />
          <TextInput label={t('bank.description')} maxLength={5000} value={form.message} errors={errors.field('financial_mutations_attributes.0.message')} onChange={(e) => setForm({ ...form, message: e.target.value })} />
          <TextInput label={t('bank.contraName')} maxLength={255} value={form.contra_account_name} errors={errors.field('financial_mutations_attributes.0.contra_account_name')} onChange={(e) => setForm({ ...form, contra_account_name: e.target.value })} />
          <TextInput label={t('bank.contraAccount')} width="medium" maxLength={64} spellCheck={false} value={form.contra_account_number} errors={errors.field('financial_mutations_attributes.0.contra_account_number')} onChange={(e) => setForm({ ...form, contra_account_number: e.target.value })} />
        </FormSection>
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={add.isPending}>{t('bank.addTransactionButton')}</button>
          {added && <span role="status" className="hint">{t('bank.transactionAdded')}</span>}
        </div>
      </form>
    </Block>
  )
}
