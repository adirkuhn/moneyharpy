import { type FormEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { formErrors, FormRow, SelectInput, TextInput } from '../forms/Form'
import { InfoIcon } from '../layout/icons'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { type CategoryFlow, useCategoryFlow } from './flow'
import { type AccountType, balanceSheetTypes, type LedgerAccount, profitAndLossTypes, type Template, type TemplateAccount, tree, useApplyTemplate, useLedgerAccounts, useTemplates } from './queries'
import './categories.css'

/** Where the name a theme asks for goes in its categories' names. */
const placeholder = '{name}'

/**
 * Moneybird's "Add category from <theme>"
 * (/ledger_accounts/blueprint_ledger_accounts/new?ledger_account_set=<key>):
 * the theme's categories by part of the books to tick one by one, or the one
 * category a theme such as Revenue adds. Saving adds them, then goes back.
 */
export function ThemePage() {
  const { administrationId = '' } = useParams()
  const [params] = useSearchParams()
  const flow = useCategoryFlow(administrationId)
  const templates = useTemplates(administrationId)
  const accounts = useLedgerAccounts(administrationId)
  if (!templates.data || !accounts.data) {
    return <Loading />
  }
  const template = templates.data.find((tpl) => tpl.key === params.get('ledger_account_set'))
  if (!template) {
    return <NotFoundPage />
  }
  return template.single
    ? <SingleTheme administrationId={administrationId} template={template} single={template.single} accounts={accounts.data} flow={flow} />
    : <CategoriesTheme administrationId={administrationId} template={template} flow={flow} />
}

type ThemeProps = { administrationId: string; template: Template; flow: CategoryFlow }

/** The page around a theme's form: its title, intro, warning and the buttons under it. */
function ThemeForm({ template, flow, pending, onSubmit, children }: Omit<ThemeProps, 'administrationId'> & { pending: boolean; onSubmit: () => void; children: React.ReactNode }) {
  const { t } = useTranslation()
  const submit = (e: FormEvent) => {
    e.preventDefault()
    onSubmit()
  }
  const title = flow.forInvoice ? t('bookkeeping.theme.titleFor', { name: template.name }) : t('bookkeeping.theme.title', { name: template.name })
  return (
    <Page title={title}>
      <form className="category-card category-theme" aria-label={title} onSubmit={submit} noValidate>
        {template.intro && <p>{template.intro}</p>}
        {template.warning && <p><strong>{t('bookkeeping.theme.warning')}</strong> {template.warning}</p>}
        {children}
        <div className="category-card__actions category-card__actions--center">
          <button className="btn btn--primary" disabled={pending}>{flow.forInvoice ? t('bookkeeping.theme.saveBack') : t('settings.save')}</button>
          <Link className="btn btn--secondary" to={flow.returnTo} state={flow.returnState}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}

/** A theme of categories to tick, shown for the choice made and completed with the name asked for. */
function CategoriesTheme({ administrationId, template, flow }: ThemeProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const apply = useApplyTemplate(administrationId)
  const [choice, setChoice] = useState('')
  const [name, setName] = useState('')
  const [ticked, setTicked] = useState(() => new Set(template.ledger_accounts.filter((a) => a.selected && !a.exists).map((a) => a.key)))
  const errors = formErrors(apply.error, { name: template.name_label, ledger_accounts: t('bookkeeping.parts.categories') })

  const shown = template.ledger_accounts.filter((a) => !template.choice || (choice !== '' && (a.choices.length === 0 || a.choices.includes(choice))))
  const children = (key: string) => shown.filter((a) => a.parent === key)
  // A parent the theme adds goes along with its sub-categories, so it is not ticked by hand.
  const isParent = (a: TemplateAccount) => !a.exists && children(a.key).length > 0
  const isTicked = (a: TemplateAccount): boolean => a.exists || ticked.has(a.key) || (isParent(a) && children(a.key).some(isTicked))
  const toggle = (key: string, on: boolean) => setTicked((s) => {
    const next = new Set(s)
    if (on) {
      next.add(key)
    } else {
      next.delete(key)
    }
    return next
  })
  const label = (a: TemplateAccount) => (template.name_label ? a.name.replaceAll(placeholder, name.trim() || placeholder) : a.name)

  const submit = () => {
    const keys = shown.filter((a) => !a.exists && isTicked(a)).map((a) => a.key)
    if (keys.length === 0) {
      navigate(flow.returnTo, { state: flow.returnState })
      return
    }
    apply.mutate(
      { key: template.key, body: { ledger_accounts: keys, ...(template.name_label ? { name: name.trim() } : {}) } },
      { onSuccess: () => navigate(flow.returnTo, { state: flow.returnState }) },
    )
  }
  const sides = [
    { title: t('bookkeeping.theme.balanceSheet'), types: balanceSheetTypes },
    { title: t('bookkeeping.theme.profitAndLoss'), types: profitAndLossTypes },
  ]
  const asked = template.choice || template.name_label
  return (
    <ThemeForm template={template} flow={flow} pending={apply.isPending} onSubmit={submit}>
      <Problems messages={[...errors.general, ...(errors.field('ledger_accounts') ?? [])]} />
      {asked && (
        <div className="category-theme__asked">
          {template.choice && (
            <SelectInput label={template.choice.label} width="medium" value={choice} onChange={(e) => setChoice(e.target.value)}>
              <option value="" disabled>{t('bookkeeping.theme.choose')}</option>
              {template.choice.options.map((o) => <option key={o.key} value={o.key}>{o.name}</option>)}
            </SelectInput>
          )}
          {template.name_label && (
            <TextInput label={template.name_label} width="medium" maxLength={200} autoComplete="off" value={name} errors={errors.field('name')} onChange={(e) => setName(e.target.value)} />
          )}
        </div>
      )}
      {template.choice && choice === '' ? (
        <p>{template.choice.prompt}</p>
      ) : (
        <div className="category-theme__sides">
          {sides.map((side) => (
            <Side key={side.title} title={side.title} types={side.types} accounts={shown}>
              {(a) => <Category account={a} label={label} below={children} ticked={isTicked} locked={isParent} onToggle={toggle} />}
            </Side>
          ))}
        </div>
      )}
    </ThemeForm>
  )
}

type SideProps = { title: string; types: AccountType[]; accounts: TemplateAccount[]; children: (a: TemplateAccount) => React.ReactNode }

/** One part of the books, with a row per type: its name on the left and its categories, as a tree, on the right. */
function Side({ title, types, accounts, children }: SideProps) {
  const { t } = useTranslation()
  const heading = useId()
  const keys = new Set(accounts.map((a) => a.key))
  const shown = types.filter((type) => accounts.some((a) => a.account_type === type))
  if (shown.length === 0) {
    return null
  }
  return (
    <fieldset className="category-theme__side" aria-labelledby={heading}>
      <h3 id={heading} className="category-theme__side-title">{title}</h3>
      {shown.map((type) => (
        <div key={type} className="category-theme__type">
          <h4 className="category-theme__type-title">{t(`bookkeeping.accountTypes.${type}`)}</h4>
          <ul className="category-theme__list">
            {accounts.filter((a) => a.account_type === type && (!a.parent || !keys.has(a.parent))).map((a) => children(a))}
          </ul>
        </div>
      ))}
    </fieldset>
  )
}

type CategoryProps = {
  account: TemplateAccount
  label: (a: TemplateAccount) => string
  below: (key: string) => TemplateAccount[]
  ticked: (a: TemplateAccount) => boolean
  locked: (a: TemplateAccount) => boolean
  onToggle: (key: string, on: boolean) => void
}

/** A category to tick, with its explanation behind an information mark and its sub-categories indented under it. */
function Category(props: CategoryProps) {
  const { account: a, label, below, ticked, locked, onToggle } = props
  const { t } = useTranslation()
  const existsId = useId()
  const subs = below(a.key)
  return (
    <li>
      <div className={`category-theme__account${a.exists ? ' is-there' : ''}`} title={a.exists ? t('bookkeeping.theme.exists') : undefined}>
        <label className="category-theme__check">
          <input
            type="checkbox"
            checked={ticked(a)}
            disabled={a.exists || locked(a)}
            aria-describedby={a.exists ? existsId : undefined}
            onChange={(e) => onToggle(a.key, e.target.checked)}
          />
          <span>{label(a)}</span>
        </label>
        {a.exists && <span id={existsId} hidden>{t('bookkeeping.theme.exists')}</span>}
        {a.description && <InfoTip text={a.description} />}
      </div>
      {subs.length > 0 && (
        <ul className="category-theme__list category-theme__list--sub">
          {subs.map((s) => <Category key={s.key} {...props} account={s} />)}
        </ul>
      )}
    </li>
  )
}

/** A short explanation behind an information mark, shown on hover and focus, as Moneybird's tooltips. */
function InfoTip({ text }: { text: string }) {
  const { t } = useTranslation()
  const id = useId()
  return (
    <span className="category-tip">
      <button type="button" className="category-tip__button" aria-label={t('list.moreInfo')} aria-describedby={id}>
        <InfoIcon />
      </button>
      <span id={id} role="tooltip" className="category-tip__text">{text}</span>
    </span>
  )
}

type SingleProps = ThemeProps & { single: NonNullable<Template['single']>; accounts: LedgerAccount[] }

/** A theme that adds one category: its name, its parent and its kind, such as revenue from services. */
function SingleTheme({ administrationId, template, single, accounts, flow }: SingleProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const apply = useApplyTemplate(administrationId)
  const kindLabel = useId()
  const [form, setForm] = useState({ name: '', parent_id: '', option: single.options[0]?.key ?? '' })
  const errors = formErrors(apply.error, { name: single.name_label, parent_id: t('bookkeeping.parent'), option: single.options_label || t('bookkeeping.theme.kind') })
  const parents = tree(accounts.filter((a) => a.account_type === single.account_type && a.active))
  const submit = () =>
    apply.mutate(
      { key: template.key, body: { name: form.name.trim(), option: form.option, ...(single.parent ? { parent_id: form.parent_id } : {}) } },
      { onSuccess: () => navigate(flow.returnTo, { state: flow.returnState }) },
    )
  return (
    <ThemeForm template={template} flow={flow} pending={apply.isPending} onSubmit={submit}>
      <Problems messages={errors.general} />
      <div className={`category-theme__asked${single.parent ? ' category-theme__asked--stacked' : ''}${single.options_label ? ' category-theme__asked--last' : ''}`}>
        <TextInput label={single.name_label} width="medium" required maxLength={255} autoComplete="off" value={form.name} errors={errors.field('name')} onChange={(e) => setForm({ ...form, name: e.target.value })} />
        {single.parent && (
          <SelectInput label={t('bookkeeping.parent')} width="medium" value={form.parent_id} errors={errors.field('parent_id')} onChange={(e) => setForm({ ...form, parent_id: e.target.value })}>
            <option value="">{t('bookkeeping.noParent')}</option>
            {parents.map(({ account: a, depth }) => <option key={a.id} value={a.id}>{`${' '.repeat(depth)}${a.account_id ? `${a.account_id} ` : ''}${a.name}`}</option>)}
          </SelectInput>
        )}
        {single.options_label && (
          <FormRow label={single.options_label} width="medium" errors={errors.field('option')} labelExtra={single.options_help ? <InfoTip text={single.options_help} /> : undefined}>
            {({ labelId: _, ...control }) => (
              <select {...control} value={form.option} onChange={(e) => setForm({ ...form, option: e.target.value })}>
                {single.options.map((o) => <option key={o.key} value={o.key}>{o.name}</option>)}
              </select>
            )}
          </FormRow>
        )}
      </div>
      {!single.options_label && (
        <div className="category-theme__kinds" role="radiogroup" aria-labelledby={kindLabel}>
          <span id={kindLabel} className="sr-only">{t('bookkeeping.theme.kind')}</span>
          {single.options.map((o) => (
            <label key={o.key} className="category-theme__kind">
              <input type="radio" name={kindLabel} checked={form.option === o.key} onChange={() => setForm({ ...form, option: o.key })} />
              <span>{o.name}</span>
            </label>
          ))}
          {errors.field('option') && <p className="form-input__message form-input__message--error">{errors.field('option')!.join(' ')}</p>}
        </div>
      )}
    </ThemeForm>
  )
}
