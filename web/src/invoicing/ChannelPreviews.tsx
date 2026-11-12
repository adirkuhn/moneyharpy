import type { TFunction } from 'i18next'
import type { CSSProperties, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { formatAmount } from '../banking/queries'
import { addressLines, documentDate } from '../invoices/paper'
import type { Identity } from '../settings/queries'
import { type PreviewData, previewTagValues } from './previewData'
import { type DocumentStyle, type FileKind, styleFileURL } from './queries'
import { fontStacks } from './StylePreview'
import { TextileText } from '../forms/TextileEditor'
import { renderTags, type TagValues } from '../forms/textile'
import { shortPaymentReference } from '../invoices/paymentReference'

/** The settings of the Email, Public views and Online sales tabs the previews follow before they are saved. */
export type ChannelForm = Pick<DocumentStyle, 'email_header_type' | 'email_header_content_position' | 'email_header_background_color' | 'email_header_text'
  | 'email_header_text_color' | 'email_logo_display_width' | 'email_footer_type' | 'email_footer_text_color' | 'email_footer_background_color'
  | 'email_footer_profile_title' | 'email_footer_profile_subtitle' | 'body_background_color' | 'email_font_family' | 'email_moneybird_label_enabled'
  | 'public_view_logo_display_size' | 'public_view_background_color' | 'public_view_moneybird_label_enabled' | 'checkout_show_address' | 'checkout_show_phone_field'
  | 'checkout_for_business' | 'checkout_show_identity' | 'checkout_show_withdrawal_button' | 'checkout_favorite_label'>

type Props = {
  administrationId: string
  style: DocumentStyle
  form: ChannelForm
  sender: Identity | undefined
  language: string
  data: PreviewData
}

const hashOf = (style: DocumentStyle, kind: FileKind) => (style as Record<string, unknown>)[`${kind}_hash`] as string | null | undefined

/** The URL of a style's image of a kind, falling back to another kind such as the document logo. */
function imageURL(administrationId: string, style: DocumentStyle, ...kinds: FileKind[]) {
  for (const kind of kinds) {
    const hash = hashOf(style, kind)
    if (hash) {
      return styleFileURL(administrationId, style.id ?? '', kind, hash)
    }
  }
  return undefined
}

/** Fills in an email text's placeholders, as the server does when it sends. */
export function expandEmail(template: string, values: Record<string, string>, tags: TagValues = {}) {
  return renderTags(template, tags).replace(/\{(\w+)\}/g, (all, key: string) => values[key] ?? all)
}

function useDocumentT(language: string): TFunction {
  const { i18n } = useTranslation()
  return i18n.getFixedT(language)
}

/** The invoice email as the style dresses it, with the workflow's text and the latest invoice's details. */
export function EmailPreview({ administrationId, style, form, sender, language, data }: Props) {
  const { t } = useTranslation()
  const dt = useDocumentT(language)
  const money = (v: number) => formatAmount(v, data.currency, language)
  const values = {
    invoice_id: data.invoiceNumber, invoice_date: documentDate(data.invoiceDate, language), due_date: documentDate(data.dueDate, language),
    total: money(data.totals.total), contact_name: data.customer.contactName, company_name: sender?.company_name ?? '',
    payment_reference: shortPaymentReference(data.paymentReference), reference: data.reference,
  }
  const tags = previewTagValues(data, language, sender)
  const subject = expandEmail(data.emailSubject || dt('documentStyle.emailSubject'), values, tags)
  const body = expandEmail(data.emailBody || dt('invoices.sendForm.defaultMessage'), values, tags)
  const details: Array<[string, string]> = (
    [
      [dt('documentStyle.emailDetails.invoice'), data.invoiceNumber],
      [dt('documentStyle.emailDetails.sender'), sender?.email ? `${sender.company_name} (${sender.email})` : sender?.company_name ?? ''],
      [dt('documentStyle.emailDetails.reference'), data.reference],
      [dt('documentStyle.emailDetails.invoiceDate'), values.invoice_date],
      [dt('documentStyle.emailDetails.dueDate'), values.due_date],
      [dt('documentStyle.emailDetails.amountDue'), values.total],
    ] as Array<[string, string]>
  ).filter(([, v]) => v)
  const logo = imageURL(administrationId, style, 'email_logo', 'logo')
  const headerImage = imageURL(administrationId, style, 'email_header_background')
  const type = form.email_header_type
  const withLogo = type.endsWith('_with_logo')
  const withText = type.endsWith('_with_text')
  const headerStyle: CSSProperties = type.startsWith('background_image')
    ? { backgroundColor: form.email_header_background_color, backgroundImage: headerImage ? `url(${headerImage})` : undefined }
    : { backgroundColor: form.email_header_background_color }

  return (
    <div className="channel-preview" role="region" aria-label={t('documentStyle.emailPreview')}>
      <p className="channel-preview__subject"><span>{t('documentStyle.subject')}:</span> {subject}</p>
      <div className="email-preview" style={{ backgroundColor: form.body_background_color, fontFamily: fontStacks[form.email_font_family] }}>
        <article className="email-preview__card" aria-label={t('documentStyle.emailPreview')}>
          {type === 'background_image' && headerImage && <img className="email-preview__image" src={headerImage} alt="" />}
          {type !== '' && type !== 'background_image' && (
            <header
              className={`email-preview__header email-preview__header--${form.email_header_content_position}${type.startsWith('background_image') ? ' has-image' : ''}`}
              style={headerStyle}
            >
              {withLogo && logo && <img src={logo} alt="" style={{ width: `${form.email_logo_display_width}%` }} />}
              {withText && <p style={{ color: form.email_header_text_color }}>{form.email_header_text}</p>}
            </header>
          )}
          <div className="email-preview__body">
            {body.split(/\n{2,}/).map((p, i) => (
              <p key={i}>{p.split('\n').map((l, j) => <span key={j} className="invoice-paper__line">{l}</span>)}</p>
            ))}
            <table className="email-preview__details">
              <tbody>
                {details.map(([k, v]) => <tr key={k}><th scope="row">{k}</th><td>{v}</td></tr>)}
              </tbody>
            </table>
          </div>
          <EmailFooter administrationId={administrationId} style={style} form={form} sender={sender} />
        </article>
        {form.email_moneybird_label_enabled && <p className="email-preview__label">{dt('documentStyle.sentWith')}</p>}
      </div>
    </div>
  )
}

function EmailFooter({ administrationId, style, form, sender }: Pick<Props, 'administrationId' | 'style' | 'form' | 'sender'>) {
  const colours = { color: form.email_footer_text_color, backgroundColor: form.email_footer_background_color }
  switch (form.email_footer_type) {
    case 'contact_information':
      return (
        <footer className="email-preview__footer" style={colours}>
          {[sender?.company_name, ...(sender ? addressLines(sender) : []), sender?.email, sender?.phone].filter(Boolean).map((l, i) => <p key={i}>{l}</p>)}
        </footer>
      )
    case 'signature': {
      const picture = imageURL(administrationId, style, 'email_footer_profile_picture')
      return (
        <footer className="email-preview__footer" style={colours}>
          {picture && <img className="email-preview__avatar" src={picture} alt="" />}
          {form.email_footer_profile_title && <p className="invoice-paper__bold">{form.email_footer_profile_title}</p>}
          {form.email_footer_profile_subtitle && <p>{form.email_footer_profile_subtitle}</p>}
        </footer>
      )
    }
    case 'background_image': {
      const image = imageURL(administrationId, style, 'email_footer_background')
      return image ? <img className="email-preview__image" src={image} alt="" /> : null
    }
  }
  return null
}

/** The online view of an invoice, as customers would open it from the email. */
export function PublicViewPreview({ administrationId, style, form, sender, language, data }: Props) {
  const { t } = useTranslation()
  const dt = useDocumentT(language)
  const money = (v: number) => formatAmount(v, data.currency, language)
  const logo = imageURL(administrationId, style, 'public_view_logo', 'logo')
  const iban = sender?.bank_account_number
  return (
    <div className="channel-preview" role="region" aria-label={t('documentStyle.publicViewPreview')}>
      <div className="public-preview" style={{ backgroundColor: form.public_view_background_color }}>
        <article className="public-preview__card" aria-label={t('documentStyle.publicViewPreview')}>
          <header className="public-preview__header">
            <div className="public-preview__logo">{logo && <img src={logo} alt="" style={{ width: `${form.public_view_logo_display_size}%` }} />}</div>
            <h2>{dt('invoices.document.invoice')} {data.invoiceNumber}</h2>
          </header>
          <div className="public-preview__parties">
            <PartyBlock name={sender?.company_name ?? ''} lines={[...(sender ? addressLines(sender) : []), sender?.email ?? ''].filter(Boolean)} />
            <PartyBlock name={data.customer.name} lines={data.customer.address} />
          </div>
          <table className="public-preview__lines" aria-label={dt('invoices.document.lines')}>
            <tbody>
              {data.lines.map((l, i) => (
                <tr key={i}>
                  <td className="public-preview__quantity">{l.quantity}</td>
                  <td>
                    {l.description}
                    {l.quantity !== 1 && <span className="public-preview__unit">{dt('documentStyle.perUnit', { price: money(l.price) })}</span>}
                  </td>
                  <td className="invoice-paper__money">{money(l.quantity * l.price)}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <table className="public-preview__totals">
            <tbody>
              <tr><th scope="row">{dt('invoices.document.subtotal')}</th><td>{money(data.totals.subtotal)}</td></tr>
              {data.totals.vat.map((v) => <tr key={v.percentage}><th scope="row">{v.percentage}% {dt('invoices.document.vat')}</th><td>{money(v.amount)}</td></tr>)}
              <tr className="is-total"><th scope="row">{dt('invoices.document.total')}</th><td>{money(data.totals.total)}</td></tr>
            </tbody>
          </table>
          {(data.paymentConditions || iban) && (
            <section className="public-preview__conditions">
              <h3>{dt('documentStyle.paymentConditions')}</h3>
              {data.paymentConditions && <p className="invoice-paper__conditions"><TextileText text={data.paymentConditions} values={previewTagValues(data, language, sender)} /></p>}
            </section>
          )}
        </article>
        <aside className="public-preview__pay">{dt('documentStyle.payPanel')}</aside>
        {form.public_view_moneybird_label_enabled && <p className="email-preview__label">{dt('documentStyle.madeWith')}</p>}
      </div>
    </div>
  )
}

function PartyBlock({ name, lines }: { name: string; lines: string[] }) {
  return (
    <div>
      <p className="invoice-paper__bold">{name}</p>
      {lines.map((l, i) => <p key={i}>{l}</p>)}
    </div>
  )
}

/** The online sales checkout: the products on offer, and the customer's details it asks for. */
export function CheckoutPreview({ administrationId, style, form, sender, language, data }: Props) {
  const { t } = useTranslation()
  const dt = useDocumentT(language)
  const logo = imageURL(administrationId, style, 'public_view_logo', 'logo')
  const vat = (data.totals.vat[0]?.percentage ?? 21) / 100
  const products = data.products.length
    ? data.products
    : [1, 2, 3].map((n) => ({ title: dt('documentStyle.sample.subscription', { n }), description: dt('documentStyle.sample.subscriptionText'), price: 10 * n, frequency: 'month' }))
  const price = (p: number) => formatAmount(form.checkout_for_business ? p : p * (1 + vat), data.currency, language)
  const field = (label: string, wide = false) => <span className={`checkout-preview__field${wide ? ' is-wide' : ''}`}>{label}</span>
  return (
    <div className="channel-preview" role="region" aria-label={t('documentStyle.checkoutPreview')}>
      <div className="checkout-preview">
        <div className="checkout-preview__products">
          <div className="checkout-preview__logo">{logo && <img src={logo} alt="" style={{ width: `${form.public_view_logo_display_size}%` }} />}</div>
          <ul aria-label={dt('documentStyle.products')}>
            {products.map((p, i) => (
              <li key={i} className={`checkout-preview__product${i === 1 ? ' is-favorite' : ''}`}>
                <p className="checkout-preview__title">
                  <span>{p.title}</span>
                  {i === 1 && form.checkout_favorite_label && <span className="checkout-preview__badge">{form.checkout_favorite_label}</span>}
                </p>
                <p>{p.description}</p>
                <p className="checkout-preview__price">
                  {price(p.price)}{p.frequency && <> {dt(`documentStyle.per.${p.frequency}`, { defaultValue: '' })}</>}
                  {form.checkout_for_business && <span className="hint"> {dt('documentStyle.exclVat')}</span>}
                </p>
              </li>
            ))}
          </ul>
        </div>
        <div className="checkout-preview__form">
          <Section title={dt('documentStyle.checkout.contact')}>
            {field(dt('documentStyle.checkout.email'), true)}
            {form.checkout_for_business && field(dt('documentStyle.checkout.company'), true)}
            {form.checkout_show_address && (
              <>
                <p className="checkout-preview__caption">{dt('documentStyle.checkout.address')}</p>
                {field(dt('documentStyle.checkout.firstName'))}{field(dt('documentStyle.checkout.lastName'))}
                {field(dt('documentStyle.checkout.country'), true)}
                {field(dt('documentStyle.checkout.zipcode'))}{field(dt('documentStyle.checkout.number'))}
                {field(dt('documentStyle.checkout.street'), true)}{field(dt('documentStyle.checkout.city'), true)}
              </>
            )}
            {form.checkout_show_phone_field && field(dt('documentStyle.checkout.phone'), true)}
            <p className="checkout-preview__check"><span aria-hidden="true" className={form.checkout_for_business ? 'is-checked' : ''} /> {dt('documentStyle.checkout.business')}</p>
          </Section>
          <Section title={dt('documentStyle.checkout.payment')}>{field(dt('documentStyle.checkout.method'), true)}</Section>
          {form.checkout_show_identity && sender && (
            <Section title={dt('documentStyle.checkout.seller')}>
              <p className="invoice-paper__bold">{sender.company_name}</p>
              {addressLines(sender).map((l) => <p key={l}>{l}</p>)}
              {sender.chamber_of_commerce && <p>{dt('documentStyle.fields.chamber_of_commerce')}: {sender.chamber_of_commerce}</p>}
              {sender.tax_number && <p>{dt('documentStyle.fields.tax_number')}: {sender.tax_number}</p>}
            </Section>
          )}
          {form.checkout_show_withdrawal_button && <span className="checkout-preview__button is-secondary">{dt('documentStyle.checkout.withdraw')}</span>}
          <span className="checkout-preview__button">{dt('documentStyle.checkout.pay')}</span>
        </div>
      </div>
    </div>
  )
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="checkout-preview__section">
      <h3>{title}</h3>
      <div className="checkout-preview__fields">{children}</div>
    </section>
  )
}
