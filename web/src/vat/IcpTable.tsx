import { useTranslation } from 'react-i18next'
import type { components } from '../api/schema'
import { useEuros } from './BoxTable'
import type { IcpKind } from './queries'

type Line = components['schemas']['VatReturnICPLine']

type Props = {
  lines: Line[]
  label: string
  /** While filing: the kind chosen per contact, and how to change it. */
  kinds?: Record<string, IcpKind>
  onKind?: (contactId: string, kind: IcpKind) => void
}

/** The supplies to businesses in other EU countries per customer, as the ICP declaration lists them. */
export function IcpTable({ lines, label, kinds, onKind }: Props) {
  const { t } = useTranslation()
  const euros = useEuros()
  return (
    <table className="data-table vat-icp" aria-label={label}>
      <thead>
        <tr>
          <th scope="col">{t('vat.icp.customer')}</th>
          <th scope="col">{t('vat.icp.country')}</th>
          <th scope="col">{t('vat.icp.vatNumber')}</th>
          <th scope="col">{t('vat.icp.kind')}</th>
          <th scope="col" className="vat-boxes__amount">{t('vat.icp.amount')}</th>
        </tr>
      </thead>
      <tbody>
        {lines.length === 0 && (
          <tr><td colSpan={5} className="vat-empty">{t('vat.icp.none')}</td></tr>
        )}
        {lines.map((l) => {
          const kind = kinds?.[l.contact_id] ?? l.kind
          return (
            <tr key={l.contact_id}>
              <td>{l.name}</td>
              <td>{l.country}</td>
              <td>{l.vat_number || <span className="vat-missing">{t('vat.icp.noVatNumber')}</span>}</td>
              <td>
                {onKind ? (
                  <select aria-label={t('vat.icp.kindOf', { name: l.name })} value={kind} onChange={(e) => onKind(l.contact_id, e.target.value as IcpKind)}>
                    <option value="services">{t('vat.icp.kinds.services')}</option>
                    <option value="goods">{t('vat.icp.kinds.goods')}</option>
                  </select>
                ) : t(`vat.icp.kinds.${kind}`)}
              </td>
              <td className="vat-boxes__amount">{euros(l.amount)}</td>
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}
