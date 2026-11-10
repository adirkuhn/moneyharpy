import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { ConfirmDelete } from '../forms/ConfirmDelete'
import { Menu } from '../layout/Menu'
import { Block } from '../settings/Block'
import { useDeleteWorkflow, useWorkflows, type Workflow } from './queries'

/** Invoice and quote workflows in tables, purchase workflows as a placeholder. */
export function WorkflowsBlock({ administrationId }: { administrationId: string }) {
  const { t } = useTranslation()
  const workflows = useWorkflows(administrationId)
  const base = `/${administrationId}`
  const ofType = (type: string) => workflows.data?.filter((w) => w.type === type) ?? []

  return (
    <Block
      id="workflows"
      title={t('invoicing.parts.workflows')}
      subtitle={t('invoicing.workflowsSubtitle')}
      action={
        <Menu label={t('settings.add')} buttonClassName="btn btn--accent" align="right">
          <ul className="menu__list">
            <li><Link to={`${base}/workflows/new/invoice_workflow`}>{t('invoicing.addInvoiceWorkflow')}</Link></li>
            <li><Link to={`${base}/workflows/new/estimate_workflow`}>{t('invoicing.addEstimateWorkflow')}</Link></li>
          </ul>
        </Menu>
      }
    >
      <WorkflowTable administrationId={administrationId} title={t('invoicing.invoiceWorkflows')} workflows={ofType('InvoiceWorkflow')} invoices />
      <WorkflowTable administrationId={administrationId} title={t('invoicing.estimateWorkflows')} workflows={ofType('EstimateWorkflow')} />
      <div className="table-card">
        <p className="table-card__title">{t('invoicing.purchaseWorkflows')}</p>
        <p className="empty table-card__empty">{t('invoicing.purchaseUnavailable')}</p>
      </div>
    </Block>
  )
}

type TableProps = { administrationId: string; title: string; workflows: Workflow[]; invoices?: boolean }

function WorkflowTable({ administrationId, title, workflows, invoices }: TableProps) {
  const { t } = useTranslation()
  const remove = useDeleteWorkflow(administrationId)
  return (
    <table className="data-table" aria-label={title}>
      <thead>
        <tr>
          <th scope="col">{title}</th>
          {invoices && <th scope="col" className="data-table__extra">{t('invoicing.directDebit')}</th>}
          {invoices && <th scope="col" className="data-table__extra">{t('invoicing.paymentMethods')}</th>}
          <th scope="col"><span className="sr-only">{t('settings.delete')}</span></th>
        </tr>
      </thead>
      <tbody>
        {workflows.map((w) => (
          <tr key={w.id}>
            <td>
              <Link to={`/${administrationId}/workflows/${w.id}`}>{w.name}</Link>
              {!w.active && <span className="hint"> · {t('invoicing.deactivated')}</span>}
            </td>
            {invoices && <td className="data-table__extra">{t('invoicing.inactive')}</td>}
            {invoices && <td className="data-table__extra">{w.payment_methods?.length ? w.payment_methods.join(', ') : '-'}</td>}
            <td className="data-table__actions">
              {w.default ? (
                <button type="button" className="btn btn--secondary btn--small" disabled aria-label={t('invoicing.defaultCannotBeDeleted')} title={t('invoicing.defaultCannotBeDeleted')}>
                  {t('settings.delete')}
                </button>
              ) : (
                <ConfirmDelete
                  label={t('invoicing.deleteName', { name: w.name })}
                  confirmLabel={t('invoicing.confirmDeleteName', { name: w.name })}
                  text={t('settings.delete')}
                  confirmText={t('settings.confirmDelete')}
                  disabled={remove.isPending}
                  onConfirm={() => remove.mutate(w.id ?? '')}
                />
              )}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
