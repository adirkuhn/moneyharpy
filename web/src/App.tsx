import { Navigate, Outlet, Route, Routes } from 'react-router-dom'
import { AdministrationHomePage } from './administrations/AdministrationPages'
import { AdministrationsPage } from './administrations/AdministrationsPage'
import { LoginPage } from './auth/LoginPage'
import { EditPasswordPage, ForgotPasswordPage } from './auth/PasswordPages'
import { RegisterPage } from './auth/RegisterPage'
import { RequireAuth } from './auth/RequireAuth'
import { InstanceSettingsPage } from './instance/InstanceSettingsPage'
import { AppShell } from './layout/AppShell'
import { NotFoundPage } from './layout/NotFoundPage'
import { CreateAdministrationPage } from './onboarding/CreateAdministrationPage'
import { FirstActionPage } from './onboarding/FirstActionPage'
import { ManualCompanyPage } from './onboarding/ManualCompanyPage'
import { PermalinkPage } from './resources/PermalinkPage'
import { DocumentStyleFormPage } from './invoicing/DocumentStyleFormPage'
import { InvoicesSettingsPage } from './invoicing/InvoicesSettingsPage'
import { WorkflowFormPage } from './invoicing/WorkflowFormPage'
import { CompanySettingsPage } from './settings/CompanySettingsPage'
import { CustomFieldFormPage } from './settings/CustomFieldFormPage'
import { IdentityFormPage } from './settings/IdentityFormPage'
import { DeletePage } from './management/DeletePage'
import { InvitePage } from './management/InvitePage'
import { ManagementSettingsPage } from './management/ManagementSettingsPage'
import { PrivacyPage } from './management/PrivacyPage'
import { SupportAccessPage } from './management/SupportAccessPage'
import { UserFormPage } from './management/UserFormPage'
import { InvitationPage } from './auth/InvitationPage'
import { ConnectionFormPage } from './integrations/ConnectionFormPage'
import { IntegrationsPage } from './integrations/IntegrationsPage'
import { WebhookFormPage, WebhookPage } from './integrations/WebhookPages'
import { TwoFactorSignInPage } from './auth/TwoFactorSignInPage'
import { BankSettingsPage } from './banking/BankSettingsPage'
import { BookingRuleFormPage } from './banking/BookingRuleFormPage'
import { FinancialAccountPage } from './banking/FinancialAccountPage'
import { TransactionPage } from './banking/TransactionPage'
import { TransactionsPage } from './banking/TransactionsPage'
import { BookkeepingSettingsPage } from './bookkeeping/BookkeepingSettingsPage'
import { LedgerAccountFormPage } from './bookkeeping/LedgerAccountFormPage'
import { SchemePage } from './bookkeeping/SchemePage'
import { TaxRateFormPage } from './bookkeeping/TaxRateFormPage'
import { TemplatesPage } from './bookkeeping/TemplatesPage'
import { SettingsLayout } from './settings/SettingsLayout'
import { UserPage } from './user/UserPage'

export function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      <Route path="/login/two-factor" element={<TwoFactorSignInPage />} />
      <Route path="/invitations/:token" element={<InvitationPage />} />
      <Route path="/passwords/new" element={<ForgotPasswordPage />} />
      <Route path="/passwords/edit" element={<EditPasswordPage />} />
      <Route element={<RequireAuth><Outlet /></RequireAuth>}>
        <Route path="/onboarding/company/create_administration" element={<CreateAdministrationPage />} />
        <Route path="/onboarding/company/manually" element={<ManualCompanyPage />} />
        <Route path="/:administrationId/onboarding/first_action/new" element={<FirstActionPage />} />
      </Route>
      <Route element={<RequireAuth><AppShell /></RequireAuth>}>
        <Route path="/" element={<Navigate to="/administrations" replace />} />
        <Route path="/administrations" element={<AdministrationsPage />} />
        <Route path="/user" element={<UserPage />} />
        <Route path="/instance/settings" element={<InstanceSettingsPage />} />
        <Route path="/r/:id" element={<PermalinkPage />} />
        <Route path="/:administrationId" element={<AdministrationHomePage />} />
        <Route path="/:administrationId/settings" element={<SettingsLayout />}>
          <Route index element={<Navigate to="company" replace />} />
          <Route path="company" element={<CompanySettingsPage />} />
          <Route path="administration" element={<ManagementSettingsPage />} />
          <Route path="invoices_and_estimates" element={<InvoicesSettingsPage />} />
          <Route path="bookkeeping" element={<BookkeepingSettingsPage />} />
          <Route path="bank" element={<BankSettingsPage />} />
          <Route path="connections" element={<IntegrationsPage />} />
          <Route path="*" element={<NotFoundPage />} />
        </Route>
        <Route path="/:administrationId/custom_fields" element={<SettingsLayout />}>
          <Route path="new" element={<CustomFieldFormPage />} />
          <Route path=":fieldId/edit" element={<CustomFieldFormPage />} />
        </Route>
        <Route path="/:administrationId/workflows" element={<SettingsLayout />}>
          <Route path="new/:kind" element={<WorkflowFormPage />} />
          <Route path=":workflowId" element={<WorkflowFormPage />} />
        </Route>
        <Route path="/:administrationId/document_styles" element={<SettingsLayout />}>
          <Route path="new" element={<DocumentStyleFormPage />} />
          <Route path=":styleId" element={<Navigate to="edit" replace />} />
          <Route path=":styleId/edit" element={<DocumentStyleFormPage />} />
        </Route>
        <Route path="/:administrationId/connections" element={<SettingsLayout />}>
          <Route path="new" element={<ConnectionFormPage />} />
        </Route>
        <Route path="/:administrationId/webhooks" element={<SettingsLayout />}>
          <Route path="new" element={<WebhookFormPage />} />
          <Route path=":webhookId" element={<WebhookPage />} />
        </Route>
        <Route path="/:administrationId/users" element={<SettingsLayout />}>
          <Route path=":userId" element={<UserFormPage />} />
        </Route>
        <Route path="/:administrationId/user_invitations" element={<SettingsLayout />}>
          <Route path="new" element={<InvitePage />} />
        </Route>
        <Route path="/:administrationId/support_access" element={<SettingsLayout />}>
          <Route index element={<SupportAccessPage />} />
        </Route>
        <Route path="/:administrationId/privacy" element={<SettingsLayout />}>
          <Route path=":topic" element={<PrivacyPage />} />
        </Route>
        <Route path="/:administrationId/delete" element={<SettingsLayout />}>
          <Route index element={<DeletePage />} />
        </Route>
        <Route path="/:administrationId/financial_mutations" element={<TransactionsPage />} />
        <Route path="/:administrationId/financial_mutations/:mutationId" element={<TransactionPage />} />
        <Route path="/:administrationId/financial_accounts" element={<SettingsLayout />}>
          <Route path="new" element={<FinancialAccountPage />} />
          <Route path=":accountId" element={<FinancialAccountPage />} />
        </Route>
        <Route path="/:administrationId/booking_rules" element={<SettingsLayout />}>
          <Route path="new" element={<BookingRuleFormPage />} />
          <Route path=":ruleId" element={<BookingRuleFormPage />} />
        </Route>
        <Route path="/:administrationId/tax_rates" element={<SettingsLayout />}>
          <Route path="new" element={<TaxRateFormPage />} />
          <Route path=":taxRateId" element={<TaxRateFormPage />} />
        </Route>
        <Route path="/:administrationId/ledger_accounts" element={<SettingsLayout />}>
          <Route path="new" element={<LedgerAccountFormPage />} />
          <Route path="scheme" element={<SchemePage />} />
          <Route path=":ledgerAccountId" element={<LedgerAccountFormPage />} />
        </Route>
        <Route path="/:administrationId/ledger_account_templates" element={<SettingsLayout />}>
          <Route index element={<TemplatesPage />} />
        </Route>
        <Route path="/:administrationId/identities" element={<SettingsLayout />}>
          <Route path="new" element={<IdentityFormPage mode="new" />} />
          <Route path="edit_default" element={<IdentityFormPage mode="default" />} />
          <Route path=":identityId" element={<Navigate to="edit" replace />} />
          <Route path=":identityId/edit" element={<IdentityFormPage mode="edit" />} />
        </Route>
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  )
}
