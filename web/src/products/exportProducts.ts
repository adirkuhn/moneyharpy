import type { TFunction } from 'i18next'
import { api, unwrap } from '../api/client'
import { type Product, productFrequency } from './queries'

/** The most the API gives in one page; the export asks page after page. */
const exportPage = 100

/** Names of what a product refers to, as the list shows them. */
type Names = { rate: (p: Product) => string; category: (p: Product) => string }

/** Every active product, page after page. */
async function allProducts(administrationId: string): Promise<Product[]> {
  const out: Product[] = []
  for (let page = 1; ; page++) {
    const list = await unwrap(api.GET('/api/v2/{administration_id}/products', {
      params: { path: { administration_id: administrationId }, query: { page: String(page), per_page: String(exportPage) } },
    }))
    out.push(...list)
    if (list.length < exportPage) {
      return out
    }
  }
}

/** The sheet of the products: a bold header, then a row per product with its amount as a number. */
export function productSheet(products: Product[], names: Names, t: TFunction) {
  const header = ['productTitle', 'reference', 'description', 'amount', 'currency', 'vatRate', 'category', 'frequency']
    .map((k) => ({ value: t(`products.${k}`), fontWeight: 'bold' as const }))
  return [
    header,
    ...products.map((p) => [
      { value: p.title ?? '' }, { value: p.identifier ?? '' }, { value: p.description ?? '' },
      { value: Number(p.price), type: Number, format: '#,##0.00' }, { value: p.currency ?? 'EUR' },
      { value: names.rate(p) }, { value: names.category(p) }, { value: productFrequency(p, t) },
    ]),
  ]
}

/** Downloads every product as an Excel file, as Moneybird's Export products. */
export async function exportProducts(administrationId: string, names: Names, t: TFunction, _language: string) {
  const products = await allProducts(administrationId)
  const { default: writeXlsxFile } = await import('write-excel-file/browser')
  await writeXlsxFile(productSheet(products, names, t), { sheet: t('products.title') }).toFile(`${t('products.title')}.xlsx`)
}
