import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formErrors } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { BoxTable, CopyTable, useEuros } from './BoxTable'
import { IcpTable } from './IcpTable'
import { periodName } from './period'
import { type IcpKind, periodParam, useSubmitVatReturn, useVatPeriods, useVatPreview, type VatKind, vatKinds, type VatPreview } from './queries'
import './vat.css'

const loginURL = 'https://www.belastingdienst.nl/wps/wcm/connect/nl/ondernemers/content/inloggen-voor-ondernemers'
const helpURL = 'https://www.belastingdienst.nl/wps/wcm/connect/nl/btw/content/ik-moet-btw-aangifte-doen-hoe-vul-ik-die-in'

type Step = 'type' | 'period' | 'check' | 'submit'

/**
 * Moneybird's "Declare new VAT Return" (/declarations/new), filed by hand:
 * the kind, the period, the checks with the amounts, and the amounts to
 * type over at the tax authority before marking it filed.
 */
export function NewVatReturnPage() {
  const { t, i18n } = useTranslation()
  const { administrationId = '' } = useParams()
  const administration = useAdministration(administrationId)
  const [step, setStep] = useState<Step>('type')
  const [kind, setKind] = useState<VatKind>('vat_return')
  const [period, setPeriod] = useState('')
  const periods = useVatPeriods(administrationId, step === 'type' ? undefined : kind)
  const preview = useVatPreview(administrationId, kind, step === 'check' || step === 'submit' ? period : undefined)

  if (administration.error instanceof ApiError && administration.error.status < 500) {
    return <NotFoundPage />
  }
  if (!administration.data) {
    return <Loading />
  }
  const list = `/${administrationId}/declarations`
  const options = periods.data ?? []
  const chosen = options.find((p) => periodParam(p) === period) ?? options[0]
  const name = chosen ? periodName(chosen.period_from, chosen.period_to, t, i18n.language) : ''
  const at = (s: Step) => ['type', 'period', 'check', 'submit'].indexOf(s) <= ['type', 'period', 'check', 'submit'].indexOf(step)
  const edit = (s: Step, label: string) => <button type="button" className="btn btn--link vat-step__value" onClick={() => setStep(s)}>{label}</button>

  return (
    <Page title={t('vat.new.title')} back={{ to: list, label: t('vat.backToList') }}>
      <section className="vat-step" aria-labelledby="vat-step-type">
        <h2 id="vat-step-type" className="vat-step__title">{t('vat.new.type')}</h2>
        {step === 'type' ? (
          <form onSubmit={(e) => { e.preventDefault(); setPeriod(''); setStep('period') }}>
            <fieldset className="vat-kinds">
              <legend className="sr-only">{t('vat.new.type')}</legend>
              {vatKinds.map((k) => (
                <label key={k} className={`vat-kind${k === kind ? ' is-checked' : ''}`}>
                  <input type="radio" name="kind" value={k} checked={k === kind} onChange={() => setKind(k)} />
                  <span className="vat-kind__title">{t(`vat.kinds.${k}`)}</span>
                  <span className="vat-kind__text">{t(`vat.new.kindText.${k}`)}</span>
                </label>
              ))}
            </fieldset>
            <button className="btn btn--primary">{t('vat.new.next')}</button>
          </form>
        ) : edit('type', t(`vat.kinds.${kind}`))}
      </section>

      {at('period') && (
        <section className="vat-step" aria-labelledby="vat-step-period">
          <h2 id="vat-step-period" className="vat-step__title">{t('vat.new.period')}</h2>
          {step === 'period' ? (
            !periods.data ? <Loading /> : (
              <form onSubmit={(e) => { e.preventDefault(); if (chosen) { setPeriod(periodParam(chosen)); setStep('check') } }}>
                {kind !== 'supplement' && chosen && (
                  <p className="vat-step__text">
                    {t('vat.new.periodSetting', { period: t(`vat.granularity.${chosen.granularity}`) })}{' '}
                    <Link to={`/${administrationId}/settings/bookkeeping`}>{t('vat.new.periodSettingLink')}</Link>
                  </p>
                )}
                {options.length === 0 ? (
                  <p className="empty">{t(`vat.new.noPeriods.${kind}`)}</p>
                ) : (
                  <label className="vat-step__field">
                    <span>{t('vat.new.period')}</span>
                    <select value={chosen ? periodParam(chosen) : ''} onChange={(e) => setPeriod(e.target.value)}>
                      {options.map((p) => <option key={periodParam(p)} value={periodParam(p)}>{periodName(p.period_from, p.period_to, t, i18n.language)}</option>)}
                    </select>
                  </label>
                )}
                <button className="btn btn--primary" disabled={!chosen}>{t('vat.new.next')}</button>
              </form>
            )
          ) : edit('period', name)}
        </section>
      )}

      {at('check') && (
        <section className="vat-step" aria-labelledby="vat-step-check">
          <h2 id="vat-step-check" className="vat-step__title">{t('vat.new.check')}</h2>
          {step === 'check' ? (
            !preview.data ? (preview.error ? <Problems messages={formErrors(preview.error, {}).general} /> : <Loading />) : (
              <>
                <button type="button" className="btn btn--link" onClick={() => void preview.refetch()}>{t('vat.new.reload')}</button>
                <Checks administrationId={administrationId} preview={preview.data} />
                {kind === 'icp_declaration'
                  ? <IcpTable lines={preview.data.icp_lines} label={t('vat.icp.title')} />
                  : <BoxTable figures={preview.data} periodName={name} label={t('vat.boxesOf', { period: name })} />}
                <button type="button" className="btn btn--primary" onClick={() => setStep('submit')}>{t('vat.new.next')}</button>
              </>
            )
          ) : edit('check', t('vat.new.checkAgain'))}
        </section>
      )}

      {step === 'submit' && preview.data && (
        <Submit administrationId={administrationId} companyName={administration.data.name ?? ''} kind={kind} period={period} name={name} preview={preview.data} />
      )}
    </Page>
  )
}

/** What may be missing from the books, as warnings, and the checks that passed. */
function Checks({ administrationId, preview }: { administrationId: string; preview: VatPreview }) {
  const { t } = useTranslation()
  const { i18n } = useTranslation()
  const c = preview.checks
  const money = (v: string) => new Intl.NumberFormat(i18n.language, { style: 'currency', currency: 'EUR' }).format(Number(v))
  const base = `/${administrationId}`
  const all = [
    { failed: c.open_transactions > 0, title: t('vat.checks.open.title'), text: <Link to={`${base}/financial_mutations?state=unprocessed`}>{t('vat.checks.open.text', { count: c.open_transactions })}</Link>, passed: t('vat.checks.open.passed') },
    { failed: Number(c.untaxed_purchases) !== 0, title: t('vat.checks.untaxed.title'), text: t('vat.checks.untaxed.text', { amount: money(c.untaxed_purchases) }), passed: t('vat.checks.untaxed.passed') },
    { failed: c.bookings_without_vat > 0, title: t('vat.checks.withoutVat.title'), text: t('vat.checks.withoutVat.text', { count: c.bookings_without_vat }), passed: t('vat.checks.withoutVat.passed') },
    { failed: c.unprocessed_documents > 0, title: t('vat.checks.documents.title'), text: <Link to={`${base}/documents`}>{t('vat.checks.documents.text', { count: c.unprocessed_documents })}</Link>, passed: t('vat.checks.documents.passed') },
  ]
  const passed = all.filter((x) => !x.failed)
  return (
    <ul className="vat-checks" aria-label={t('vat.new.check')}>
      {all.filter((x) => x.failed).map((x) => (
        <li key={x.title} className="vat-check is-warning">
          <strong>{x.title}</strong>
          <span>{x.text}</span>
        </li>
      ))}
      {passed.length > 0 && (
        <li className="vat-check is-passed">
          <strong>{t('vat.checks.passed', { count: passed.length })}</strong>
          {passed.map((x) => <span key={x.title}>{x.passed}</span>)}
        </li>
      )}
    </ul>
  )
}

type SubmitProps = { administrationId: string; companyName: string; kind: VatKind; period: string; name: string; preview: VatPreview }

/**
 * Moneybird's "Submit manually": log in at the tax authority, type the
 * amounts over, file once the total matches; then confirm and mark it filed
 * here.
 */
function Submit({ administrationId, companyName, kind, period, name, preview }: SubmitProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const euros = useEuros()
  const submit = useSubmitVatReturn(administrationId)
  const [kinds, setKinds] = useState<Record<string, IcpKind>>({})
  const [confirmed, setConfirmed] = useState(false)
  const late = new Date().toISOString().slice(0, 10) > preview.deadline
  const send = (e: FormEvent) => {
    e.preventDefault()
    submit.mutate(
      { kind, period, ...(kind === 'icp_declaration' ? { icp_lines: preview.icp_lines.map((l) => ({ contact_id: l.contact_id, kind: kinds[l.contact_id] ?? l.kind })) } : {}) },
      { onSuccess: (saved) => navigate(`/${administrationId}/declarations/${saved.id}`) },
    )
  }
  return (
    <section className="vat-step" aria-labelledby="vat-step-submit">
      <h2 id="vat-step-submit" className="vat-step__title">{t('vat.new.submit')}</h2>
      <form onSubmit={send} aria-label={t('vat.new.submit')}>
        {late && <p className="notice vat-late" role="alert">{t('vat.new.late', { date: preview.deadline })}</p>}
        <ol className="vat-manual">
          <li>
            <strong>{t('vat.manual.login')}</strong>
            <span>{t('vat.manual.loginText', { period: name })}</span>
            <a className="btn btn--secondary" href={loginURL} target="_blank" rel="noopener noreferrer">{t('vat.manual.portal')}</a>
          </li>
          <li>
            <strong>{t('vat.manual.fill')}</strong>
            <span>{t(kind === 'icp_declaration' ? 'vat.manual.fillIcp' : 'vat.manual.fillText')}</span>
            {kind === 'icp_declaration'
              ? <IcpTable lines={preview.icp_lines} label={t('vat.manual.amounts')} kinds={kinds} onKind={(id, k) => setKinds({ ...kinds, [id]: k })} />
              : <CopyTable figures={preview} label={t('vat.manual.amounts')} />}
          </li>
          <li>
            <strong>{t('vat.manual.send')}</strong>
            <span>
              {kind === 'icp_declaration' ? t('vat.manual.sendIcp') : t('vat.manual.sendText', { total: euros(preview.total) })}{' '}
              <a href={helpURL} target="_blank" rel="noopener noreferrer">{t('vat.manual.help')}</a>
            </span>
          </li>
        </ol>
        <label className="check vat-confirm">
          <input type="checkbox" checked={confirmed} onChange={(e) => setConfirmed(e.target.checked)} />
          <span>{t('vat.manual.confirm', { kind: t(`vat.kinds.${kind}`), name: companyName, period: name })}</span>
        </label>
        <Problems messages={formErrors(submit.error, {}).general} />
        <button className="btn btn--primary" disabled={!confirmed || submit.isPending}>{t('vat.manual.markFiled')}</button>
      </form>
    </section>
  )
}
