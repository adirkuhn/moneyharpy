import { type FormEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { CheckboxInput, FormRow, FormSection, formErrors, SelectInput, TextArea, TextInput } from '../forms/Form'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { type Identity, useIdentities } from '../settings/queries'
import { LineEditor } from './LineEditor'
import {
  type DocumentStyle,
  type DocumentStyleChanges,
  type FileKind,
  type Line,
  styleFileURL,
  useDeleteDocumentStyle,
  useDocumentStyle,
  useRemoveStyleFile,
  useSaveDocumentStyle,
  useUploadStyleFile,
} from './queries'

const positions = ['left', 'center', 'right'] as const
const fonts = ['lucida_grande', 'helvetica', 'arial', 'verdana', 'georgia', 'times_new_roman', 'courier'] as const
const senderFields = ['company_name', 'address', 'newline', 'email', 'phone', 'website', 'chamber_of_commerce', 'tax_number', 'bank_account_number', 'bank_account_bic', 'bank_account_name'] as const
const invoiceFields = ['invoice_id', 'invoice_date', 'due_date', 'reference', 'customer_id'] as const
const estimateFields = ['estimate_id', 'estimate_date', 'due_date', 'reference', 'customer_id'] as const
const layoutKeys = ['invoice_sender_address', 'invoice_metadata_left', 'invoice_metadata_right', 'estimate_sender_address', 'estimate_metadata_left', 'estimate_metadata_right'] as const
type LayoutKey = (typeof layoutKeys)[number]

type Form = Required<Omit<DocumentStyleChanges, 'default' | LayoutKey>> & { default: boolean } & Record<LayoutKey, Line[]>

const defaultSender: Line[] = [{ field: 'company_name', bold: true }, { field: 'address' }, { field: 'newline' }, { field: 'email' }, { field: 'phone' }]

function toForm(d: DocumentStyle | null, defaultIdentity: string): Form {
  // Responses type the lines loosely, as Moneybird's schema does.
  const lines = (l: Array<Record<string, string | boolean>> | undefined, fallback: Line[]): Line[] =>
    (l ?? fallback).map((x) => ({ field: String(x.field ?? ''), bold: x.bold === true, label: x.label === true }))
  return {
    name: d?.name ?? '',
    identity_id: d?.identity_id ?? defaultIdentity,
    default: d?.default ?? false,
    logo_container_full_width: d?.logo_container_full_width ?? false,
    logo_display_width: d?.logo_display_width ?? 50,
    logo_position: d?.logo_position ?? 'left',
    print_on_stationery: d?.print_on_stationery ?? false,
    paper_size: d?.paper_size ?? 'A4',
    address_position: d?.address_position ?? 'left',
    font_size: d?.font_size ?? 10,
    font_family: d?.font_family ?? 'lucida_grande',
    custom_css: d?.custom_css ?? '',
    invoice_sender_address: lines(d?.invoice_sender_address, defaultSender),
    invoice_metadata_left: lines(d?.invoice_metadata_left, [{ field: 'reference' }]),
    invoice_metadata_right: lines(d?.invoice_metadata_right, [{ field: 'invoice_date' }, { field: 'due_date' }]),
    estimate_sender_address: lines(d?.estimate_sender_address, defaultSender),
    estimate_metadata_left: lines(d?.estimate_metadata_left, [{ field: 'reference' }]),
    estimate_metadata_right: lines(d?.estimate_metadata_right, [{ field: 'estimate_date' }, { field: 'due_date' }]),
  }
}

/** Adds a document style (/document_styles/new) or edits one. */
export function DocumentStyleFormPage() {
  const { administrationId = '', styleId } = useParams()
  const existing = useDocumentStyle(administrationId, styleId)
  const identities = useIdentities(administrationId)
  if (existing.error instanceof ApiError && existing.error.status < 500) {
    return <NotFoundPage />
  }
  if (!identities.data || (styleId && !existing.data)) {
    return <Loading />
  }
  return <StyleForm key={existing.data?.id ?? 'new'} administrationId={administrationId} style={existing.data ?? null} identities={identities.data} />
}

type FormProps = { administrationId: string; style: DocumentStyle | null; identities: Identity[] }

function StyleForm({ administrationId, style, identities }: FormProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const defaultIdentity = identities.find((i) => i.default)?.id ?? ''
  const [form, setForm] = useState(() => toForm(style, defaultIdentity))
  const save = useSaveDocumentStyle(administrationId, style?.id)
  const remove = useDeleteDocumentStyle(administrationId)
  const back = `/${administrationId}/settings/invoices_and_estimates#document-styles`
  const labels = {
    name: t('documentStyle.name'), identity_id: t('documentStyle.sender'), logo_position: t('documentStyle.logoPosition'),
    logo_display_width: t('documentStyle.logoWidth'), paper_size: t('documentStyle.paperSize'), address_position: t('documentStyle.addressPosition'),
    font_family: t('documentStyle.fontFamily'), font_size: t('documentStyle.fontSize'), custom_css: t('documentStyle.customCss'),
  }
  const errors = formErrors(save.error ?? remove.error, labels)
  const isDefault = style?.default ?? false

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const { default: makeDefault, ...rest } = form
    save.mutate({ ...rest, ...(isDefault ? {} : { default: makeDefault }) } as DocumentStyleChanges, { onSuccess: () => navigate(back) })
  }
  const choice = (key: 'logo_position' | 'paper_size' | 'address_position' | 'font_family', values: readonly string[], text: (v: string) => string) => (
    <SelectInput label={labels[key]} width="medium" value={form[key]} errors={errors.field(key)} onChange={(e) => setForm({ ...form, [key]: e.target.value })}>
      {values.map((v) => <option key={v} value={v}>{text(v)}</option>)}
    </SelectInput>
  )
  const lines = (key: LayoutKey, title: string, fields: readonly string[]) => (
    <LineEditor title={title} fields={fields} lines={form[key]} onChange={(next) => setForm({ ...form, [key]: next })} />
  )

  return (
    <Page
      title={style ? t('documentStyle.edit') : t('documentStyle.new')}
      back={{ to: back, label: t('workflow.back') }}
      actions={style && !isDefault ? <ConfirmDelete label={t('settings.delete')} confirmLabel={t('settings.confirmDelete')} disabled={remove.isPending} onConfirm={() => remove.mutate(style.id ?? '', { onSuccess: () => navigate(back) })} /> : undefined}
    >
      <form className="form-sheet" onSubmit={submit} noValidate>
        <Problems messages={errors.general} />
        <FormSection>
          <TextInput label={labels.name} required maxLength={255} autoComplete="off" value={form.name} errors={errors.field('name')} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <SelectInput label={labels.identity_id} value={form.identity_id} errors={errors.field('identity_id')} onChange={(e) => setForm({ ...form, identity_id: e.target.value })}>
            {identities.map((i) => <option key={i.id} value={i.id}>{i.company_name}</option>)}
          </SelectInput>
          {!isDefault && <CheckboxInput label={t('documentStyle.default')} checked={form.default} onChange={(e) => setForm({ ...form, default: e.target.checked })} />}
        </FormSection>

        <FormSection title={t('documentStyle.logo')}>
          {style ? <FileField administrationId={administrationId} style={style} kind="logo" /> : <p className="empty">{t('documentStyle.saveFirst')}</p>}
          {choice('logo_position', positions, (v) => t(`documentStyle.positions.${v}`))}
          <TextInput
            label={labels.logo_display_width}
            type="number"
            inputMode="numeric"
            min={10}
            max={100}
            width="small"
            help={t('documentStyle.logoWidthHelp')}
            value={String(form.logo_display_width)}
            errors={errors.field('logo_display_width')}
            onChange={(e) => setForm({ ...form, logo_display_width: Number(e.target.value) })}
          />
          <CheckboxInput label={t('documentStyle.fullWidth')} checked={form.logo_container_full_width} onChange={(e) => setForm({ ...form, logo_container_full_width: e.target.checked })} />
        </FormSection>

        <FormSection title={t('documentStyle.background')}>
          {style && <FileField administrationId={administrationId} style={style} kind="background" />}
          <CheckboxInput label={t('documentStyle.printOnStationery')} checked={form.print_on_stationery} onChange={(e) => setForm({ ...form, print_on_stationery: e.target.checked })} />
        </FormSection>

        <FormSection>
          {choice('paper_size', ['A4', 'letter'], (v) => (v === 'A4' ? 'A4' : 'Letter'))}
          {choice('address_position', ['left', 'right'], (v) => t(`documentStyle.positions.${v}`))}
          {choice('font_family', fonts, (v) => t(`documentStyle.fonts.${v}`))}
          <SelectInput label={labels.font_size} width="small" value={String(form.font_size)} errors={errors.field('font_size')} onChange={(e) => setForm({ ...form, font_size: Number(e.target.value) })}>
            {[8, 9, 10, 11, 12, 13, 14].map((n) => <option key={n} value={n}>{n} pt</option>)}
          </SelectInput>
        </FormSection>

        <FormSection title={t('documentStyle.layout')}>
          <div className="layout-columns">
            <div>
              {lines('invoice_sender_address', `${t('documentStyle.invoiceLayout')}: ${t('documentStyle.senderAddress')}`, senderFields)}
              {lines('invoice_metadata_left', `${t('documentStyle.invoiceLayout')}: ${t('documentStyle.metadataLeft')}`, invoiceFields)}
              {lines('invoice_metadata_right', `${t('documentStyle.invoiceLayout')}: ${t('documentStyle.metadataRight')}`, invoiceFields)}
            </div>
            <div>
              {lines('estimate_sender_address', `${t('documentStyle.estimateLayout')}: ${t('documentStyle.senderAddress')}`, senderFields)}
              {lines('estimate_metadata_left', `${t('documentStyle.estimateLayout')}: ${t('documentStyle.metadataLeft')}`, estimateFields)}
              {lines('estimate_metadata_right', `${t('documentStyle.estimateLayout')}: ${t('documentStyle.metadataRight')}`, estimateFields)}
            </div>
          </div>
        </FormSection>

        <FormSection>
          <TextArea label={labels.custom_css} help={t('documentStyle.customCssHelp')} maxLength={20000} spellCheck={false} value={form.custom_css} errors={errors.field('custom_css')} onChange={(e) => setForm({ ...form, custom_css: e.target.value })} />
        </FormSection>

        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={save.isPending}>{t('settings.save')}</button>
          <Link className="btn btn--secondary" to={back}>{t('common.cancel')}</Link>
        </div>
      </form>
    </Page>
  )
}

const accepted: Record<FileKind, string> = { logo: 'image/png,image/jpeg,image/gif', background: 'image/png,image/jpeg,application/pdf' }

/** Shows, uploads and removes a document style's logo or background. */
function FileField({ administrationId, style, kind }: { administrationId: string; style: DocumentStyle; kind: FileKind }) {
  const { t } = useTranslation()
  const id = useId()
  const upload = useUploadStyleFile(administrationId, style.id ?? '')
  const remove = useRemoveStyleFile(administrationId, style.id ?? '')
  const [current, setCurrent] = useState(style)
  const what = t(`documentStyle.${kind}`)
  const hash = kind === 'logo' ? current.logo_hash : current.background_hash
  const errors = formErrors(upload.error ?? remove.error, { [kind]: what })
  const help = t(kind === 'logo' ? 'documentStyle.logoHelp' : 'documentStyle.backgroundHelp')

  return (
    <FormRow label={what} errors={[...(errors.field(kind) ?? []), ...errors.general]} help={upload.isPending ? t('documentStyle.uploading') : help}>
      {(control) => (
        <div className="file-field">
          {hash &&
            (kind === 'logo' || !hash ? (
              <img className="file-field__preview" src={styleFileURL(administrationId, current.id ?? '', kind, hash)} alt={t('documentStyle.preview', { what })} />
            ) : (
              <a href={styleFileURL(administrationId, current.id ?? '', kind, hash)} target="_blank" rel="noreferrer">{t('documentStyle.preview', { what })}</a>
            ))}
          <label className="btn btn--secondary btn--small file-field__pick" htmlFor={`${id}-file`}>
            {hash ? t('documentStyle.replace', { what }) : t('documentStyle.upload', { what })}
          </label>
          <input
            id={`${id}-file`}
            className="sr-only"
            type="file"
            accept={accepted[kind]}
            aria-label={t('documentStyle.upload', { what })}
            aria-describedby={control['aria-describedby']}
            onChange={(e) => {
              const data = e.target.files?.[0]
              if (data) {
                upload.mutate({ file: kind, data }, { onSuccess: setCurrent })
              }
              e.target.value = ''
            }}
          />
          {hash && (
            <button type="button" className="btn btn--secondary btn--small" disabled={remove.isPending} onClick={() => remove.mutate(kind, { onSuccess: setCurrent })}>
              {t('documentStyle.remove', { what })}
            </button>
          )}
        </div>
      )}
    </FormRow>
  )
}
