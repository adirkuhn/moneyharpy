import { type FormEvent, useDeferredValue, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { formErrors, SelectInput, TextArea, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { type CategoryFlow, useCategoryFlow } from './flow'
import { type AccountType, balanceSheetTypes, type LedgerAccount, profitAndLossTypes, type TaxonomyItem, tree, useBookkeepingSettings, useLedgerAccounts, useSaveLedgerAccount, useTaxonomySearch } from './queries'
import { TaxonomyPicker } from './TaxonomyPicker'
import './categories.css'

type Side = 'profit_and_loss' | 'balance_sheet'
const typesOf: Record<Side, AccountType[]> = { profit_and_loss: profitAndLossTypes, balance_sheet: balanceSheetTypes }
const sideOf = (type: AccountType): Side => (balanceSheetTypes.includes(type) ? 'balance_sheet' : 'profit_and_loss')

/**
 * Moneybird's Add single category (/ledger_accounts/new): the category on the
 * left, and on the right the chart scheme code suggested for its name, or the
 * type and code chosen by hand.
 */
export function SingleCategoryPage() {
  const { administrationId = '' } = useParams()
  const settings = useBookkeepingSettings(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  const flow = useCategoryFlow(administrationId)
  if (!settings.data || !accounts.data) {
    return <Loading />
  }
  return <SingleCategory administrationId={administrationId} hasScheme={settings.data.chart_scheme !== ''} accounts={accounts.data} flow={flow} />
}

type Props = { administrationId: string; hasScheme: boolean; accounts: LedgerAccount[]; flow: CategoryFlow }

function SingleCategory({ administrationId, hasScheme, accounts, flow }: Props) {
  const { t } = useTranslation()
  const b = (key: string, options: Record<string, unknown> = {}) => t(`bookkeeping.${key}`, options)
  const navigate = useNavigate()
  const formLabel = useId()
  const save = useSaveLedgerAccount(administrationId)
  const [form, setForm] = useState({
    name: '',
    account_type: (flow.forInvoice ? 'revenue' : 'expenses') as AccountType,
    account_id: '',
    parent_id: '',
    description: '',
    rgs_code: '',
  })
  // The code chosen from the suggestions, so its name can be shown on the left.
  const [chosen, setChosen] = useState<TaxonomyItem | null>(null)
  const [manual, setManual] = useState(!hasScheme)
  const labels = {
    name: b('categoryName'), account_type: b('single.type'), account_id: b('accountNumber'), parent_id: b('parent'),
    description: b('description'), rgs_code: b('schemeCode'),
  }
  const errors = formErrors(save.error, labels)
  const parents = tree(accounts.filter((a) => a.account_type === form.account_type))
  const side = sideOf(form.account_type)

  const setType = (account_type: AccountType) => {
    setForm({ ...form, account_type, parent_id: '', rgs_code: '' })
    setChosen(null)
  }
  const setCode = (rgs_code: string, item: TaxonomyItem | null = null) => {
    setForm({ ...form, rgs_code })
    setChosen(item)
  }
  const submit = (e: FormEvent) => {
    e.preventDefault()
    const { rgs_code, ...ledger_account } = form
    save.mutate({ ledger_account, ...(hasScheme ? { rgs_code } : {}) }, { onSuccess: (saved) => navigate(flow.returnTo, { state: flow.stateAfter([saved]) }) })
  }

  return (
    <Page title={b('single.title')} back={{ to: `/${administrationId}/ledger_accounts/blueprints${flow.query}`, label: b('single.back') }}>
      <div className="single-category">
        <form className="category-card single-category__form" aria-labelledby={formLabel} onSubmit={submit} noValidate>
          <span id={formLabel} hidden>{b('single.title')}</span>
          <div className="single-category__chosen">
            <p className="single-category__side">{b(`theme.${side === 'balance_sheet' ? 'balanceSheet' : 'profitAndLoss'}`)}</p>
            <p>{b(`accountTypes.${form.account_type}`)}</p>
            {form.rgs_code && <p className="hint">{form.rgs_code}{chosen && chosen.code === form.rgs_code ? ` · ${chosen.name}` : ''}</p>}
          </div>
          <Problems messages={errors.general} />
          <TextInput label={labels.name} required maxLength={255} autoComplete="off" value={form.name} errors={errors.field('name')} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <TextArea label={labels.description} maxLength={1000} value={form.description} help={b('single.optional')} errors={errors.field('description')} onChange={(e) => setForm({ ...form, description: e.target.value })} />
          <SelectInput label={labels.parent_id} value={form.parent_id} errors={errors.field('parent_id')} onChange={(e) => setForm({ ...form, parent_id: e.target.value })}>
            <option value="">{b('noParent')}</option>
            {parents.map(({ account: a, depth }) => <option key={a.id} value={a.id}>{`${' '.repeat(depth)}${a.account_id ? `${a.account_id} ` : ''}${a.name}`}</option>)}
          </SelectInput>
          <TextInput label={labels.account_id} width="small" maxLength={20} autoComplete="off" value={form.account_id} help={b('single.optional')} errors={errors.field('account_id')} onChange={(e) => setForm({ ...form, account_id: e.target.value })} />
          <div className="category-card__actions">
            <button className="btn btn--primary" disabled={save.isPending}>{t('settings.save')}</button>
            <Link className="btn btn--secondary" to={flow.returnTo} state={flow.returnState}>{t('common.cancel')}</Link>
          </div>
        </form>
        <section className="category-card single-category__picker" aria-label={b('single.select')}>
          {manual ? (
            <Manual
              administrationId={administrationId}
              hasScheme={hasScheme}
              type={form.account_type}
              code={form.rgs_code}
              errors={[...(errors.field('account_type') ?? []), ...(errors.field('rgs_code') ?? [])]}
              onBack={hasScheme ? () => setManual(false) : undefined}
              onType={setType}
              onCode={(code) => setCode(code)}
            />
          ) : (
            <Suggestions
              administrationId={administrationId}
              name={form.name}
              type={form.account_type}
              code={form.rgs_code}
              errors={errors.field('rgs_code')}
              onChoose={(item) => setCode(item.code ?? '', item)}
              onManual={() => setManual(true)}
            />
          )}
        </section>
      </div>
    </Page>
  )
}

type SuggestionsProps = {
  administrationId: string
  name: string
  type: AccountType
  code: string
  errors?: string[]
  onChoose: (item: TaxonomyItem) => void
  onManual: () => void
}

/** Codes of the chart scheme whose name matches the category's name, as Moneybird suggests them. */
function Suggestions({ administrationId, name, type, code, errors, onChoose, onManual }: SuggestionsProps) {
  const { t } = useTranslation()
  const b = (key: string) => t(`bookkeeping.single.${key}`)
  const query = useDeferredValue(name.trim())
  const items = useTaxonomySearch(administrationId, query, type)
  const heading = useId()
  const manualLink = (
    <button type="button" className="link-button" onClick={onManual}>{b('manual')}</button>
  )
  if (query === '') {
    return (
      <div className="single-category__empty">
        <p>{b('suggest')}</p>
        <p className="hint">{b('or')}</p>
        {manualLink}
        {errors && <p className="form-input__message form-input__message--error">{errors.join(' ')}</p>}
      </div>
    )
  }
  const found = items.data ?? []
  return (
    <div className="single-category__suggestions">
      <h3 id={heading} className="single-category__title">{b('suggestions')}</h3>
      {errors && <p className="form-input__message form-input__message--error">{errors.join(' ')}</p>}
      {found.length === 0 ? (
        <p className="hint">{items.isFetching ? '' : b('noSuggestions')}</p>
      ) : (
        <div className="single-category__options" role="radiogroup" aria-labelledby={heading}>
          {found.map((item) => (
            <label key={item.code} className={`single-category__option${item.code === code ? ' is-checked' : ''}`}>
              <input type="radio" name="suggestion" checked={item.code === code} onChange={() => onChoose(item)} />
              <span className="code-chip">{item.code}</span>
              <span>{item.name}</span>
            </label>
          ))}
        </div>
      )}
      <p className="hint">{b('or')} {manualLink}</p>
    </div>
  )
}

type ManualProps = {
  administrationId: string
  hasScheme: boolean
  type: AccountType
  code: string
  errors: string[]
  onBack?: () => void
  onType: (type: AccountType) => void
  onCode: (code: string) => void
}

/** The part of the books, the type and, with a chart scheme, the code, chosen by hand. */
function Manual({ administrationId, hasScheme, type, code, errors, onBack, onType, onCode }: ManualProps) {
  const { t } = useTranslation()
  const b = (key: string) => t(`bookkeeping.${key}`)
  const [side, setSide] = useState<Side>(sideOf(type))
  const sideName = useId()
  const typeLabel = useId()
  const pick = (next: Side) => {
    setSide(next)
    if (sideOf(type) !== next) {
      onType(typesOf[next][0]!)
    }
  }
  return (
    <div className="single-category__manual">
      <div className="single-category__manual-header">
        {onBack && <button type="button" className="btn btn--secondary btn--small" onClick={onBack}>{b('single.goBack')}</button>}
        <h3 className="single-category__title">{b('single.select')}</h3>
      </div>
      <fieldset className="segmented">
        <legend className="sr-only">{b('single.side')}</legend>
        <div className="segmented__options">
          {(['profit_and_loss', 'balance_sheet'] as Side[]).map((s) => (
            <label key={s} className={`segmented__option${side === s ? ' is-checked' : ''}`}>
              <input type="radio" name={sideName} checked={side === s} onChange={() => pick(s)} />
              {b(`theme.${s === 'balance_sheet' ? 'balanceSheet' : 'profitAndLoss'}`)}
            </label>
          ))}
        </div>
      </fieldset>
      <div className="single-category__options" role="radiogroup" aria-labelledby={typeLabel}>
        <span id={typeLabel} className="sr-only">{b('single.type')}</span>
        {typesOf[side].map((ty) => (
          <label key={ty} className={`single-category__option${ty === type ? ' is-checked' : ''}`}>
            <input type="radio" name={typeLabel} checked={ty === type} onChange={() => onType(ty)} />
            <span>{b(`accountTypes.${ty}`)}</span>
          </label>
        ))}
      </div>
      {hasScheme && (
        <TaxonomyPicker administrationId={administrationId} accountType={type} label={b('schemeCode')} value={code} onChange={onCode} />
      )}
      {errors.length > 0 && <p className="form-input__message form-input__message--error">{errors.join(' ')}</p>}
    </div>
  )
}
