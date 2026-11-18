import { Fragment } from 'react'
import { useTranslation } from 'react-i18next'
import type { components } from '../api/schema'
import type { VatBox } from './queries'

type Figures = components['schemas']['VatReturnFigures']

/** The boxes of the Dutch return per section, and whether each has an amount charged on and VAT. */
export const sections: { section: string; boxes: { box: string; base: boolean; tax: boolean }[] }[] = [
  { section: '1', boxes: [{ box: '1a', base: true, tax: true }, { box: '1b', base: true, tax: true }, { box: '1c', base: true, tax: true }, { box: '1d', base: true, tax: true }, { box: '1e', base: true, tax: false }] },
  { section: '2', boxes: [{ box: '2a', base: true, tax: true }] },
  { section: '3', boxes: [{ box: '3a', base: true, tax: false }, { box: '3b', base: true, tax: false }, { box: '3c', base: true, tax: false }] },
  { section: '4', boxes: [{ box: '4a', base: true, tax: true }, { box: '4b', base: true, tax: true }] },
  { section: '5', boxes: [{ box: '5a', base: false, tax: true }, { box: '5b', base: false, tax: true }] },
]

const zero: VatBox = { box: '', base: '0', tax: '0', period_base: '0', period_tax: '0', correction_base: '0', correction_tax: '0' }

/** A box of the return, 5a worked out from the VAT of the others. */
export function boxOf(figures: Figures, code: string): VatBox {
  if (code !== '5a') {
    return figures.boxes.find((b) => b.box === code) ?? { ...zero, box: code }
  }
  const owed = figures.boxes.filter((b) => b.box !== '5b')
  const sum = (key: 'period_tax' | 'correction_tax') => String(owed.reduce((s, b) => s + Number(b[key]), 0))
  return { ...zero, box: '5a', tax: figures.owed, period_tax: sum('period_tax'), correction_tax: sum('correction_tax') }
}

/** Whole euros, as the return is filed: €1202. */
export function useEuros() {
  const { i18n } = useTranslation()
  return (v: string | number) => new Intl.NumberFormat(i18n.language, { style: 'currency', currency: 'EUR', maximumFractionDigits: 0, minimumFractionDigits: 0 }).format(Number(v))
}

type Props = {
  figures: Figures
  /** The period's name, for the rows that split a corrected box. */
  periodName: string
  label: string
}

/**
 * Moneybird's table of a return: its sections and boxes with what VAT is
 * charged on and the VAT, a corrected box split into the period and the
 * corrections, and the total to pay or receive.
 */
export function BoxTable({ figures, periodName, label }: Props) {
  const { t } = useTranslation()
  const euros = useEuros()
  const shown = (v: string, on: boolean) => (on ? euros(v) : '')
  return (
    <table className="data-table vat-boxes" aria-label={label}>
      <thead>
        <tr>
          <th scope="col" className="vat-boxes__code"><span className="sr-only">{t('vat.box')}</span></th>
          <th scope="col"><span className="sr-only">{t('vat.description')}</span></th>
          <th scope="col" className="vat-boxes__amount">{t('vat.base')}</th>
          <th scope="col" className="vat-boxes__amount">{t('vat.tax')}</th>
        </tr>
      </thead>
      {sections.map((s) => (
        <tbody key={s.section}>
          <tr className="vat-boxes__section">
            <th scope="rowgroup" className="vat-boxes__code">{s.section}.</th>
            <th scope="rowgroup" colSpan={3}>{t(`vat.sections.${s.section}`)}</th>
          </tr>
          {s.boxes.map((def) => {
            const b = boxOf(figures, def.box)
            const corrected = Number(b.correction_base) !== 0 || Number(b.correction_tax) !== 0
            return (
              <Fragment key={def.box}>
                <tr>
                  <td className="vat-boxes__code">{def.box}.</td>
                  <td>{t(`vat.boxes.${def.box}`)}</td>
                  <td className="vat-boxes__amount">{shown(b.base, def.base)}</td>
                  <td className="vat-boxes__amount">{shown(b.tax, def.tax)}</td>
                </tr>
                {corrected && (
                  <>
                    <tr className="vat-boxes__part">
                      <td />
                      <td>{periodName}</td>
                      <td className="vat-boxes__amount">{shown(b.period_base, def.base)}</td>
                      <td className="vat-boxes__amount">{shown(b.period_tax, def.tax)}</td>
                    </tr>
                    <tr className="vat-boxes__part">
                      <td />
                      <td>{t('vat.corrections')}</td>
                      <td className="vat-boxes__amount">{shown(b.correction_base, def.base)}</td>
                      <td className="vat-boxes__amount">{shown(b.correction_tax, def.tax)}</td>
                    </tr>
                  </>
                )}
              </Fragment>
            )
          })}
        </tbody>
      ))}
      <tfoot>
        <tr>
          <td />
          <th scope="row">
            {t('vat.total')}
            <span className="vat-boxes__hint">{t('vat.totalHint')}</span>
          </th>
          <td />
          <td className="vat-boxes__amount vat-boxes__total">{euros(figures.total)}</td>
        </tr>
      </tfoot>
    </table>
  )
}

/** The amounts to type over at the tax authority, box by box: Moneybird's copy table. */
export function CopyTable({ figures, label }: { figures: Figures; label: string }) {
  const { t } = useTranslation()
  const euros = useEuros()
  return (
    <table className="data-table vat-copy" aria-label={label}>
      <thead>
        <tr>
          <th scope="col">{t('vat.rubric')}</th>
          <th scope="col" className="vat-boxes__amount">{t('vat.revenue')}</th>
          <th scope="col" className="vat-boxes__amount">{t('vat.vat')}</th>
        </tr>
      </thead>
      <tbody>
        {sections.flatMap((s) => s.boxes).map((def) => {
          const b = boxOf(figures, def.box)
          return (
            <tr key={def.box}>
              <th scope="row">{def.box}.</th>
              <td className="vat-boxes__amount">{def.base ? euros(b.base) : '-'}</td>
              <td className="vat-boxes__amount">{def.tax ? euros(b.tax) : '-'}</td>
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}
