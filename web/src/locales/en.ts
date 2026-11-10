export const en = {
  common: { language: 'Language', theme: 'Theme', system: 'System', light: 'Light', dark: 'Dark', cancel: 'Cancel', next: 'Next', skip: 'Skip to content', genericError: 'Something went wrong. Try again.', requestFailed: 'The request failed' },
  login: {
    title: 'Login', email: 'Email address', password: 'Password', submit: 'Login to MoneyHarpy', or: 'or', forgot: 'Forgot your password?', noAccount: 'No account?', passwordChanged: 'Password changed. Log in with your new password.', google: 'Continue with Google', lm2: 'Continue with LM2',
    unreachable: 'MoneyHarpy could not be reached. Reload the page to try again.',
    errors: {
      google: 'Google sign-in did not complete. Try again or use your password.',
      lm2: 'LM2 sign-in did not complete. Try again or use your password.',
      registration_closed: 'Registration is closed. Ask an administrator for access.',
      email_taken: 'This email already has an account. Sign in with your password.',
    },
  },
  register: { title: 'Create your MoneyHarpy account', name: 'Name', hint: 'Use at least 12 characters.', submit: 'Create account', already: 'Already have an account?' },
  passwords: {
    forgotTitle: 'Forgot your password?', forgotBody: 'Enter your email address and we will send you a link to choose a new password.', send: 'Send reset link', sent: 'If this address has an account, we sent a link to it. It works for one hour.', back: 'Back to login',
    editTitle: 'Choose a new password', new: 'New password', save: 'Save new password', badLink: 'This link is invalid or has expired', newLink: 'Request a new link',
  },
  nav: {
    main: 'Main', home: 'MoneyHarpy home', notifications: 'Notifications', noNotifications: 'You have no notifications.', select: 'Select administration', yours: 'Your administrations',
    all: 'All administrations', new: 'New administration', account: 'User account', instance: 'Instance settings', docs: 'API documentation', signOut: 'Sign out',
  },
  notFound: { title: 'Page not found', body: 'This page does not exist or you have no access to it.', back: 'Go to your administrations' },
  administrations: {
    title: 'Select an administration', add: 'Add administration', list: 'Administrations', loadError: 'Administrations could not be loaded. Reload the page to try again.',
    empty: 'Add your first administration to start bookkeeping.', favorite: 'Mark {{name}} as favorite', unfavorite: 'Remove {{name}} from favorites', settingsFor: 'Settings for {{name}}',
    homeEmpty: 'Invoices, contacts and quotes for this administration will appear here.', settings: '{{name}} settings', language: 'Language', currency: 'Currency', country: 'Country', timeZone: 'Time zone', starts: 'Bookkeeping starts',
  },
  onboarding: {
    welcome: 'Welcome {{name}}', subtitle: 'In a few steps, we will create your financial administration.',
    companyQuestion: 'What is the name of the company?', searchPlaceholder: 'Search...', searchHelp: 'We search the Dutch Chamber of Commerce register.',
    notRegistered: 'The company is not registered yet, or is not Dutch', noResults: 'No company found. Check the name or enter the details yourself.', searchFailed: 'The company register could not be reached. Enter the details yourself.', kvkNumber: 'KVK {{number}}',
    createTitle: 'Create company', createSubtitle: 'What are the name and address of your company?', companyName: 'Company name', country: 'Country', findCountry: 'Find country', zipcode: 'Zipcode', houseNumber: 'House number',
    address: 'Address', addDetails: 'Add address details', removeDetails: 'Remove address details', additional: 'Additional address details', city: 'City', coc: 'Chamber of Commerce number', currency: 'Currency', findCurrency: 'Find currency', noOption: 'Nothing found',
    unknownAddress: 'Combination of zipcode and house number is unknown',
    firstTitle: 'What do you want to start with?', firstSubtitle: 'Choose one of the options below to get started.', getStarted: 'Get started',
    invoice: 'I want to send an invoice', invoiceBody: 'Describe what you sold, set the amount and VAT rate, and email it to your customer.',
    purchase: 'I want to process a purchase', purchaseBody: 'Add the invoices and receipts of what your company bought.',
    bank: 'I want to connect my bank', bankBody: 'Bring in your transactions so they can be matched with your invoices.',
    open: 'I want to open my administration', openBody: 'Look around and set up your administration your own way.',
  },
  user: {
    title: 'User account', name: 'Name', email: 'Email', tokens: 'API tokens', tokensHint: 'Send a token as a bearer token to use the', api: 'API', copy: 'Copy this token now. It will not be shown again.', tokenName: 'Token name', create: 'Create token', list: 'API tokens', revoke: 'Revoke', revokeName: 'Revoke {{name}}',
  },
  instance: {
    title: 'Instance settings', register: 'Allow new users to register', googleTitle: 'Google login', googleEnable: 'Let users continue with Google', googleId: 'Google client ID', googleSecret: 'Google client secret', stored: 'Stored. Type to replace it.',
    lm2Title: 'LM2 login', lm2Enable: 'Let users continue with LM2', lm2Url: 'LM2 address', lm2App: 'App name in LM2',
    kvkTitle: 'Company search', kvkEnable: 'Search the Dutch company register', kvkKey: 'KVK API key', addressTitle: 'Address lookup', addressEnable: 'Complete Dutch addresses from zipcode and house number', addressHint: 'Sends the zipcode and house number to PDOK, the Dutch government address service.',
    smtpTitle: 'Email', smtpEnable: 'Send email through this mail server', smtpHint: 'Needed for password recovery.', smtpHost: 'SMTP host', smtpPort: 'SMTP port', smtpUser: 'SMTP username', smtpPassword: 'SMTP password', smtpFrom: 'Send email from', smtpSecurity: 'Encryption', smtpStarttls: 'STARTTLS (usually port 587)', smtpTls: 'TLS (usually port 465)', smtpNone: 'None', testEmail: 'Send test email', testSent: 'Test email sent to {{email}}',
    save: 'Save settings', saved: 'Settings saved',
  },
}

export type Messages = typeof en
