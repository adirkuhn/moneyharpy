import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { CheckboxInput, FormSection, formErrors, SelectInput, TextArea, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { CategorySelect } from './CategorySelect'
import { type BookingRule, type RuleCondition, useBookingRules, useDeleteBookingRule, useFinancialAccounts, useSaveBookingRule } from './queries'

const textOperators: RuleCondition['operator'][] = ['contains', 'equals', 'starts_with', 'ends_with']
const amountOperators: RuleCondition['operator'][] = ['equals', 'greater_than', 'less_than']
const fields: RuleCondition['field'][] = ['message', 'contra_account_name', 'contra_account_number', 'amount']

/** Adds a booking rule (/booking_rules/new) or edits one (/booking_rules/:id). */
export function BookingRuleFormPage() {
  const { administrationId = '', ruleId } = useParams()
  const rules = useBookingRules(administrationId)
  if (!rules.data) {
    return <Loading />
  }
  const rule = ruleId ? rules.data.find((r) => r.id === ruleId) : undefined
  if (ruleId && !rule) {
    return <NotFoundPage />
  }
  return <RuleForm key={rule?.id ?? 'new'} administrationId={administrationId} rule={rule ?? null} />
}

function RuleForm({ administrationId, rule }: { administrationId: string; rule: BookingRule | null }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const accounts = useFinancialAccounts(administrationId)
  const save = useSaveBookingRule(administrationId, rule?.id)
  const remove = useDeleteBookingRule(administrationId)
  const [form, setForm] = useState({
    name: rule?.name ?? '',
    active: rule?.active ?? true,
    match_all: rule?.match_all ?? true,
    direction: rule?.direction ?? 'any',
    financial_account_id: rule?.financial_account_id ?? '',
    conditions: rule?.conditions ?? [{ field: 'contra_account_name', operator: 'contains', value: '' } as RuleCondition],
    ledger_account_id: rule?.ledger_account_id ?? '',
    description: rule?.description ?? '',
  })
  const back = `/${administrationId}/settings/bank#booking-rules`
  const labels = { name: t('bank.ruleName'), direction: t('bank.direction'), financial_account_id: t('bank.ruleAccount'), ledger_account_id: t('bank.category'), description: t('bank.description'), conditions: t('bank.conditions') }
  const errors = formErrors(save.error ?? remove.error, labels)
  const setCondition = (i: number, change: Partial<RuleCondition>) => {
    const conditions = form.conditions.map((c, j) => {
      if (j !== i) {
        return c
      }
      const next = { ...c, ...change }
      const allowed = next.field === 'amount' ? amountOperators : textOperators
      return allowed.includes(next.operator) ? next : { ...next, operator: allowed[0] ?? 'equals' }
    })
    setForm({ ...form, conditions })
  }
  const submit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate(form, { onSuccess: () => navigate(back) })
  }
  const deleteAction = rule && (
    <ConfirmDelete label={t('settings.delete')} confirmLabel={t('settings.confirmDelete')} disabled={remove.isPending} onConfirm={() => remove.mutate(rule.id, { onSuccess: () => navigate(back) })} />
  )
  return (
    <Page title={rule ? t('bank.editRule') : t('bank.newRule')} back={{ to: back, label: t('bank.back') }} actions={deleteAction || undefined}>
      <form className="form-sheet" onSubmit={submit} noValidate>
        <Problems messages={errors.general} />
        <FormSection>
          <TextInput label={labels.name} required maxLength={255} autoComplete="off" value={form.name} errors={errors.field('name')} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <SelectInput label={labels.direction} width="medium" value={form.direction} errors={errors.field('direction')} onChange={(e) => setForm({ ...form, direction: e.target.value as typeof form.direction })}>
            <option value="any">{t('bank.directions.any')}</option>
            <option value="debit">{t('bank.directions.debit')}</option>
            <option value="credit">{t('bank.directions.credit')}</option>
          </SelectInput>
          <SelectInput label={labels.financial_account_id} width="medium" value={form.financial_account_id} errors={errors.field('financial_account_id')} onChange={(e) => setForm({ ...form, financial_account_id: e.target.value })}>
            <option value="">{t('bank.allAccounts')}</option>
            {accounts.data?.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
          </SelectInput>
        </FormSection>
        <FormSection title={labels.conditions}>
          {errors.field('conditions') && <Problems messages={errors.field('conditions') ?? []} />}
          <SelectInput label={t('bank.matching')} width="medium" value={form.match_all ? 'all' : 'any'} onChange={(e) => setForm({ ...form, match_all: e.target.value === 'all' })}>
            <option value="all">{t('bank.matchAll')}</option>
            <option value="any">{t('bank.matchAny')}</option>
          </SelectInput>
          {form.conditions.map((c, i) => (
            <fieldset className="reminder" key={i}>
              <legend className="reminder__title">{t('bank.condition', { n: i + 1 })}</legend>
              <SelectInput label={t('bank.field')} width="medium" value={c.field} onChange={(e) => setCondition(i, { field: e.target.value as RuleCondition['field'] })}>
                {fields.map((f) => <option key={f} value={f}>{t(`bank.fields.${f}`)}</option>)}
              </SelectInput>
              <SelectInput label={t('bank.operator')} width="medium" value={c.operator} onChange={(e) => setCondition(i, { operator: e.target.value as RuleCondition['operator'] })}>
                {(c.field === 'amount' ? amountOperators : textOperators).map((o) => <option key={o} value={o}>{t(`bank.operators.${o}`)}</option>)}
              </SelectInput>
              <TextInput
                label={t('bank.value')}
                width={c.field === 'amount' ? 'small' : 'long'}
                inputMode={c.field === 'amount' ? 'decimal' : undefined}
                maxLength={255}
                value={c.value}
                errors={errors.field(`conditions.${i}`)}
                onChange={(e) => setCondition(i, { value: e.target.value })}
              />
              {form.conditions.length > 1 && (
                <button type="button" className="btn btn--secondary btn--small reminder__remove" onClick={() => setForm({ ...form, conditions: form.conditions.filter((_, j) => j !== i) })}>
                  {t('bank.removeCondition', { n: i + 1 })}
                </button>
              )}
            </fieldset>
          ))}
          {form.conditions.length < 10 && (
            <button type="button" className="icon-link icon-link--button" onClick={() => setForm({ ...form, conditions: [...form.conditions, { field: 'message', operator: 'contains', value: '' }] })}>
              {t('bank.addCondition')}
            </button>
          )}
        </FormSection>
        <FormSection title={t('bank.then')}>
          <CategorySelect administrationId={administrationId} label={labels.ledger_account_id} value={form.ledger_account_id} errors={errors.field('ledger_account_id')} onChange={(ledger_account_id) => setForm({ ...form, ledger_account_id })} />
          <TextArea label={labels.description} maxLength={1000} value={form.description} errors={errors.field('description')} onChange={(e) => setForm({ ...form, description: e.target.value })} />
          <CheckboxInput label={t('bookkeeping.active')} checked={form.active} onChange={(e) => setForm({ ...form, active: e.target.checked })} />
        </FormSection>
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={save.isPending}>{t('settings.save')}</button>
          <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}
