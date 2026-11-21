import type { TFunction } from 'i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { type FormEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams, useSearchParams } from 'react-router-dom'
import { api, ApiError, unwrap } from '../api/client'
import type { components } from '../api/schema'
import { Problems } from '../auth/AuthLayout'
import { formatAmount } from '../banking/queries'
import { formErrors, TextInput } from '../forms/Form'
import { type Signature, SignaturePad } from '../forms/SignaturePad'
import { Loading } from '../layout/Loading'
import './online.css'
import { formatUnitPrice } from '../invoices/totals'

type OnlineEstimate = components['schemas']['OnlineEstimate']

function useOnlineEstimate(id: string, code: string) {
  return useQuery({
    queryKey: ['online-estimate', id, code],
    queryFn: () => unwrap(api.GET('/api/online/estimates/{id}', { params: { path: { id }, query: { code } } })),
    retry: false,
  })
}

/** dd-mm-yyyy, as the quote itself prints its dates. */
function day(d?: string | null) {
  if (!d) {
    return ''
  }
  const [y, m, dd] = d.split('-')
  return `${dd}-${m}-${y}`
}

/**
 * The page the link in a quote email opens, after Moneybird's online
 * quotes: the quote as its document, its PDF, and, while it is open, the
 * form its contact accepts it with by name, email address and signature.
 * It needs no account; the code in the link opens it.
 */
export function OnlineEstimatePage() {
  const { estimateId = '' } = useParams()
  const [params] = useSearchParams()
  const code = params.get('code') ?? ''
  const { t, i18n } = useTranslation()
  const estimate = useOnlineEstimate(estimateId, code)
  if (estimate.error) {
    const gone = estimate.error instanceof ApiError && estimate.error.status === 404
    return (
      <main className="online-page">
        <section className="online-card online-card--message">
          <h1>{gone ? t('online.gone') : t('online.failed')}</h1>
          {gone && <p>{t('online.goneText')}</p>}
        </section>
      </main>
    )
  }
  if (!estimate.data) {
    return <Loading />
  }
  const e = estimate.data
  // The page speaks the quote's language, as its PDF does.
  const lt = i18n.getFixedT(e.language || i18n.language)
  const title = lt('online.title', { number: e.estimate_id ?? '' })
  const money = (v?: string | null) => formatAmount(v, e.currency, e.language || i18n.language)
  const pdf = `/api/online/estimates/${estimateId}/pdf?code=${encodeURIComponent(code)}`
  const s = e.sender
  return (
    <main className="online-page" lang={e.language}>
      <header className="online-page__bar">
        <span className="online-page__company">{s.company_name}</span>
        <a className="btn btn--secondary" href={pdf} download>{lt('online.download')}</a>
      </header>
      <article className="online-card online-document" role="document" aria-label={title}>
        <div className="online-document__head">
          <div className="online-document__recipient">{e.contact_name}</div>
          <address className="online-document__sender">
            <strong>{s.company_name}</strong>
            {[s.address1, s.address2, [s.zipcode, s.city].filter(Boolean).join(' ')].filter(Boolean).map((line) => <span key={line}>{line}</span>)}
            {s.email && <span>{s.email}</span>}
            {s.phone && <span>{s.phone}</span>}
            {s.chamber_of_commerce && <span>{lt('online.coc')}: {s.chamber_of_commerce}</span>}
            {s.tax_number && <span>{lt('online.vat')}: {s.tax_number}</span>}
          </address>
        </div>
        <h1 className="online-document__title">{title}</h1>
        <dl className="online-document__facts">
          {e.reference && <><dt>{lt('online.reference')}</dt><dd>{e.reference}</dd></>}
          <dt>{lt('online.date')}</dt><dd>{day(e.estimate_date)}</dd>
          <dt>{lt('online.due')}</dt><dd>{day(e.due_date)}</dd>
        </dl>
        {e.pre_text && <p className="online-document__text">{e.pre_text}</p>}
        <table className="online-document__lines" aria-label={lt('online.lines')}>
          <thead>
            <tr>
              <th className="num">{lt('online.quantity')}</th>
              <th>{lt('online.description')}</th>
              <th className="num">{lt('online.price')}</th>
              <th className="num">{lt('online.total')}</th>
              <th className="num">{lt('online.vatRate')}</th>
            </tr>
          </thead>
          <tbody>
            {e.details.map((d, i) => (
              <tr key={i}>
                <td className="num">{d.amount}</td>
                <td className="online-document__description">{d.description}</td>
                <td className="num">{formatUnitPrice(d.price, e.currency, e.language || i18n.language)}</td>
                <td className="num">{money(d.total_price_excl_tax_with_discount)}</td>
                <td className="num">{d.tax_percentage}%</td>
              </tr>
            ))}
          </tbody>
        </table>
        <dl className="online-document__totals">
          <dt>{lt('online.subtotal')}</dt><dd>{money(e.total_price_excl_tax)}</dd>
          {e.tax_totals.map((r) => (
            <div key={r.tax_percentage} className="online-document__total-row"><dt>{lt('online.vatOver', { rate: r.tax_percentage })}</dt><dd>{money(r.tax_amount)}</dd></div>
          ))}
          <dt className="online-document__grand">{lt('online.grandTotal')}</dt><dd className="online-document__grand">{money(e.total_price_incl_tax)}</dd>
        </dl>
        {e.post_text && <p className="online-document__text">{e.post_text}</p>}
      </article>
      <Acceptance estimate={e} id={estimateId} code={code} t={lt} />
    </main>
  )
}

type AcceptanceProps = { estimate: OnlineEstimate; id: string; code: string; t: TFunction }

/** Under the quote: the form to accept it, or what became of it. */
function Acceptance({ estimate: e, id, code, t }: AcceptanceProps) {
  const heading = useId()
  const client = useQueryClient()
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [signature, setSignature] = useState<Signature | null>(null)
  const [unsigned, setUnsigned] = useState(false)
  const sign = useMutation({
    mutationFn: () => unwrap(api.POST('/api/online/estimates/{id}/sign', {
      params: { path: { id }, query: { code } },
      body: { name: name.trim(), email: email.trim(), signature_output: signature?.image ?? '', audit_trail: JSON.stringify(signature?.strokes ?? []) },
    })),
    onSuccess: (signed) => client.setQueryData(['online-estimate', id, code], signed),
  })
  if (e.signed) {
    const at = e.signed_at ? day(e.signed_at.slice(0, 10)) : ''
    return (
      <section className="online-card online-acceptance online-acceptance--done" role="status">
        <h2>{t('online.accepted')}</h2>
        <p>{t('online.acceptedBy', { name: e.signer_name ?? '', date: at })}</p>
      </section>
    )
  }
  if (!e.signable) {
    return <p className="online-card online-acceptance notice">{t('online.closed')}</p>
  }
  const labels = { name: t('online.name'), email: t('online.email'), signature_output: t('signature.label') }
  const errors = formErrors(sign.error, labels)
  const submit = (ev: FormEvent) => {
    ev.preventDefault()
    if (!signature) {
      setUnsigned(true)
      return
    }
    sign.mutate()
  }
  const signatureErrors = unsigned && !signature ? [t('signature.required')] : errors.field('signature_output')
  return (
    <section className="online-card online-acceptance" aria-labelledby={heading}>
      <form onSubmit={submit} noValidate aria-labelledby={heading}>
        <h2 id={heading}>{t('online.accept')}</h2>
        <p>{t('online.acceptText')}</p>
        <Problems messages={errors.general} />
        <TextInput label={labels.name} value={name} autoComplete="name" errors={errors.field('name')} onChange={(ev) => setName(ev.target.value)} />
        <TextInput label={labels.email} type="email" value={email} autoComplete="email" errors={errors.field('email')} onChange={(ev) => setEmail(ev.target.value)} />
        <SignaturePad
          label={labels.signature_output}
          invalid={!!signatureErrors?.length}
          onChange={(s) => {
            setSignature(s)
            setUnsigned(false)
          }}
        />
        {signatureErrors && signatureErrors.length > 0 && <p className="form-input__message form-input__message--error">{signatureErrors.join(' ')}</p>}
        <p className="online-acceptance__note">{t('online.signatureNote')}</p>
        <div>
          <button className="btn btn--primary" disabled={sign.isPending}>{t('online.submit')}</button>
        </div>
      </form>
    </section>
  )
}
