import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { ConfirmPanel, DetailBlock, DetailList, type DetailRow } from '../detail/Detail'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { CheckboxInput, formErrors, TextArea, TextInput } from '../forms/Form'
import { Loading } from '../layout/Loading'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { MoreMenu } from '../list/ListActions'
import { countryName } from '../settings/format'
import {
  type Contact, contactName, type PersonChanges, useAddNote, useArchiveContact, useContact, useDeleteContact, useDeleteNote,
  useDeletePerson, useSaveContact, useSavePerson,
} from './queries'

/** One contact: its details, contact people and notes (/contacts/:id). */
export function ContactPage() {
  const { t } = useTranslation()
  const { administrationId = '', contactId = '' } = useParams()
  const navigate = useNavigate()
  const contact = useContact(administrationId, contactId)
  const archive = useArchiveContact(administrationId, contactId)
  const restore = useSaveContact(administrationId, contactId)
  const remove = useDeleteContact(administrationId, contactId)
  const [confirming, setConfirming] = useState(false)

  if (contact.error instanceof ApiError && contact.error.status < 500) {
    return <NotFoundPage />
  }
  if (!contact.data) {
    return <Loading />
  }
  const c = contact.data
  const list = `/${administrationId}/contacts`
  const problems = formErrors(archive.error ?? restore.error ?? remove.error, {}).general
  const actions = (
    <>
      <Link className="btn btn--secondary" to={`${list}/${c.id}/edit`}>{t('contacts.edit')}</Link>
      <MoreMenu
        items={[
          c.archived
            ? { label: t('contacts.restore'), disabled: restore.isPending, onSelect: () => restore.mutate({ archived: false }) }
            : { label: t('contacts.archive'), disabled: archive.isPending, onSelect: () => archive.mutate() },
          { label: t('contacts.delete'), danger: true, onSelect: () => setConfirming(true) },
        ]}
      />
    </>
  )

  return (
    <Page title={contactName(c)} subtitle={t('contacts.number', { id: c.customer_id })} back={{ to: list, label: t('contacts.back') }} actions={actions}>
      <Problems messages={problems} />
      {confirming && (
        <ConfirmPanel
          message={t('contacts.confirmDeleteText')}
          confirmLabel={t('contacts.confirmDelete')}
          disabled={remove.isPending}
          onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(list) })}
          onCancel={() => setConfirming(false)}
        />
      )}
      {c.archived && <p className="notice">{t('contacts.archivedNotice')}</p>}
      <Details contact={c} />
      <People administrationId={administrationId} contact={c} />
      <Notes administrationId={administrationId} contact={c} />
    </Page>
  )
}

function Details({ contact: c }: { contact: Contact }) {
  const { t, i18n } = useTranslation()
  const address = [c.address1, c.address2, [c.zipcode, c.city].filter(Boolean).join(' '), countryName(i18n.language, c.country ?? '')].filter(Boolean)
  const rows: DetailRow[] = [
    [t('contacts.address'), address.length > 0 && address.map((line) => <span key={line} className="details__line">{line}</span>)],
    [t('contacts.invoiceEmail'), c.send_invoices_to_email],
    [t('contacts.attention'), c.send_invoices_to_attention],
    [t('contacts.phone'), c.phone],
    [t('contacts.deliveryMethod'), c.delivery_method && t(`contacts.delivery.${c.delivery_method}`)],
    [t('contacts.taxNumber'), c.tax_number],
    [t('contacts.chamberOfCommerce'), c.chamber_of_commerce],
    [t('contacts.bankAccount'), c.bank_account],
    [t('contacts.siIdentifier'), c.si_identifier && `${c.si_identifier_type ?? ''} ${c.si_identifier}`.trim()],
    [t('contacts.mandate'), c.sepa_active && [c.sepa_iban, c.sepa_mandate_id, c.sepa_mandate_date].filter(Boolean).join(' · ')],
    ...(c.custom_fields ?? []).map((f): DetailRow => [f.name ?? '', f.value]),
  ]
  return (
    <DetailBlock title={t('contacts.detailsTitle')}>
      <DetailList rows={rows} />
    </DetailBlock>
  )
}

const emptyPerson: Required<PersonChanges> = { firstname: '', lastname: '', email: '', phone: '', department: '' }

function People({ administrationId, contact }: { administrationId: string; contact: Contact }) {
  const { t } = useTranslation()
  const [adding, setAdding] = useState(false)
  const [form, setForm] = useState(emptyPerson)
  const save = useSavePerson(administrationId, contact.id ?? '')
  const remove = useDeletePerson(administrationId, contact.id ?? '')
  const labels = { firstname: t('contacts.firstname'), lastname: t('contacts.lastname'), email: t('contacts.email'), phone: t('contacts.phone'), department: t('contacts.department') }
  const errors = formErrors(save.error ?? remove.error, labels)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate(form, { onSuccess: () => { setAdding(false); setForm(emptyPerson) } })
  }
  const people = contact.contact_people ?? []
  return (
    <DetailBlock
      title={t('contacts.people')}
      actions={!adding && <button type="button" className="btn btn--secondary" onClick={() => setAdding(true)}>{t('contacts.addPerson')}</button>}
    >
      {people.length === 0 && !adding && <p className="empty">{t('contacts.noPeople')}</p>}
      {people.length > 0 && (
        <ul className="people">
          {people.map((p) => {
            const name = [p.firstname, p.lastname].filter(Boolean).join(' ')
            return (
              <li key={p.id} className="people__row">
                <div>
                  <p className="people__name">{name}</p>
                  {[p.department, p.email, p.phone].filter(Boolean).map((detail) => <p key={detail} className="hint">{detail}</p>)}
                </div>
                <ConfirmDelete label={t('contacts.deletePerson', { name })} confirmLabel={t('contacts.confirmDeletePerson', { name })} text={t('contacts.remove')} confirmText={t('contacts.confirmRemove')} disabled={remove.isPending} onConfirm={() => remove.mutate(String(p.id ?? ''))} />
              </li>
            )
          })}
        </ul>
      )}
      {adding && (
        <form className="form-sheet" onSubmit={submit} noValidate>
          <Problems messages={errors.general} />
          {(Object.keys(labels) as (keyof typeof labels)[]).map((field) => (
            <TextInput key={field} label={labels[field]} maxLength={255} type={field === 'email' ? 'email' : 'text'} value={form[field]} errors={errors.field(field)} onChange={(e) => setForm({ ...form, [field]: e.target.value })} />
          ))}
          <div className="form-sheet__actions">
            <button className="btn btn--primary" disabled={save.isPending}>{t('contacts.savePerson')}</button>
            <button type="button" className="btn btn--secondary" onClick={() => setAdding(false)}>{t('common.cancel')}</button>
          </div>
        </form>
      )}
    </DetailBlock>
  )
}

function Notes({ administrationId, contact }: { administrationId: string; contact: Contact }) {
  const { t, i18n } = useTranslation()
  const [text, setText] = useState('')
  const [todo, setTodo] = useState(false)
  const add = useAddNote(administrationId, contact.id ?? '')
  const remove = useDeleteNote(administrationId, contact.id ?? '')
  const errors = formErrors(add.error ?? remove.error, { note: t('contacts.newNote') })
  const submit = (e: FormEvent) => {
    e.preventDefault()
    add.mutate({ note: text, todo }, { onSuccess: () => { setText(''); setTodo(false) } })
  }
  const notes = contact.notes ?? []
  return (
    <DetailBlock title={t('contacts.notes')}>
      {notes.length > 0 && (
        <ul className="notes">
          {notes.map((n) => (
            <li key={n.id} className="notes__row">
              <div>
                <p className="notes__text">{n.note}</p>
                <p className="hint">
                  {n.todo && <span className="ledger__state">{n.completed_at ? t('contacts.done') : t('contacts.todo')}</span>}{' '}
                  {new Date(n.created_at ?? '').toLocaleDateString(i18n.language, { dateStyle: 'medium' })}
                </p>
              </div>
              <ConfirmDelete label={t('contacts.deleteNote')} confirmLabel={t('contacts.confirmDeleteNote')} text={t('contacts.remove')} confirmText={t('contacts.confirmRemove')} disabled={remove.isPending} onConfirm={() => remove.mutate(n.id ?? '')} />
            </li>
          ))}
        </ul>
      )}
      <form className="form-sheet" onSubmit={submit} noValidate>
        <Problems messages={errors.general} />
        <TextArea label={t('contacts.newNote')} maxLength={10000} value={text} errors={errors.field('note')} onChange={(e) => setText(e.target.value)} />
        <CheckboxInput label={t('contacts.isTodo')} checked={todo} onChange={(e) => setTodo(e.target.checked)} />
        <div className="form-sheet__actions">
          <button className="btn btn--primary" disabled={add.isPending || !text.trim()}>{t('contacts.addNote')}</button>
        </div>
      </form>
    </DetailBlock>
  )
}
