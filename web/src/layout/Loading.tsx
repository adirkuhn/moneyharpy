import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

/** How long a load may take before saying so, to avoid a flash on fast loads. */
const delayMs = 300

/** Says the page is loading once a load takes longer than a moment. */
export function Loading() {
  const { t } = useTranslation()
  const [visible, setVisible] = useState(false)
  useEffect(() => {
    const timer = setTimeout(() => setVisible(true), delayMs)
    return () => clearTimeout(timer)
  }, [])
  return visible ? <p role="status" className="loading">{t('common.loading')}</p> : null
}
