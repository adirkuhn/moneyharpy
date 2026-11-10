import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { Block } from '../settings/Block'
import { type CustomField, type CustomFieldSource, useCustomFields, useDeleteCustomField, useReorderCustomFields } from '../settings/queries'

const sources: CustomFieldSource[] = ['identity', 'contact', 'sales_invoice', 'estimate']

/** Custom fields per kind of record, in their order, with edit and delete. */
export function CustomFieldsBlock({ administrationId }: { administrationId: string }) {
  const { t } = useTranslation()
  const fields = useCustomFields(administrationId)
  const here = `/${administrationId}/settings/invoices_and_estimates`
  const add = `/${administrationId}/custom_fields/new?${new URLSearchParams({ return_to: here })}`
  const groups = sources.map((source) => ({ source, fields: fields.data?.filter((f) => f.source === source) ?? [] })).filter((g) => g.fields.length > 0)

  return (
    <Block
      id="custom-fields"
      title={t('invoicing.parts.customFields')}
      subtitle={t('invoicing.customFieldsSubtitle')}
      action={<Link className="btn btn--accent" to={add}>{t('settings.add')}</Link>}
    >
      {fields.data && groups.length === 0 && <p className="empty">{t('invoicing.noCustomFields')}</p>}
      {groups.map((g) => <FieldTable key={g.source} administrationId={administrationId} source={g.source} fields={g.fields} />)}
    </Block>
  )
}

type TableProps = { administrationId: string; source: CustomFieldSource; fields: CustomField[] }

function FieldTable({ administrationId, source, fields }: TableProps) {
  const { t } = useTranslation()
  const reorder = useReorderCustomFields(administrationId)
  const remove = useDeleteCustomField(administrationId)
  const title = t(`invoicing.sources.${source}`)
  const move = (index: number, by: number) => {
    const ids = fields.map((f) => f.id)
    ;[ids[index], ids[index + by]] = [ids[index + by] ?? '', ids[index] ?? '']
    reorder.mutate({ source, ids })
  }
  return (
    <table className="data-table" aria-label={title}>
      <thead>
        <tr>
          <th scope="col">{title}</th>
          <th scope="col"><span className="sr-only">{t('settings.edit')}</span></th>
        </tr>
      </thead>
      <tbody>
        {fields.map((f, index) => (
          <tr key={f.id}>
            <td>{f.name}</td>
            <td className="data-table__actions">
              <button type="button" className="icon-btn" aria-label={t('invoicing.moveUp', { name: f.name })} disabled={index === 0 || reorder.isPending} onClick={() => move(index, -1)}>↑</button>
              <button type="button" className="icon-btn" aria-label={t('invoicing.moveDown', { name: f.name })} disabled={index === fields.length - 1 || reorder.isPending} onClick={() => move(index, 1)}>↓</button>
              <Link className="btn btn--secondary btn--small" aria-label={t('invoicing.editName', { name: f.name })} to={`/${administrationId}/custom_fields/${f.id}/edit`}>{t('settings.edit')}</Link>
              <ConfirmDelete
                label={t('invoicing.deleteName', { name: f.name })}
                confirmLabel={t('invoicing.confirmDeleteName', { name: f.name })}
                text={t('settings.delete')}
                confirmText={t('settings.confirmDelete')}
                disabled={remove.isPending}
                onConfirm={() => remove.mutate(f.id)}
              />
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
