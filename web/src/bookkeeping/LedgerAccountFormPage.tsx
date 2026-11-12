import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { CheckboxInput, FormNotice, FormSection, formErrors, SelectInput, TextArea, TextInput } from '../forms/Form'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import {
  type AccountType, balanceSheetTypes, documentTypes, type DocumentType, type LedgerAccount, profitAndLossTypes, tree,
  useBookkeepingSettings, useDeleteLedgerAccount, useLedgerAccounts, useSaveLedgerAccount,
} from './queries'
import { TaxonomyPicker } from './TaxonomyPicker'

/** Edits a category (/ledger_accounts/:id); Add single category adds one. */
export function LedgerAccountFormPage() {
  const { administrationId = '', ledgerAccountId } = useParams()
  const settings = useBookkeepingSettings(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  if (!settings.data || !accounts.data) {
    return <Loading />
  }
  const account = ledgerAccountId ? accounts.data.find((a) => a.id === ledgerAccountId) : undefined
  if (ledgerAccountId && !account) {
    return <NotFoundPage />
  }
  return <LedgerAccountForm key={account?.id ?? 'new'} administrationId={administrationId} hasScheme={settings.data.chart_scheme !== ''} accounts={accounts.data} account={account ?? null} />
}

type Props = { administrationId: string; hasScheme: boolean; accounts: LedgerAccount[]; account: LedgerAccount | null }

function LedgerAccountForm({ administrationId, hasScheme, accounts, account }: Props) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const save = useSaveLedgerAccount(administrationId, account?.id)
  const remove = useDeleteLedgerAccount(administrationId)
  const system = Boolean(account?.system_role)
  const [form, setForm] = useState({
    name: account?.name ?? '',
    account_type: (account?.account_type ?? 'expenses') as AccountType,
    account_id: account?.account_id ?? '',
    parent_id: account?.parent_id ? String(account.parent_id) : '',
    description: account?.description ?? '',
    allowed_document_types: (account?.allowed_document_types ?? null) as DocumentType[] | null,
    active: account?.active ?? true,
    rgs_code: account?.taxonomy_item?.code ?? '',
  })
  const back = `/${administrationId}/settings/bookkeeping#categories`
  const labels = {
    name: t('bookkeeping.categoryName'), account_type: t('bookkeeping.accountType'), account_id: t('bookkeeping.accountNumber'),
    parent_id: t('bookkeeping.parent'), description: t('bookkeeping.description'), rgs_code: t('bookkeeping.schemeCode'),
    allowed_document_types: t('bookkeeping.documentTypes'),
  }
  const errors = formErrors(save.error ?? remove.error, labels)

  // A category cannot sit under itself or under one of its own sub-categories.
  const excluded = new Set<string>()
  if (account?.id) {
    for (const { account: a } of tree(accounts)) {
      if (a.id === account.id || (a.parent_id && excluded.has(String(a.parent_id)))) {
        excluded.add(a.id ?? '')
      }
    }
  }
  const parents = tree(accounts.filter((a) => a.account_type === form.account_type && !excluded.has(a.id ?? '')))

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const { rgs_code, allowed_document_types, ...rest } = form
    save.mutate(
      {
        ledger_account: { ...rest, ...(allowed_document_types ? { allowed_document_types } : {}), ...(system ? { account_type: undefined, active: undefined } : {}) },
        ...(hasScheme ? { rgs_code } : {}),
      },
      { onSuccess: () => navigate(back) },
    )
  }
  const typeOption = (type: AccountType) => <option key={type} value={type}>{t(`bookkeeping.accountTypes.${type}`)}</option>
  const docs = form.allowed_document_types ?? []

  const deleteAction = account && !system && (
    account.active ? (
      <button type="button" className="btn btn--secondary" disabled={remove.isPending} onClick={() => remove.mutate(account.id ?? '', { onSuccess: () => navigate(back) })}>{t('bookkeeping.deactivate')}</button>
    ) : (
      <ConfirmDelete label={t('settings.delete')} confirmLabel={t('settings.confirmDelete')} disabled={remove.isPending} onConfirm={() => remove.mutate(account.id ?? '', { onSuccess: () => navigate(back) })} />
    )
  )

  return (
    <Page title={t('bookkeeping.editCategory')} back={{ to: back, label: t('bookkeeping.back') }} actions={deleteAction || undefined}>
      <form className="form-sheet" onSubmit={submit} noValidate>
        {system && <FormNotice>{t('bookkeeping.systemCategory')}</FormNotice>}
        <Problems messages={errors.general} />
        <FormSection>
          <TextInput label={labels.name} required maxLength={255} autoComplete="off" value={form.name} errors={errors.field('name')} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <SelectInput label={labels.account_type} width="medium" value={form.account_type} disabled={system} errors={errors.field('account_type')} onChange={(e) => setForm({ ...form, account_type: e.target.value as AccountType, parent_id: '', allowed_document_types: null })}>
            <optgroup label={t('bookkeeping.profitAndLoss')}>{profitAndLossTypes.map(typeOption)}</optgroup>
            <optgroup label={t('bookkeeping.balanceSheet')}>{balanceSheetTypes.map(typeOption)}</optgroup>
          </SelectInput>
          <TextInput label={labels.account_id} width="small" maxLength={20} autoComplete="off" value={form.account_id} errors={errors.field('account_id')} onChange={(e) => setForm({ ...form, account_id: e.target.value })} />
          <SelectInput label={labels.parent_id} width="medium" value={form.parent_id} errors={errors.field('parent_id')} onChange={(e) => setForm({ ...form, parent_id: e.target.value })}>
            <option value="">{t('bookkeeping.noParent')}</option>
            {parents.map(({ account: a, depth }) => <option key={a.id} value={a.id}>{`${' '.repeat(depth)}${a.account_id ? `${a.account_id} ` : ''}${a.name}`}</option>)}
          </SelectInput>
          {hasScheme && (
            <TaxonomyPicker
              administrationId={administrationId}
              accountType={form.account_type}
              label={labels.rgs_code}
              value={form.rgs_code}
              currentName={account?.taxonomy_item?.name}
              errors={errors.field('rgs_code')}
              onChange={(rgs_code) => setForm({ ...form, rgs_code })}
            />
          )}
          <TextArea label={labels.description} maxLength={1000} value={form.description} errors={errors.field('description')} onChange={(e) => setForm({ ...form, description: e.target.value })} />
          {!system && <CheckboxInput label={t('bookkeeping.active')} checked={form.active} onChange={(e) => setForm({ ...form, active: e.target.checked })} />}
        </FormSection>
        {account && (
          <FormSection title={labels.allowed_document_types}>
            {documentTypes.map((d) => (
              <CheckboxInput
                key={d}
                label={t(`bookkeeping.documentTypeNames.${d}`)}
                checked={docs.includes(d)}
                onChange={(e) => setForm({ ...form, allowed_document_types: e.target.checked ? [...docs, d] : docs.filter((x) => x !== d) })}
              />
            ))}
          </FormSection>
        )}
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={save.isPending}>{t('settings.save')}</button>
          <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}
