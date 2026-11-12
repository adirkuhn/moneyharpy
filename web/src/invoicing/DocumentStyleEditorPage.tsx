import { type FormEvent, type ReactNode, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { Tip } from '../contacts/ContactFormParts'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { FormNotice, FormRow, formErrors, SelectInput, TextArea } from '../forms/Form'
import { addressLines, defaultSenderLines } from '../invoices/paper'
import { Loading } from '../layout/Loading'
import { Menu } from '../layout/Menu'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { type Identity, useIdentities } from '../settings/queries'
import { AccordionItem, PartIcon } from './Accordion'
import { FieldColumns } from './FieldColumns'
import { type DocumentStyle, type DocumentStyleChanges, useDeleteDocumentStyle, useDocumentStyle, useInvoiceSettings, useSaveDocumentStyle } from './queries'
import { StyleFileField } from './StyleFileField'
import { type PreviewLayout, StylePreview } from './StylePreview'
import { estimateMetadataFields, invoiceMetadataFields, normalLines, senderFields } from './styleLayout'
import '../contacts/contacts.css'
import './styleEditor.css'

const tabs = ['document', 'email', 'public_view', 'checkout'] as const
type Tab = (typeof tabs)[number]
const previewLanguages = ['en', 'nl'] as const
const fonts = ['lucida_grande', 'arial', 'helvetica', 'verdana', 'georgia', 'times_new_roman', 'courier'] as const
const fontSizes = [8, 9, 10, 11, 12, 13, 14]

type StyleForm = PreviewLayout & Required<Pick<DocumentStyleChanges, 'print_on_stationery' | 'paper_size' | 'custom_css'>>

function toForm(d: DocumentStyle): StyleForm {
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
    invoice_sender_address: normalLines(d.invoice_sender_address, defaultSenderLines),
    invoice_metadata_left: normalLines(d.invoice_metadata_left, [{ field: 'reference' }]),
    invoice_metadata_right: normalLines(d.invoice_metadata_right, [{ field: 'invoice_date' }, { field: 'due_date' }]),
    estimate_sender_address: normalLines(d.estimate_sender_address, defaultSenderLines),
    estimate_metadata_left: normalLines(d.estimate_metadata_left, [{ field: 'reference' }]),
    estimate_metadata_right: normalLines(d.estimate_metadata_right, [{ field: 'estimate_date' }, { field: 'due_date' }]),
  }
}

/** Moneybird's document style editor (/document_styles/:id/edit/:tab/:language): settings in parts beside a preview. */
export function DocumentStyleEditorPage() {
  const { i18n } = useTranslation()
  const { administrationId = '', styleId = '', tab, language } = useParams()
  const style = useDocumentStyle(administrationId, styleId)
  const identities = useIdentities(administrationId)
  const settings = useInvoiceSettings(administrationId)
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
      tab={tab as Tab}
      language={preview}
      pinned={language !== undefined}
      numbers={{ invoice: settings.data?.invoice_id_example ?? '', estimate: settings.data?.estimate_id_example ?? '' }}
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
  tab: Tab
  language: string
  /** Whether the address names the preview language, which the tabs then keep. */
  pinned: boolean
  numbers: { invoice: string; estimate: string }
}

function Editor({ administrationId, style, identities, tab, language, pinned, numbers }: EditorProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [current, setCurrent] = useState(style)
  const [form, setForm] = useState(() => toForm(style))
  const save = useSaveDocumentStyle(administrationId, style.id)
  const remove = useDeleteDocumentStyle(administrationId)
  const back = `/${administrationId}/settings/invoices_and_estimates#document-styles`
  const base = `/${administrationId}/document_styles/${style.id}`
  const sender = identities.find((i) => i.id === style.identity_id) ?? identities.find((i) => i.default) ?? identities[0]

  const submit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate(form, { onSuccess: () => navigate(back) })
  }

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
              {tab === 'document' ? (
                <DocumentParts administrationId={administrationId} style={current} onFile={setCurrent} form={form} setForm={setForm} sender={sender} error={save.error ?? remove.error} />
              ) : (
                <UnavailableParts tab={tab} />
              )}
            </div>
            <div className="style-editor__actions">
              <button className="btn btn--primary" disabled={save.isPending || tab !== 'document'}>{t('documentStyle.save')}</button>
              <Link className="btn btn--secondary" to={back}>{t('documentStyle.goBack')}</Link>
            </div>
          </form>
          <div className="style-editor__preview">
            <p className="style-editor__note">
              {t('documentStyle.conditionsNote')} <Link to={`/${administrationId}/settings/invoices_and_estimates#workflows`}>{t('documentStyle.conditionsLink')}</Link>.
            </p>
            <StylePreview administrationId={administrationId} style={current} layout={form} sender={sender} language={language} invoiceNumber={numbers.invoice} estimateNumber={numbers.estimate} />
          </div>
        </div>
      </div>
    </Page>
  )
}

type PartsProps = {
  administrationId: string
  style: DocumentStyle
  onFile: (style: DocumentStyle) => void
  form: StyleForm
  setForm: (form: StyleForm) => void
  sender: Identity | undefined
  error: unknown
}

type Part = 'logo' | 'font' | 'background' | 'layout' | 'invoiceSender' | 'estimateSender' | 'invoiceRecipient' | 'estimateRecipient' | 'invoiceMetadata' | 'estimateMetadata' | 'css' | 'advanced'

/** Which part holds each field, so a refused field opens its part. */
const partOf: Record<string, Part> = {
  logo_position: 'logo', logo_display_width: 'logo', logo_container_full_width: 'logo', font_family: 'font', font_size: 'font', address_position: 'layout',
  invoice_sender_address: 'invoiceSender', estimate_sender_address: 'estimateSender', invoice_metadata_left: 'invoiceMetadata', invoice_metadata_right: 'invoiceMetadata',
  estimate_metadata_left: 'estimateMetadata', estimate_metadata_right: 'estimateMetadata', custom_css: 'css', print_on_stationery: 'advanced', paper_size: 'advanced',
}

/** The parts of the "Invoices and quotes" tab. */
function DocumentParts({ administrationId, style, onFile, form, setForm, sender, error }: PartsProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState<Set<Part>>(new Set())
  const labels = {
    logo_position: t('documentStyle.logoPosition'), logo_display_width: t('documentStyle.logoWidth'), font_family: t('documentStyle.fontFamily'), font_size: t('documentStyle.fontSize'),
    address_position: t('documentStyle.addressPosition'), custom_css: t('documentStyle.customCss'), paper_size: t('documentStyle.paperSize'),
    invoice_sender_address: t('documentStyle.sections.invoiceSender'), estimate_sender_address: t('documentStyle.sections.estimateSender'),
    invoice_metadata_left: t('documentStyle.sections.invoiceMetadata'), invoice_metadata_right: t('documentStyle.sections.invoiceMetadata'),
    estimate_metadata_left: t('documentStyle.sections.estimateMetadata'), estimate_metadata_right: t('documentStyle.sections.estimateMetadata'),
  }
  const errors = formErrors(error, labels)
  const refused = new Set(Object.keys(labels).filter((k) => errors.field(k)).map((k) => partOf[k]))
  const item = (part: Part, icon: Parameters<typeof PartIcon>[0]['kind'], children: ReactNode) => (
    <AccordionItem
      key={part}
      title={t(`documentStyle.sections.${part}`)}
      icon={<PartIcon kind={icon} />}
      open={open.has(part) || refused.has(part)}
      onToggle={() => {
        const next = new Set(open)
        if (next.has(part)) {
          next.delete(part)
        } else {
          next.add(part)
        }
        setOpen(next)
      }}
    >
      {children}
    </AccordionItem>
  )
  const set = <K extends keyof StyleForm>(key: K, value: StyleForm[K]) => setForm({ ...form, [key]: value })
  const fieldErrors = (...keys: string[]) => keys.flatMap((k) => errors.field(k) ?? [])
  const senderValue = (field: string) => {
    if (!sender) {
      return []
    }
    if (field === 'address') {
      return addressLines(sender)
    }
    const v = (sender as Record<string, unknown>)[field]
    return typeof v === 'string' && v !== '' ? [v] : []
  }
  const notAvailable = <p className="hint style-editor__unavailable">{t('documentStyle.notAvailable')}</p>

  return (
    <div className="style-accordion">
      <Problems messages={errors.general} />
      {item('logo', 'image', (
        <>
          <StyleFileField administrationId={administrationId} style={style} kind="logo" onChange={onFile} />
          <Segmented label={labels.logo_position} value={form.logo_position} options={(['left', 'center', 'right'] as const).map((v) => ({ value: v, label: t(`documentStyle.positions.${v}`), icon: <AlignIcon align={v} /> }))} onChange={(v) => set('logo_position', v)} errors={errors.field('logo_position')} />
          <FormRow label={labels.logo_display_width} errors={errors.field('logo_display_width')}>
            {({ labelId: _, ...control }) => (
              <span className="style-range">
                <input type="range" min={10} max={100} step={1} value={form.logo_display_width} onChange={(e) => set('logo_display_width', Number(e.target.value))} {...control} />
                <output aria-hidden="true">{form.logo_display_width}%</output>
              </span>
            )}
          </FormRow>
          <Toggle label={t('documentStyle.fullWidth')} checked={form.logo_container_full_width} onChange={(on) => set('logo_container_full_width', on)} />
        </>
      ))}
      {item('font', 'font', (
        <>
          <SelectInput label={labels.font_family} value={form.font_family} errors={errors.field('font_family')} onChange={(e) => set('font_family', e.target.value)}>
            {fonts.map((f) => <option key={f} value={f}>{t(`documentStyle.fonts.${f}`)}</option>)}
          </SelectInput>
          <SelectInput label={labels.font_size} value={String(form.font_size)} errors={errors.field('font_size')} onChange={(e) => set('font_size', Number(e.target.value))}>
            {fontSizes.map((n) => <option key={n} value={n}>{n}pt</option>)}
          </SelectInput>
        </>
      ))}
      {item('background', 'image', (
        <>
          <StyleFileField administrationId={administrationId} style={style} kind="background" onChange={onFile} />
          <FormNotice>{t('documentStyle.backgroundPreview')}</FormNotice>
          <label className="check is-disabled">
            <input type="checkbox" disabled />
            <span>{t('documentStyle.hideSender')}</span>
          </label>
          {notAvailable}
        </>
      ))}
      {item('layout', 'layout', (
        <>
          <Segmented label={labels.address_position} value={form.address_position} options={(['left', 'right'] as const).map((v) => ({ value: v, label: t(`documentStyle.positions.${v}`), icon: <AlignIcon align={v} /> }))} onChange={(v) => set('address_position', v)} errors={errors.field('address_position')} />
          <FormRow label={t('documentStyle.senderWidth')} help={t('documentStyle.notAvailable')}>
            {({ labelId: _, ...control }) => <input type="range" disabled {...control} />}
          </FormRow>
        </>
      ))}
      {(['invoice', 'estimate'] as const).map((kind) =>
        item(`${kind}Sender`, 'card', (
          <>
            <p className="hint">{t('documentStyle.senderHelp')}</p>
            <FieldColumns
              titles={[t('documentStyle.show')]}
              columns={[form[`${kind}_sender_address`]]}
              fields={senderFields}
              blankLines
              value={senderValue}
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
            {notAvailable}
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
              fields={kind === 'invoice' ? invoiceMetadataFields : estimateMetadataFields}
              onChange={([left, right]) => setForm({ ...form, [`${kind}_metadata_left`]: left ?? [], [`${kind}_metadata_right`]: right ?? [] })}
            />
            <FieldErrors messages={fieldErrors(`${kind}_metadata_left`, `${kind}_metadata_right`)} />
          </>
        )),
      )}
      {item('css', 'code', (
        <>
          <TextArea label={labels.custom_css} help={t('documentStyle.customCssHelp')} rows={8} maxLength={20000} spellCheck={false} value={form.custom_css} errors={errors.field('custom_css')} onChange={(e) => set('custom_css', e.target.value)} />
          <FormRow label={t('documentStyle.cssImages')} help={t('documentStyle.notAvailable')}>
            {({ labelId: _, ...control }) => <input type="file" disabled {...control} />}
          </FormRow>
        </>
      ))}
      {item('advanced', 'cog', (
        <>
          <Toggle label={t('documentStyle.ownStationery')} tip={t('documentStyle.ownStationeryHelp')} checked={form.print_on_stationery} onChange={(on) => set('print_on_stationery', on)} />
          <Toggle label={t('documentStyle.showQr')} tip={t('documentStyle.showQrHelp')} checked={false} disabled onChange={() => undefined} />
          <Segmented label={labels.paper_size} value={form.paper_size} options={(['A4', 'letter'] as const).map((v) => ({ value: v, label: t(`documentStyle.paperSizes.${v}`) }))} onChange={(v) => set('paper_size', v)} errors={errors.field('paper_size')} />
        </>
      ))}
    </div>
  )
}

/** The other tabs: Moneybird's parts, which MoneyHarpy has no settings for yet. */
function UnavailableParts({ tab }: { tab: Exclude<Tab, 'document'> }) {
  const { t } = useTranslation()
  const parts: Record<typeof tab, Array<{ part: string; icon: Parameters<typeof PartIcon>[0]['kind']; tip?: string }>> = {
    email: [{ part: 'header', icon: 'mail' }, { part: 'footer', icon: 'mail' }, { part: 'general', icon: 'cog' }],
    public_view: [
      { part: 'logo', icon: 'image' }, { part: 'general', icon: 'cog' },
      ...(['invoiceSender', 'estimateSender', 'invoiceRecipient', 'estimateRecipient'] as const).map((part) => ({ part, icon: 'card' as const, tip: t('documentStyle.onlyUnderDocument') })),
    ],
    checkout: [{ part: 'logo', icon: 'image' }, { part: 'basic', icon: 'cog' }, { part: 'favorite', icon: 'cog' }],
  }
  return (
    <div className="style-accordion">
      <FormNotice>{t(`documentStyle.tabUnavailable.${tab}`)}</FormNotice>
      {parts[tab].map(({ part, icon, tip }) => (
        <AccordionItem key={part} title={t(`documentStyle.sections.${part}`)} icon={<PartIcon kind={icon} />} open={false} disabled tip={tip ?? t('documentStyle.notAvailable')} onToggle={() => undefined} />
      ))}
    </div>
  )
}

function FieldErrors({ messages }: { messages: string[] }) {
  return messages.length ? <p className="form-input__message form-input__message--error">{messages.join(' ')}</p> : null
}

type SegmentedProps<T extends string> = { label: string; value: T; options: Array<{ value: T; label: string; icon?: ReactNode }>; errors?: string[]; onChange: (value: T) => void }

/** A labelled row of joined buttons for a short choice, such as left, centered or right. */
function Segmented<T extends string>({ label, value, options, errors, onChange }: SegmentedProps<T>) {
  const name = useId()
  return (
    <fieldset className="segmented style-segmented">
      <legend className="segmented__legend">{label}</legend>
      <div className="segmented__options">
        {options.map((o) => (
          <label key={o.value} className={`segmented__option${value === o.value ? ' is-checked' : ''}`} title={o.label}>
            <input type="radio" name={name} value={o.value} checked={value === o.value} aria-label={o.label} onChange={() => onChange(o.value)} />
            {o.icon ?? o.label}
          </label>
        ))}
      </div>
      {errors && errors.length > 0 && <p className="form-input__message form-input__message--error">{errors.join(' ')}</p>}
    </fieldset>
  )
}

/** An on-off switch with its label on the left, as in Moneybird's style editor. */
function Toggle({ label, tip, checked, disabled, onChange }: { label: string; tip?: string; checked: boolean; disabled?: boolean; onChange: (on: boolean) => void }) {
  return (
    <div className="style-toggle">
      <span className="style-toggle__label">
        <label>
          <span>{label}</span>
          <input type="checkbox" role="switch" className="switch" checked={checked} disabled={disabled} onChange={(e) => onChange(e.target.checked)} />
        </label>
        {tip && <Tip text={tip} />}
      </span>
    </div>
  )
}

function AlignIcon({ align }: { align: 'left' | 'center' | 'right' }) {
  const lines = { left: 'M4 6h16M4 10h10M4 14h16M4 18h10', center: 'M4 6h16M7 10h10M4 14h16M7 18h10', right: 'M4 6h16M10 10h10M4 14h16M10 18h10' }
  return (
    <svg viewBox="0 0 24 24" width="1.125em" height="1.125em" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" aria-hidden="true">
      <path d={lines[align]} />
    </svg>
  )
}
