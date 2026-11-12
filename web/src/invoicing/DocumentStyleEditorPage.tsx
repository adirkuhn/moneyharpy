import { type FormEvent, type ReactNode, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { FormNotice, type FormErrors, formErrors, SelectInput, TextArea } from '../forms/Form'
import { addressLines, defaultSenderLines } from '../invoices/paper'
import { Loading } from '../layout/Loading'
import { Menu } from '../layout/Menu'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { type CustomField, type Identity, useCustomFields, useIdentities } from '../settings/queries'
import { AccordionItem, PartIcon } from './Accordion'
import { CheckoutParts, EmailParts, fonts, PublicViewParts } from './ChannelParts'
import { type ChannelForm, CheckoutPreview, EmailPreview, PublicViewPreview } from './ChannelPreviews'
import { FieldColumns } from './FieldColumns'
import { type PreviewData, usePreviewData } from './previewData'
import { type DocumentStyle, type DocumentStyleChanges, useDeleteDocumentStyle, useDocumentStyle, useSaveDocumentStyle } from './queries'
import { AlignIcon, FieldErrors, Range, Segmented, Toggle } from './StyleControls'
import { StyleFileField } from './StyleFileField'
import { StyleImagesField } from './StyleImagesField'
import { customerFields, customFieldId, estimateMetadataFields, invoiceMetadataFields, normalLines, senderFields, withCustomFields } from './styleLayout'
import { type PreviewLayout, StylePreview } from './StylePreview'
import '../contacts/contacts.css'
import './styleEditor.css'

const tabs = ['document', 'email', 'public_view', 'checkout'] as const
type Tab = (typeof tabs)[number]
const previewLanguages = ['en', 'nl'] as const
const fontSizes = [8, 9, 10, 11, 12, 13, 14]

export type StyleForm = PreviewLayout & ChannelForm & Required<Pick<DocumentStyleChanges, 'print_on_stationery' | 'paper_size' | 'custom_css'>>

export function toForm(d: DocumentStyle): StyleForm {
  return {
    logo_container_full_width: d.logo_container_full_width ?? false,
    logo_display_width: d.logo_display_width ?? 50,
    logo_position: d.logo_position ?? 'left',
    print_on_stationery: d.print_on_stationery ?? false,
    paper_size: d.paper_size ?? 'A4',
    address_position: d.address_position ?? 'left',
    font_size: d.font_size ?? 10,
    font_family: d.font_family ?? 'lucida_grande',
    custom_css: d.custom_css ?? '',
    hide_sender_address: d.hide_sender_address,
    sender_address_width: d.sender_address_width,
    show_epc_code: d.show_epc_code,
    invoice_sender_address: normalLines(d.invoice_sender_address, defaultSenderLines),
    invoice_metadata_left: normalLines(d.invoice_metadata_left, [{ field: 'reference' }]),
    invoice_metadata_right: normalLines(d.invoice_metadata_right, [{ field: 'invoice_date' }, { field: 'due_date' }]),
    estimate_sender_address: normalLines(d.estimate_sender_address, defaultSenderLines),
    estimate_metadata_left: normalLines(d.estimate_metadata_left, [{ field: 'reference' }]),
    estimate_metadata_right: normalLines(d.estimate_metadata_right, [{ field: 'estimate_date' }, { field: 'due_date' }]),
    invoice_recipient_address: normalLines(d.invoice_recipient_address, []),
    estimate_recipient_address: normalLines(d.estimate_recipient_address, []),
    email_header_type: d.email_header_type,
    email_header_content_position: d.email_header_content_position,
    email_header_background_color: d.email_header_background_color,
    email_header_text: d.email_header_text,
    email_header_text_color: d.email_header_text_color,
    email_logo_display_width: d.email_logo_display_width,
    email_footer_type: d.email_footer_type,
    email_footer_text_color: d.email_footer_text_color,
    email_footer_background_color: d.email_footer_background_color,
    email_footer_profile_title: d.email_footer_profile_title,
    email_footer_profile_subtitle: d.email_footer_profile_subtitle,
    body_background_color: d.body_background_color,
    email_font_family: d.email_font_family,
    email_label_enabled: d.email_label_enabled,
    public_view_logo_display_size: d.public_view_logo_display_size,
    public_view_background_color: d.public_view_background_color,
    public_view_label_enabled: d.public_view_label_enabled,
    checkout_show_address: d.checkout_show_address,
    checkout_show_phone_field: d.checkout_show_phone_field,
    checkout_for_business: d.checkout_for_business,
    checkout_show_identity: d.checkout_show_identity,
    checkout_show_withdrawal_button: d.checkout_show_withdrawal_button,
    checkout_favorite_label: d.checkout_favorite_label,
  }
}

/** Moneybird's document style editor (/document_styles/:id/edit/:tab/:language): settings in parts beside a preview. */
export function DocumentStyleEditorPage() {
  const { i18n } = useTranslation()
  const { administrationId = '', styleId = '', tab, language } = useParams()
  const style = useDocumentStyle(administrationId, styleId)
  const identities = useIdentities(administrationId)
  const customFields = useCustomFields(administrationId)
  if (!tabs.includes(tab as Tab) || (language !== undefined && !previewLanguages.includes(language as 'en'))) {
    return <NotFoundPage />
  }
  if (style.error instanceof ApiError && style.error.status < 500) {
    return <NotFoundPage />
  }
  if (!style.data || !identities.data) {
    return <Loading />
  }
  const preview = language ?? (i18n.language.startsWith('nl') ? 'nl' : 'en')
  return (
    <Editor
      key={style.data.id}
      administrationId={administrationId}
      style={style.data}
      identities={identities.data}
      customFields={customFields.data ?? []}
      tab={tab as Tab}
      language={preview}
      pinned={language !== undefined}
    />
  )
}

/** Sends /document_styles/:id/edit to its first tab, as Moneybird does. */
export function DocumentStyleEditRedirect() {
  return <Navigate to="document" replace />
}

type EditorProps = {
  administrationId: string
  style: DocumentStyle
  identities: Identity[]
  customFields: CustomField[]
  tab: Tab
  language: string
  /** Whether the address names the preview language, which the tabs then keep. */
  pinned: boolean
}

/** Labels of every field the editor sends, so a refused one shows with its name and opens its part. */
function fieldLabels(t: (key: string) => string): Record<string, string> {
  const email = (k: string) => t(`documentStyle.email.${k}`)
  return {
    logo_position: t('documentStyle.logoPosition'), logo_display_width: t('documentStyle.logoWidth'), font_family: t('documentStyle.fontFamily'), font_size: t('documentStyle.fontSize'),
    address_position: t('documentStyle.addressPosition'), custom_css: t('documentStyle.customCss'), paper_size: t('documentStyle.paperSize'),
    sender_address_width: t('documentStyle.senderWidth'),
    invoice_sender_address: t('documentStyle.sections.invoiceSender'), estimate_sender_address: t('documentStyle.sections.estimateSender'),
    invoice_recipient_address: t('documentStyle.sections.invoiceRecipient'), estimate_recipient_address: t('documentStyle.sections.estimateRecipient'),
    invoice_metadata_left: t('documentStyle.sections.invoiceMetadata'), invoice_metadata_right: t('documentStyle.sections.invoiceMetadata'),
    estimate_metadata_left: t('documentStyle.sections.estimateMetadata'), estimate_metadata_right: t('documentStyle.sections.estimateMetadata'),
    email_header_type: email('headerType'), email_header_content_position: email('contentPosition'), email_header_background_color: email('backgroundColor'),
    email_header_text: email('text'), email_header_text_color: email('textColor'), email_logo_display_width: t('documentStyle.logoWidth'),
    email_footer_type: email('footerType'), email_footer_text_color: email('textColor'), email_footer_background_color: email('backgroundColor'),
    email_footer_profile_title: email('title'), email_footer_profile_subtitle: email('subtitle'), body_background_color: email('backgroundColor'),
    email_font_family: t('documentStyle.fontFamily'), public_view_logo_display_size: t('documentStyle.logoSize'),
    public_view_background_color: email('backgroundColor'), checkout_favorite_label: t('documentStyle.checkout.favoriteLabel'),
  }
}

function Editor({ administrationId, style, identities, customFields, tab, language, pinned }: EditorProps) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const [current, setCurrent] = useState(style)
  const [form, setForm] = useState(() => toForm(style))
  const save = useSaveDocumentStyle(administrationId, style.id)
  const remove = useDeleteDocumentStyle(administrationId)
  const data = usePreviewData(administrationId, i18n.getFixedT(language))
  const back = `/${administrationId}/settings/invoices_and_estimates#document-styles`
  const base = `/${administrationId}/document_styles/${style.id}`
  const sender = identities.find((i) => i.id === style.identity_id) ?? identities.find((i) => i.default) ?? identities[0]
  const errors = formErrors(save.error ?? remove.error, fieldLabels(t))
  const names = Object.fromEntries(customFields.map((f) => [`custom_field_${f.id}`, f.name]))
  const set = <K extends keyof StyleForm>(key: K, value: StyleForm[K]) => setForm((f) => ({ ...f, [key]: value }))

  const submit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate(form, { onSuccess: () => navigate(back) })
  }
  const setChannel = <K extends keyof ChannelForm>(key: K, value: ChannelForm[K]) => setForm((f) => ({ ...f, [key]: value }))
  const partsProps = { administrationId, style: current, onFile: setCurrent, form, errors }

  return (
    <Page
      title={style.name ?? ''}
      back={{ to: back, label: t('documentStyle.back') }}
      wide
      actions={
        <>
          <Menu label={t('documentStyle.previewLanguage', { code: language.toUpperCase() })} buttonClassName="btn btn--secondary" align="right">
            <ul className="menu__list">
              {previewLanguages.map((l) => (
                <li key={l}><Link to={`${base}/edit/${tab}/${l}`} aria-current={l === language ? 'true' : undefined}>{l.toUpperCase()}</Link></li>
              ))}
            </ul>
          </Menu>
          <Link className="btn btn--secondary" to={`${base}/edit_general`}>{t('documentStyle.general')}</Link>
          {style.default ? (
            <button type="button" className="btn btn--secondary btn--small" disabled aria-label={t('documentStyle.defaultCannotBeDeleted')} title={t('documentStyle.defaultCannotBeDeleted')}>
              {t('documentStyle.deleteLayout')}
            </button>
          ) : (
            <ConfirmDelete
              label={t('documentStyle.deleteLayout')}
              confirmLabel={t('documentStyle.confirmDeleteLayout')}
              text={t('settings.delete')}
              confirmText={t('settings.confirmDelete')}
              disabled={remove.isPending}
              onConfirm={() => remove.mutate(style.id ?? '', { onSuccess: () => navigate(back) })}
            />
          )}
        </>
      }
    >
      <div className="style-editor">
        <nav className="style-editor__tabs" aria-label={t('documentStyle.nav')}>
          {tabs.map((tb) => (
            <Link key={tb} className="style-editor__tab" to={`${base}/edit/${tb}${pinned ? `/${language}` : ''}`} aria-current={tb === tab ? 'page' : undefined}>
              {t(`documentStyle.tabs.${tb}`)}
            </Link>
          ))}
        </nav>
        <div className="style-editor__body">
          <form className="style-editor__settings" onSubmit={submit} noValidate>
            <div className="style-editor__parts">
              {tab === 'document' && <DocumentParts {...partsProps} set={set} sender={sender} customFields={customFields} names={names} />}
              {tab === 'email' && <EmailParts {...partsProps} set={setChannel} />}
              {tab === 'public_view' && <PublicViewParts {...partsProps} set={setChannel} />}
              {tab === 'checkout' && <CheckoutParts {...partsProps} set={setChannel} />}
            </div>
            <div className="style-editor__actions">
              <button className="btn btn--primary" disabled={save.isPending}>{t('documentStyle.save')}</button>
              <Link className="btn btn--secondary" to={back}>{t('documentStyle.goBack')}</Link>
            </div>
          </form>
          <div className="style-editor__preview">
            {tab === 'document' || tab === 'email' ? (
              <p className="style-editor__note">
                {t('documentStyle.conditionsNote')} <Link to={`/${administrationId}/settings/invoices_and_estimates#workflows`}>{t('documentStyle.conditionsLink')}</Link>.
              </p>
            ) : (
              <p className="style-editor__note">{t(`documentStyle.notLive.${tab}`)}</p>
            )}
            {data ? <Preview tab={tab} administrationId={administrationId} style={current} form={form} sender={sender} language={language} data={data} names={names} /> : <Loading />}
          </div>
        </div>
      </div>
    </Page>
  )
}

type PreviewProps = { tab: Tab; administrationId: string; style: DocumentStyle; form: StyleForm; sender: Identity | undefined; language: string; data: PreviewData; names: Record<string, string> }

function Preview({ tab, form, ...props }: PreviewProps) {
  const { t } = useTranslation()
  const note = props.data.source === 'invoice' ? t('documentStyle.dataFromInvoice', { number: props.data.invoiceNumber }) : t('documentStyle.dataExample')
  return (
    <>
      <p className="style-editor__source">{note}</p>
      {tab === 'document' && <StylePreview {...props} layout={form} />}
      {tab === 'email' && <EmailPreview {...props} form={form} />}
      {tab === 'public_view' && <PublicViewPreview {...props} form={form} />}
      {tab === 'checkout' && <CheckoutPreview {...props} form={form} />}
    </>
  )
}

type PartsProps = {
  administrationId: string
  style: DocumentStyle
  onFile: (style: DocumentStyle) => void
  form: StyleForm
  set: <K extends keyof StyleForm>(key: K, value: StyleForm[K]) => void
  errors: FormErrors
  sender: Identity | undefined
  customFields: CustomField[]
  names: Record<string, string>
}

type Part = 'logo' | 'font' | 'background' | 'layout' | 'invoiceSender' | 'estimateSender' | 'invoiceRecipient' | 'estimateRecipient' | 'invoiceMetadata' | 'estimateMetadata' | 'css' | 'advanced'

/** Which part holds each field, so a refused field opens its part. */
const partOf: Record<string, Part> = {
  logo_position: 'logo', logo_display_width: 'logo', logo_container_full_width: 'logo', font_family: 'font', font_size: 'font', address_position: 'layout',
  sender_address_width: 'layout', invoice_sender_address: 'invoiceSender', estimate_sender_address: 'estimateSender',
  invoice_recipient_address: 'invoiceRecipient', estimate_recipient_address: 'estimateRecipient', invoice_metadata_left: 'invoiceMetadata',
  invoice_metadata_right: 'invoiceMetadata', estimate_metadata_left: 'estimateMetadata', estimate_metadata_right: 'estimateMetadata',
  custom_css: 'css', print_on_stationery: 'advanced', paper_size: 'advanced',
}

/** The parts of the "Invoices and quotes" tab. */
function DocumentParts({ administrationId, style, onFile, form, set, errors, sender, customFields, names }: PartsProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState<Set<Part>>(new Set())
  const refused = new Set(Object.keys(partOf).filter((k) => errors.field(k)).map((k) => partOf[k]))
  const item = (part: Part, icon: Parameters<typeof PartIcon>[0]['kind'], children: ReactNode) => (
    <AccordionItem
      key={part}
      title={t(`documentStyle.sections.${part}`)}
      icon={<PartIcon kind={icon} />}
      open={open.has(part) || refused.has(part)}
      onToggle={() => {
        const next = new Set(open)
        if (!next.delete(part)) {
          next.add(part)
        }
        setOpen(next)
      }}
    >
      {children}
    </AccordionItem>
  )
  const fieldErrors = (...keys: string[]) => keys.flatMap((k) => errors.field(k) ?? [])
  const senderValue = (field: string) => {
    if (!sender) {
      return []
    }
    if (field === 'address') {
      return addressLines(sender)
    }
    const custom = customFieldId(field)
    if (custom) {
      const v = sender.custom_fields?.find((f) => String(f.id) === custom)?.value
      return v ? [v] : []
    }
    const v = (sender as Record<string, unknown>)[field]
    return typeof v === 'string' && v !== '' ? [v] : []
  }

  return (
    <div className="style-accordion">
      <Problems messages={errors.general} />
      {item('logo', 'image', (
        <>
          <StyleFileField administrationId={administrationId} style={style} kind="logo" onChange={onFile} />
          <Segmented label={t('documentStyle.logoPosition')} value={form.logo_position} options={(['left', 'center', 'right'] as const).map((v) => ({ value: v, label: t(`documentStyle.positions.${v}`), icon: <AlignIcon align={v} /> }))} onChange={(v) => set('logo_position', v)} errors={errors.field('logo_position')} />
          <Range label={t('documentStyle.logoWidth')} value={form.logo_display_width} min={10} max={100} unit="%" errors={errors.field('logo_display_width')} onChange={(v) => set('logo_display_width', v)} />
          <Toggle label={t('documentStyle.fullWidth')} checked={form.logo_container_full_width} onChange={(on) => set('logo_container_full_width', on)} />
        </>
      ))}
      {item('font', 'font', (
        <>
          <SelectInput label={t('documentStyle.fontFamily')} value={form.font_family} errors={errors.field('font_family')} onChange={(e) => set('font_family', e.target.value)}>
            {fonts.map((f) => <option key={f} value={f}>{t(`documentStyle.fonts.${f}`)}</option>)}
          </SelectInput>
          <SelectInput label={t('documentStyle.fontSize')} value={String(form.font_size)} errors={errors.field('font_size')} onChange={(e) => set('font_size', Number(e.target.value))}>
            {fontSizes.map((n) => <option key={n} value={n}>{n}pt</option>)}
          </SelectInput>
        </>
      ))}
      {item('background', 'image', (
        <>
          <StyleFileField administrationId={administrationId} style={style} kind="background" onChange={onFile} />
          <FormNotice>{t('documentStyle.backgroundPreview')}</FormNotice>
          <label className="check">
            <input type="checkbox" checked={form.hide_sender_address} onChange={(e) => set('hide_sender_address', e.target.checked)} />
            <span>{t('documentStyle.hideSender')}</span>
          </label>
        </>
      ))}
      {item('layout', 'layout', (
        <>
          <Segmented label={t('documentStyle.addressPosition')} value={form.address_position} options={(['left', 'right'] as const).map((v) => ({ value: v, label: t(`documentStyle.positions.${v}`), icon: <AlignIcon align={v} /> }))} onChange={(v) => set('address_position', v)} errors={errors.field('address_position')} />
          <Range label={t('documentStyle.senderWidth')} value={form.sender_address_width} min={100} max={400} unit="px" errors={errors.field('sender_address_width')} onChange={(v) => set('sender_address_width', v)} />
        </>
      ))}
      {(['invoice', 'estimate'] as const).map((kind) =>
        item(`${kind}Sender`, 'card', (
          <>
            <p className="hint">{t('documentStyle.senderHelp')}</p>
            <FieldColumns
              titles={[t('documentStyle.show')]}
              columns={[form[`${kind}_sender_address`]]}
              fields={withCustomFields(senderFields, customFields, ['identity'])}
              blankLines
              value={senderValue}
              names={names}
              onChange={([lines]) => set(`${kind}_sender_address`, lines ?? [])}
            />
            <FieldErrors messages={fieldErrors(`${kind}_sender_address`)} />
          </>
        )),
      )}
      {(['invoice', 'estimate'] as const).map((kind) =>
        item(`${kind}Recipient`, 'card', (
          <>
            <p className="hint">{t(`documentStyle.${kind}RecipientHelp`)}</p>
            <FieldColumns
              titles={[t('documentStyle.show')]}
              columns={[form[`${kind}_recipient_address`]]}
              fields={withCustomFields(customerFields, customFields, ['contact'])}
              names={names}
              onChange={([lines]) => set(`${kind}_recipient_address`, lines ?? [])}
            />
            <FieldErrors messages={fieldErrors(`${kind}_recipient_address`)} />
          </>
        )),
      )}
      {(['invoice', 'estimate'] as const).map((kind) =>
        item(`${kind}Metadata`, 'list', (
          <>
            <p className="hint">{t(`documentStyle.${kind}MetadataHelp`)}</p>
            <FieldColumns
              titles={[t('documentStyle.left'), t('documentStyle.right')]}
              columns={[form[`${kind}_metadata_left`], form[`${kind}_metadata_right`]]}
              fields={kind === 'invoice'
                ? withCustomFields(invoiceMetadataFields, customFields, ['sales_invoice', 'contact'])
                : withCustomFields(estimateMetadataFields, customFields, ['estimate', 'contact'])}
              names={names}
              onChange={([left, right]) => {
                set(`${kind}_metadata_left`, left ?? [])
                set(`${kind}_metadata_right`, right ?? [])
              }}
            />
            <FieldErrors messages={fieldErrors(`${kind}_metadata_left`, `${kind}_metadata_right`)} />
          </>
        )),
      )}
      {item('css', 'code', (
        <>
          <TextArea label={t('documentStyle.customCss')} help={t('documentStyle.customCssHelp')} rows={8} maxLength={20000} spellCheck={false} value={form.custom_css} errors={errors.field('custom_css')} onChange={(e) => set('custom_css', e.target.value)} />
          <StyleImagesField administrationId={administrationId} style={style} onChange={onFile} />
        </>
      ))}
      {item('advanced', 'cog', (
        <>
          <Toggle label={t('documentStyle.ownStationery')} tip={t('documentStyle.ownStationeryHelp')} checked={form.print_on_stationery} onChange={(on) => set('print_on_stationery', on)} />
          <Toggle label={t('documentStyle.showQr')} tip={t('documentStyle.showQrHelp')} checked={form.show_epc_code} onChange={(on) => set('show_epc_code', on)} />
          <Segmented label={t('documentStyle.paperSize')} value={form.paper_size} options={(['A4', 'letter'] as const).map((v) => ({ value: v, label: t(`documentStyle.paperSizes.${v}`) }))} onChange={(v) => set('paper_size', v)} errors={errors.field('paper_size')} />
        </>
      ))}
    </div>
  )
}
