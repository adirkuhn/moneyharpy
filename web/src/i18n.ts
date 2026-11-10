import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import { en } from './locales/en'
import { nl, nlServer } from './locales/nl'

export type LanguageCode = 'en' | 'nl' | 'nl-be'

export const languages: { code: LanguageCode; label: string }[] = [
  { code: 'nl', label: 'NL' },
  { code: 'nl-be', label: 'BE' },
  { code: 'en', label: 'EN' },
]

const storageKey = 'moneyharpy.language'

function initialLanguage(): LanguageCode {
  try {
    const stored = localStorage.getItem(storageKey)
    if (languages.some((l) => l.code === stored)) {
      return stored as LanguageCode
    }
  } catch {
    // Without storage the browser's language decides.
  }
  const browser = navigator.language.toLowerCase()
  if (browser === 'nl-be') {
    return 'nl-be'
  }
  return browser.startsWith('nl') ? 'nl' : 'en'
}

const dutch = { translation: nl, server: nlServer }

void i18n.use(initReactI18next).init({
  resources: { en: { translation: en, server: {} }, nl: dutch, 'nl-be': dutch },
  lng: initialLanguage(),
  fallbackLng: 'en',
  lowerCaseLng: true,
  initAsync: false,
  interpolation: { escapeValue: false },
})

document.documentElement.lang = i18n.language
i18n.on('languageChanged', (language) => {
  document.documentElement.lang = language
})

export function currentLanguage(): LanguageCode {
  return i18n.language as LanguageCode
}

export function changeLanguage(code: LanguageCode) {
  void i18n.changeLanguage(code)
  try {
    localStorage.setItem(storageKey, code)
  } catch {
    // The language still applies until the page reloads.
  }
}

/** Translates a message sent by the server, keeping unknown ones as they are. */
export function serverMessage(message: string): string {
  return i18n.t(message, { ns: 'server', defaultValue: message, keySeparator: false, nsSeparator: false })
}

export default i18n
