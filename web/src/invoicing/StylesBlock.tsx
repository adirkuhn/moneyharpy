import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { ChevronIcon } from '../layout/icons'
import { Block } from '../settings/Block'
import { useIdentities } from '../settings/queries'
import { useDocumentStyles, useSaveDocumentStyle } from './queries'

/** The administration's document styles, the default marked. */
export function StylesBlock({ administrationId }: { administrationId: string }) {
  const { t } = useTranslation()
  const styles = useDocumentStyles(administrationId)
  const identities = useIdentities(administrationId)
  const base = `/${administrationId}`
  const senderName = (id: string) => identities.data?.find((i) => i.id === id)?.company_name ?? ''

  return (
    <Block
      id="document-styles"
      title={t('invoicing.parts.styles')}
      subtitle={t('invoicing.stylesSubtitle')}
      action={<Link className="btn btn--accent" to={`${base}/document_styles/new`}>{t('settings.add')}</Link>}
    >
      <ul className="ledger" aria-label={t('invoicing.parts.styles')}>
        {styles.data?.map((style) => (
          <li className="ledger__row" key={style.id}>
            <span className="ledger__text">
              <Link className="ledger__name" to={`${base}/document_styles/${style.id}/edit`}>{style.name}</Link>
              <span className="hint">{t('invoicing.senderLine', { name: senderName(style.identity_id ?? '') })}</span>
            </span>
            {style.default ? <span className="ledger__state">{t('invoicing.defaultStyle')}</span> : <MakeDefault administrationId={administrationId} id={style.id ?? ''} />}
            <ChevronIcon className="ledger__chevron" />
          </li>
        ))}
      </ul>
    </Block>
  )
}

function MakeDefault({ administrationId, id }: { administrationId: string; id: string }) {
  const { t } = useTranslation()
  const save = useSaveDocumentStyle(administrationId, id)
  return (
    <button type="button" className="btn btn--secondary btn--small" disabled={save.isPending} onClick={() => save.mutate({ default: true })}>
      {t('invoicing.makeDefault')}
    </button>
  )
}
