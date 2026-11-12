import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import type { TagValues } from '../forms/textile'
import { type ExtraTag, TextileEditor, TextileText } from '../forms/TextileEditor'
import { ListIcon } from '../layout/icons'
import { EditableDocumentForm } from './EditableDocumentForm'

type Props = {
  value: string
  values: TagValues
  editing: boolean
  onEdit: () => void
  onChange: (value: string) => void
  onSave: () => void
  onCancel: () => void
  saving: boolean
  errors?: string[]
  /** The workflow the text comes from, edited at its own page and back here. */
  workflow?: { name: string; to: string }
  extraTags?: ExtraTag[]
}

/**
 * The payment conditions at the bottom of the invoice, after Moneybird: the
 * text with its tags filled in, which opens its editor when clicked: a card
 * with the textile editor, a link to the workflow it came from, and Save
 * and Cancel.
 */
export function PaymentConditions({ value, values, editing, onEdit, onChange, onSave, onCancel, saving, errors, workflow, extraTags }: Props) {
  const { t } = useTranslation()
  const text = value ? <span className="invoice-paper__conditions"><TextileText text={value} values={values} /></span> : <span className="invoice-paper__placeholder">{t('invoices.editor.addConditions')}</span>
  if (!editing) {
    return (
      <button type="button" className={`in-place__button invoice-paper__conditions-button${errors?.length ? ' is-invalid' : ''}`} aria-label={value ? t('invoices.editor.editConditions') : t('invoices.editor.addConditions')} onClick={onEdit}>
        {text}
        <span className="in-place__indicator"><ListIcon kind="edit" /></span>
      </button>
    )
  }
  // As on Moneybird, the card is laid over the conditions, which stay on the document underneath.
  return (
    <>
      <EditableDocumentForm
        className="conditions-form"
        label={t('invoices.paymentConditions')}
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            e.preventDefault()
            onCancel()
          }
        }}
      >
        <TextileEditor
          label={t('invoices.paymentConditions')}
          value={value}
          onChange={onChange}
          maxLength={1000}
          autoFocus
          errors={errors}
          extraTags={extraTags}
          labelExtra={workflow && <Link className="editable-document-form__link" to={workflow.to}>{t('invoices.editor.editInWorkflow', { name: workflow.name })}</Link>}
        />
        <div className="editable-document-form__buttons">
          <button type="button" className="btn btn--primary" disabled={saving} onClick={onSave}>{t('invoices.editor.save')}</button>
          <button type="button" className="btn btn--secondary" onClick={onCancel}>{t('common.cancel')}</button>
        </div>
      </EditableDocumentForm>
      <div className="invoice-paper__under-edit" aria-hidden="true" inert>{text}</div>
    </>
  )
}

