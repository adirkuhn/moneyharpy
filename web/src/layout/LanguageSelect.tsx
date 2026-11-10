import { useTranslation } from 'react-i18next'
import { changeLanguage, languages, type LanguageCode } from '../i18n'

export function LanguageSelect({ className }: { className?: string }) {
  const { t, i18n } = useTranslation()
  return (
    <label className={className}>
      <span>{t('common.language')}</span>
      <select value={i18n.language} onChange={(e) => changeLanguage(e.target.value as LanguageCode)}>
        {languages.map((l) => <option key={l.code} value={l.code}>{l.label}</option>)}
      </select>
    </label>
  )
}
