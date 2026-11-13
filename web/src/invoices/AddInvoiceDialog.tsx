import { type ReactNode, type RefObject, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { Problems } from '../auth/AuthLayout'
import type { Contact } from '../contacts/queries'
import { DatePicker } from '../forms/DatePicker'
import { type FormErrors, FormRow, TextInput } from '../forms/Form'
import { InfoIcon, ListIcon } from '../layout/icons'
import { ConfirmDialog, Modal } from '../layout/Modal'
import { PageIcon } from '../layout/pageIcons'
import type { DocumentStyle, InvoiceSettings, Workflow } from '../invoicing/queries'
import type { CustomField } from '../settings/queries'
import { ContactSelect } from './ContactSelect'
import { numberParts } from './details'
import { StylePreview } from './LayoutPicker'

export type ChosenContact = { id: string; name: string; address: string[] }

/** What the popup sets: the invoice's contact and the settings the document does not show. */
export type InvoiceSetup = {
  contact: ChosenContact
  reference: string
  /** Empty while locked: the invoice is dated on the day it is sent. */
  invoiceDate: string
  dateUnlocked: boolean
  /** The {id} part of the number, filled in by hand once unlocked. */
  sequenceId: string
  numberUnlocked: boolean
  /** Empty for the workflow the invoice gets by default. */
  workflowId: string
  /** Days until due; empty for the workflow's term. */
  term: string
  styleId: string
  /** Null while the workflow's price mode applies; set above the details. */
  inclTax: boolean | null
  /** Null while the workflow's currency applies; set above the details. */
  currency: string | null
  custom: Record<string, string>
}

type Props = {
  administrationId: string
  value: InvoiceSetup
  onChange: (change: Partial<InvoiceSetup>) => void
  onPickContact: (contact: Contact) => void
  /** The invoice workflows to choose from, and the one the invoice follows now. */
  flows: Workflow[]
  flow: Workflow | undefined
  styles: DocumentStyle[]
  style: DocumentStyle | undefined
  /** A saved invoice opens it as Moneybird's "Edit invoice": no subtitle nor layouts, and the contact's hover card. */
  existing: boolean
  /**
   * Over a recurring invoice: no invoice id nor date, which belong to the
   * invoices it makes, and no layouts, which its page sets.
   */
  recurring?: boolean
  /** Whether the settings behind Edit show from the start, as when the document's dates open it. */
  advancedOpen?: boolean
  /** The document the popup is laid over, aligned with it as Moneybird's popup--align-document. */
  anchor?: RefObject<HTMLElement | null>
  dueTerm: number
  /** The number of an invoice that was sent already; it no longer changes. */
  sentNumber?: string | null
  numbering: InvoiceSettings | undefined
  customFields: CustomField[]
  homeCountry: string
  /** This page, which the contact and custom field pages come back to. */
  here: string
  errors: FormErrors
  pending: boolean
  /** Cancel leaves a new invoice for the list, or closes the popup over a saved one. */
  cancel: { to: string } | { onClick: () => void }
  onSave: (then: 'save' | 'details') => void
  onClose: () => void
}

type Confirm = 'number' | 'date' | null

/**
 * Moneybird's "Add invoice" popup: the contact, a reference, and behind Edit
 * the invoice id, date, workflow, term and layout. Save creates the draft;
 * Invoice details creates it and opens its details. Over a saved invoice it is
 * "Edit invoice", without the layouts, which Change layout sets.
 */
export function AddInvoiceDialog(props: Props) {
  const { administrationId, value, onChange, flows, flow, styles, style, existing, recurring = false, dueTerm, sentNumber, numbering, customFields, errors, pending, cancel, onSave, onClose } = props
  const { t } = useTranslation()
  const formId = useId()
  const advancedId = useId()
  const [advanced, setAdvanced] = useState(!!props.advancedOpen)
  const [confirm, setConfirm] = useState<Confirm>(null)
  const p = (key: string, options: Record<string, unknown> = {}) => t(`invoices.popup.${key}`, options)

  const settingsLink = `/${administrationId}/settings/invoices_and_estimates`
  const next = numbering?.invoice_sequence_id ?? ''
  const parts = numberParts(numbering?.invoice_id_example ?? '', next)
  const days = value.term.trim() !== '' && Number.isFinite(Number(value.term)) ? Number(value.term) : dueTerm
  const setDays = (n: number) => onChange({ term: String(Math.min(365, Math.max(0, n))) })
  const lockButton = (label: string, onClick: () => void) => (
    <button type="button" className="btn btn--inline btn--icon" aria-label={label} title={label} onClick={onClick}>
      <PageIcon kind="lock" />
    </button>
  )
  const newField = `/${administrationId}/custom_fields/new?${new URLSearchParams({ source: 'sales_invoice', return_to: props.here })}`

  const footer = (
    <>
      {/* Save and Invoice details joined, as Moneybird's button group. */}
      <div className="btn-group">
        <button type="submit" form={formId} className="btn btn--primary" disabled={pending}>{p('save')}</button>
        <button type="button" className="btn btn--primary btn--with-icon" disabled={pending} onClick={() => onSave('details')}>
          <ListIcon kind="plus" />
          <span>{p('details')}</span>
        </button>
      </div>
      {'to' in cancel
        ? <Link className="btn btn--secondary" to={cancel.to}>{t('common.cancel')}</Link>
        : <button type="button" className="btn btn--secondary" onClick={cancel.onClick}>{t('common.cancel')}</button>}
    </>
  )

  return (
    <Modal title={existing ? p('editTitle') : recurring ? t('recurring.add') : t('invoices.newTitle')} subtitle={existing ? undefined : p('subtitle')} onClose={onClose} footer={footer} className="add-invoice" anchor={props.anchor}>
      <form
        id={formId}
        className="add-invoice__form"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          onSave('save')
        }}
      >
        <Problems messages={errors.general} />
        <ContactSelect
          administrationId={administrationId}
          value={value.contact.name}
          errors={errors.field('contact_id')}
          homeCountry={props.homeCountry}
          returnTo={props.here}
          onChange={props.onPickContact}
          hoverCard={existing && value.contact.id ? { id: value.contact.id, name: value.contact.name } : undefined}
        />
        <TextInput label={t('invoices.reference')} maxLength={1000} value={value.reference} errors={errors.field('reference')} onChange={(e) => onChange({ reference: e.target.value })} />

        {customFields.map((c, i) => (
          <TextInput
            key={c.id}
            label={c.name}
            maxLength={255}
            value={value.custom[c.id] ?? ''}
            errors={errors.field(`custom_fields_attributes.${i}`)}
            onChange={(e) => onChange({ custom: { ...value.custom, [c.id]: e.target.value } })}
          />
        ))}
        <div className="add-invoice__box">
          <div className="add-invoice__summary">
            <p role="note">{existing ? p('editSummary', { workflow: flow?.name ?? '', count: days }) : p('summary', { workflow: flow?.name ?? '', style: style?.name ?? '', count: days })}</p>
            <button
              type="button"
              className="btn btn--secondary btn--icon"
              title={advanced ? t('common.close') : t('invoices.edit')}
              aria-expanded={advanced}
              aria-controls={advancedId}
              onClick={() => setAdvanced(!advanced)}
            >
              {advanced ? <PageIcon kind="close" /> : <ListIcon kind="edit" />}
              <span className="sr-only">{advanced ? t('common.close') : t('invoices.edit')}</span>
            </button>
          </div>
          {advanced && (
            <fieldset id={advancedId} className="add-invoice__advanced">
              <legend className="sr-only">{t('invoices.settingsTitle')}</legend>
              {!recurring && (
                <>
                  <FormRow
                    label={p('invoiceId')}
                    errors={errors.field('invoice_sequence_id')}
                    labelExtra={!sentNumber && <Tip text={p('numberTip')}>{p('numberTip')} <Link to={settingsLink}>{p('companySettings')}</Link>.</Tip>}
                    action={!sentNumber && (value.numberUnlocked
                      ? lockButton(p('lockNumber'), () => onChange({ numberUnlocked: false, sequenceId: '' }))
                      : lockButton(p('unlockNumber'), () => setConfirm('number')))}
                  >
                    {({ labelId: _, ...control }) => (
                      <span className="add-invoice__number">
                        {!sentNumber && parts.prefix && <span className="add-invoice__affix">{parts.prefix}</span>}
                        <input
                          type="text"
                          inputMode="numeric"
                          maxLength={10}
                          {...control}
                          disabled={!!sentNumber || !value.numberUnlocked}
                          value={sentNumber || (value.numberUnlocked ? value.sequenceId : next)}
                          onChange={(e) => onChange({ sequenceId: e.target.value })}
                        />
                        {!sentNumber && parts.suffix && <span className="add-invoice__affix">{parts.suffix}</span>}
                      </span>
                    )}
                  </FormRow>

                  <FormRow
                    label={t('invoices.invoiceDate')}
                    width="medium"
                    errors={errors.field('invoice_date')}
                    action={!sentNumber && (value.dateUnlocked
                      ? lockButton(p('lockDate'), () => onChange({ dateUnlocked: false, invoiceDate: '' }))
                      : lockButton(p('unlockDate'), () => setConfirm('date')))}
                  >
                    {({ labelId: _, ...control }) => value.dateUnlocked || sentNumber
                      ? <DatePicker {...control} disabled={!!sentNumber} value={value.invoiceDate} onChange={(d) => onChange({ invoiceDate: d })} />
                      : <DatePicker {...control} disabled value="" onChange={() => {}} />}
                  </FormRow>
                </>
              )}

              <FormRow
                label={t('invoices.workflow')}
                width="medium"
                errors={errors.field('workflow_id')}
                labelExtra={<Tip text={p('workflowTip')} />}
                action={flow?.id && (
                  <Link className="btn btn--secondary btn--icon" to={`/${administrationId}/workflows/${flow.id}`} aria-label={p('editWorkflow')} title={p('editWorkflow')}>
                    <ListIcon kind="edit" />
                  </Link>
                )}
              >
                {({ labelId: _, ...control }) => (
                  <select {...control} value={value.workflowId || flow?.id || ''} onChange={(e) => onChange({ workflowId: e.target.value })}>
                    {flows.map((w) => <option key={w.id} value={w.id}>{w.name}</option>)}
                  </select>
                )}
              </FormRow>

              <FormRow label={p('dueAfter')} width="small" errors={errors.field('first_due_interval')}>
                {({ labelId: _, ...control }) => (
                  <span className="add-invoice__days">
                    <span className="add-invoice__number-field">
                      <button type="button" className="add-invoice__step add-invoice__step--down" aria-label={p('decrease')} onClick={() => setDays(days - 1)}>−</button>
                      <input
                        type="number"
                        inputMode="numeric"
                        min={0}
                        max={365}
                        {...control}
                        aria-label={p('daysLabel')}
                        value={value.term.trim() === '' ? String(dueTerm) : value.term}
                        onChange={(e) => onChange({ term: e.target.value })}
                      />
                      <button type="button" className="add-invoice__step add-invoice__step--up" aria-label={p('increase')} onClick={() => setDays(days + 1)}>+</button>
                    </span>
                    <span>{p('days')}</span>
                  </span>
                )}
              </FormRow>

              {!existing && !recurring && (
                <>
                  <hr className="add-invoice__rule" />
                  <LayoutCards
                    label={p('layout')}
                    administrationId={administrationId}
                    styles={styles}
                    current={style?.id ?? ''}
                    errors={errors.field('document_style_id')}
                    onPick={(id) => onChange({ styleId: id })}
                  />
                </>
              )}
              <div className="add-invoice__custom">
                <Link className="btn btn--secondary btn--small btn--with-icon" to={newField}>
                  <ListIcon kind="plus" />
                  <span>{p('addCustomField')}</span>
                </Link>
              </div>
            </fieldset>
          )}
        </div>
      </form>

      {confirm === 'number' && (
        <ConfirmDialog
          title={p('unlockNumberTitle')}
          description={p('unlockNumberText')}
          confirmLabel={p('fillIn')}
          onCancel={() => setConfirm(null)}
          onConfirm={() => {
            setConfirm(null)
            onChange({ numberUnlocked: true, sequenceId: value.sequenceId || next })
          }}
        />
      )}
      {confirm === 'date' && (
        <ConfirmDialog
          title={p('unlockDateTitle')}
          description={p('unlockDateText')}
          confirmLabel={p('fillIn')}
          onCancel={() => setConfirm(null)}
          onConfirm={() => {
            setConfirm(null)
            onChange({ dateUnlocked: true })
          }}
        />
      )}
    </Modal>
  )
}

/**
 * A short explanation behind an information mark, as Moneybird's tooltips.
 * With content of its own, such as a link, it shows in a bubble while the
 * mark is hovered or what it holds has focus.
 */
export function Tip({ text, children }: { text: string; children?: ReactNode }) {
  if (children) {
    return (
      <span className="add-invoice__tip add-invoice__tip--bubble">
        <InfoIcon />
        <span className="add-invoice__bubble">{children}</span>
      </span>
    )
  }
  return (
    <span className="add-invoice__tip" title={text}>
      <InfoIcon />
      <span className="sr-only">{text}</span>
    </span>
  )
}

type CardsProps = { label: string; administrationId: string; styles: DocumentStyle[]; current: string; errors?: string[]; onPick: (id: string) => void }

/** The document styles as radio cards with a small preview each. */
function LayoutCards({ label, administrationId, styles, current, errors, onPick }: CardsProps): ReactNode {
  const id = useId()
  return (
    <div className="form-input form-input--long">
      <span className="form-input__label" id={`${id}-label`}>{label}</span>
      <div className="form-input__body">
        <div className="layout-cards" role="radiogroup" aria-labelledby={`${id}-label`}>
          {styles.map((s) => (
            <label key={s.id} className={`layout-cards__card${s.id === current ? ' is-checked' : ''}`}>
              <input type="radio" name={id} value={s.id} checked={s.id === current} onChange={() => onPick(s.id ?? '')} />
              <span className="layout-cards__name">{s.name}</span>
              <StylePreview administrationId={administrationId} style={s} />
            </label>
          ))}
        </div>
        {errors && errors.length > 0 && <p className="form-input__message form-input__message--error">{errors.join(' ')}</p>}
      </div>
    </div>
  )
}
