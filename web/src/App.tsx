import { Navigate, Outlet, Route, Routes } from 'react-router-dom'
import { AdministrationHomePage } from './administrations/AdministrationPages'
import { AdministrationsPage } from './administrations/AdministrationsPage'
import { LoginPage } from './auth/LoginPage'
import { EditPasswordPage, ForgotPasswordPage } from './auth/PasswordPages'
import { RegisterPage } from './auth/RegisterPage'
import { RequireAuth } from './auth/RequireAuth'
import { InstanceSettingsPage } from './instance/InstanceSettingsPage'
import { AppShell } from './layout/AppShell'
import { AdministrationLayout } from './layout/Sidenav'
import { NotFoundPage } from './layout/NotFoundPage'
import { CreateAdministrationPage } from './onboarding/CreateAdministrationPage'
import { FirstActionPage } from './onboarding/FirstActionPage'
import { ManualCompanyPage } from './onboarding/ManualCompanyPage'
import { PermalinkPage } from './resources/PermalinkPage'
import { DocumentStyleEditorPage, DocumentStyleEditRedirect } from './invoicing/DocumentStyleEditorPage'
import { DocumentStyleGeneralPage } from './invoicing/DocumentStyleGeneralPage'
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
import { BankConnectionCallback } from './banking/BankConnectionCallback'
import { FinancialAccountsPage } from './banking/FinancialAccountsPage'
import { PaymentsPage } from './banking/PaymentsPage'
import { BookingRuleFormPage } from './banking/BookingRuleFormPage'
import { FinancialAccountPage } from './banking/FinancialAccountPage'
import { TransactionPage } from './banking/TransactionPage'
import { TransactionsPage } from './banking/TransactionsPage'
import { AddCategoryPage, TemplatesRedirect } from './bookkeeping/AddCategoryPage'
import { BookkeepingSettingsPage } from './bookkeeping/BookkeepingSettingsPage'
import { LedgerAccountFormPage } from './bookkeeping/LedgerAccountFormPage'
import { SchemePage } from './bookkeeping/SchemePage'
import { SingleCategoryPage } from './bookkeeping/SingleCategoryPage'
import { TaxRateFormPage } from './bookkeeping/TaxRateFormPage'
import { ThemePage } from './bookkeeping/ThemePage'
import { SettingsLayout } from './settings/SettingsLayout'
import { UserPage } from './user/UserPage'
import { OverviewPage } from './overview/OverviewPage'
import { StartPage } from './overview/StartPage'
import { ContactFormPage } from './contacts/ContactFormPage'
import { ContactPage } from './contacts/ContactPage'
import { ContactsPage } from './contacts/ContactsPage'
import { ExternalEditPage } from './external/ExternalFormPage'
import { ExternalInvoicePage } from './external/ExternalInvoicePage'
import { ExternalInvoicesPage } from './external/ExternalInvoicesPage'
import { ArbitraryDepreciationPage, AssetActionsPage, DivestmentPage, OutOfServicePage } from './assets/AssetActionsPage'
import { EditAssetPage, NewAssetPage } from './assets/AssetFormPage'
import { AssetPage } from './assets/AssetPage'
import { AssetsPage } from './assets/AssetsPage'
import { DocumentPage } from './incoming/DocumentPage'
import { TypedDocumentRedirect, TypeListRedirect } from './incoming/DocumentRedirect'
import { DocumentsPage } from './incoming/DocumentsPage'
import { NewDocumentPage } from './incoming/NewDocumentPage'
import { InvoiceEditRedirect, InvoiceFormPage } from './invoices/InvoiceFormPage'
import { InvoicePage } from './invoices/InvoicePage'
import { ExternalRegisterPaymentPage, RegisterPaymentPage } from './invoices/RegisterPaymentPage'
import { InvoicesPage } from './invoices/InvoicesPage'
import { JournalDocumentPage } from './journal/JournalDocumentPage'
import { JournalDocumentsPage } from './journal/JournalDocumentsPage'
import { LedgerEntriesPage } from './journal/LedgerEntriesPage'
import {
  AgingReportPage, AssetsReportPage, BalanceSheetPage, CashFlowPage, ContactReportPage, ProfitLossPage, ProjectReportPage, TaxReportPage, TrialBalancePage,
} from './reports/ReportPages'
import { ReportsOverviewPage } from './reports/OverviewPage'
import { EventsPage } from './events/EventsPage'
import { AuditReportPage } from './reports/AuditPage'
import { EstimatePage } from './estimates/EstimatePage'
import { EstimatesPage } from './estimates/EstimatesPage'
import { RecurringInvoicePage } from './recurring/RecurringInvoicePage'
import { RecurringInvoicesPage } from './recurring/RecurringInvoicesPage'
import { OnlineSalesPage, PaymentLinkNotReadyPage } from './products/OnlineSalesPage'
import { ProductFormPage } from './products/ProductFormPage'
import { ProductPage } from './products/ProductPage'
import { ProductsPage } from './products/ProductsPage'
import { EditProjectPage, NewProjectPage, ProjectRedirect } from './projects/ProjectPage'
import { ProjectsPage } from './projects/ProjectsPage'
import { TaskListPage, TaskListRedirect } from './tasklists/TaskListPage'
import { TaskListsPage } from './tasklists/TaskListsPage'
import { EditTaskListPage, NewTaskListPage } from './tasklists/TaskListFormPage'
import { TemplateSelectionPage } from './tasklists/TemplateSelectionPage'
import { BillTimePage } from './timeentries/BillTimePage'
import { HoursReportPage } from './timeentries/HoursReportPage'
import { ProjectsReportPage } from './timeentries/ProjectsReportPage'
import { TimeEntriesPage } from './timeentries/TimeEntriesPage'
import { NewVatReturnPage } from './vat/NewVatReturnPage'
import { VatReturnPage } from './vat/VatReturnPage'
import { VatReturnsPage } from './vat/VatReturnsPage'
import { TimeEntryPage } from './timeentries/TimeEntryPage'
import { OnlineEstimatePage } from './online/OnlineEstimatePage'

export function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      <Route path="/login/two-factor" element={<TwoFactorSignInPage />} />
      <Route path="/invitations/:token" element={<InvitationPage />} />
      <Route path="/passwords/new" element={<ForgotPasswordPage />} />
      <Route path="/passwords/edit" element={<EditPasswordPage />} />
      {/* A quote its contact opens from the email, without an account. */}
      <Route path="/online/estimates/:estimateId" element={<OnlineEstimatePage />} />
      <Route element={<RequireAuth><Outlet /></RequireAuth>}>
        <Route path="/onboarding/company/create_administration" element={<CreateAdministrationPage />} />
        <Route path="/onboarding/company/manually" element={<ManualCompanyPage />} />
        <Route path="/:administrationId/onboarding/first_action/new" element={<FirstActionPage />} />
      </Route>
      <Route element={<RequireAuth><AppShell /></RequireAuth>}>
        <Route path="/" element={<StartPage />} />
        <Route path="/administrations" element={<AdministrationsPage />} />
        <Route path="/user" element={<UserPage />} />
        <Route path="/instance/settings" element={<InstanceSettingsPage />} />
        <Route path="/bank_connection_callback" element={<BankConnectionCallback />} />
        <Route path="/r/:id" element={<PermalinkPage />} />
        <Route path="/:administrationId" element={<AdministrationHomePage />} />
        <Route element={<AdministrationLayout />}>
          <Route path="/:administrationId/feed" element={<OverviewPage />} />
          <Route path="/:administrationId/financial_accounts" element={<FinancialAccountsPage />} />
          <Route path="/:administrationId/purchase_transaction_batches" element={<PaymentsPage />} />
          <Route path="/:administrationId/financial_mutations" element={<TransactionsPage />} />
          <Route path="/:administrationId/financial_mutations/:mutationId" element={<TransactionPage />} />
          <Route path="/:administrationId/contacts" element={<ContactsPage />} />
          <Route path="/:administrationId/contacts/new" element={<ContactFormPage />} />
          <Route path="/:administrationId/contacts/:contactId" element={<ContactPage />} />
          <Route path="/:administrationId/contacts/:contactId/edit" element={<ContactFormPage />} />
          <Route path="/:administrationId/sales_invoices" element={<InvoicesPage />} />
          <Route path="/:administrationId/sales_invoices/new" element={<InvoiceFormPage />} />
          <Route path="/:administrationId/sales_invoices/:invoiceId" element={<InvoicePage />} />
          <Route path="/:administrationId/sales_invoices/:invoiceId/edit" element={<InvoiceEditRedirect />} />
          <Route path="/:administrationId/sales_invoices/:invoiceId/payments/:way" element={<RegisterPaymentPage />} />
          <Route path="/:administrationId/external_sales_invoices" element={<ExternalInvoicesPage />} />
          <Route path="/:administrationId/external_sales_invoices/new" element={<ExternalEditPage />} />
          <Route path="/:administrationId/external_sales_invoices/:invoiceId" element={<ExternalInvoicePage />} />
          <Route path="/:administrationId/external_sales_invoices/:invoiceId/edit" element={<ExternalEditPage />} />
          <Route path="/:administrationId/external_sales_invoices/:invoiceId/payments/:way" element={<ExternalRegisterPaymentPage />} />
          <Route path="/:administrationId/recurring_sales_invoices" element={<RecurringInvoicesPage />} />
          <Route path="/:administrationId/recurring_sales_invoices/new" element={<RecurringInvoicePage />} />
          <Route path="/:administrationId/recurring_sales_invoices/:recurringId" element={<RecurringInvoicePage />} />
          <Route path="/:administrationId/estimates" element={<EstimatesPage />} />
          <Route path="/:administrationId/estimates/new" element={<EstimatePage />} />
          <Route path="/:administrationId/estimates/:estimateId" element={<EstimatePage />} />
          <Route path="/:administrationId/products" element={<ProductsPage />} />
          <Route path="/:administrationId/products/new" element={<ProductFormPage />} />
          <Route path="/:administrationId/products/:productId" element={<ProductPage />} />
          <Route path="/:administrationId/products/:productId/edit" element={<ProductFormPage />} />
          <Route path="/:administrationId/payment_links" element={<OnlineSalesPage />} />
          <Route path="/:administrationId/payment_links/new" element={<Navigate replace to="../incomplete" relative="path" />} />
          <Route path="/:administrationId/payment_links/incomplete" element={<PaymentLinkNotReadyPage />} />
          <Route path="/:administrationId/payment_links/:productId/edit" element={<PaymentLinkNotReadyPage />} />
          <Route path="/:administrationId/subscription_templates/new" element={<PaymentLinkNotReadyPage />} />
          <Route path="/:administrationId/assets" element={<AssetsPage />} />
          <Route path="/:administrationId/assets/new" element={<NewAssetPage />} />
          <Route path="/:administrationId/assets/:assetId" element={<AssetPage />} />
          <Route path="/:administrationId/assets/:assetId/edit" element={<EditAssetPage />} />
          <Route path="/:administrationId/assets/:assetId/value_changes/new" element={<AssetActionsPage />} />
          <Route path="/:administrationId/assets/:assetId/value_changes/arbitrary/new" element={<ArbitraryDepreciationPage />} />
          <Route path="/:administrationId/assets/:assetId/value_changes/full_depreciation/new" element={<OutOfServicePage />} />
          <Route path="/:administrationId/assets/:assetId/value_changes/divestment/new" element={<DivestmentPage />} />
          <Route path="/:administrationId/projects" element={<ProjectsPage />} />
          <Route path="/:administrationId/projects/new" element={<NewProjectPage />} />
          <Route path="/:administrationId/projects/:projectId" element={<ProjectRedirect />} />
          <Route path="/:administrationId/projects/:projectId/edit" element={<EditProjectPage />} />
          <Route path="/:administrationId/time_entries" element={<TimeEntriesPage />} />
          <Route path="/:administrationId/time_entries/new" element={<TimeEntryPage />} />
          <Route path="/:administrationId/time_entries/:timeEntryId" element={<TimeEntryPage />} />
          <Route path="/:administrationId/declarations" element={<VatReturnsPage />} />
          <Route path="/:administrationId/declarations/new" element={<NewVatReturnPage />} />
          <Route path="/:administrationId/declarations/:returnId" element={<VatReturnPage />} />
          <Route path="/:administrationId/time_entries_report" element={<HoursReportPage />} />
          <Route path="/:administrationId/time_entries_wizard" element={<BillTimePage />} />
          <Route path="/:administrationId/time_entries_report/projects" element={<ProjectsReportPage />} />
          <Route path="/:administrationId/task_lists/lists" element={<TaskListsPage />} />
          <Route path="/:administrationId/task_lists/lists/new" element={<NewTaskListPage />} />
          <Route path="/:administrationId/task_lists/lists/templates_selection" element={<TemplateSelectionPage />} />
          <Route path="/:administrationId/task_lists/lists/:taskListId" element={<TaskListPage />} />
          <Route path="/:administrationId/task_lists/lists/:taskListId/edit" element={<EditTaskListPage />} />
          <Route path="/:administrationId/task_lists/:taskListId" element={<TaskListRedirect />} />
          <Route path="/:administrationId/reports" element={<ReportsOverviewPage />} />
          <Route path="/:administrationId/events" element={<EventsPage />} />
          <Route path="/:administrationId/report/audit_report" element={<AuditReportPage />} />
          <Route path="/:administrationId/report/general_ledger" element={<TrialBalancePage />} />
          <Route path="/:administrationId/report/profit_loss" element={<ProfitLossPage />} />
          <Route path="/:administrationId/report/balance_sheet" element={<BalanceSheetPage />} />
          <Route path="/:administrationId/report/tax" element={<TaxReportPage />} />
          <Route path="/:administrationId/report/cash_flow" element={<CashFlowPage />} />
          <Route path="/:administrationId/report/debtors" element={<ContactReportPage key="debtors" kind="debtors" />} />
          <Route path="/:administrationId/report/creditors" element={<ContactReportPage key="creditors" kind="creditors" />} />
          <Route path="/:administrationId/report/revenue_by_contact" element={<ContactReportPage key="revenue" kind="revenue_by_contact" />} />
          <Route path="/:administrationId/report/expenses_by_contact" element={<ContactReportPage key="expenses" kind="expenses_by_contact" />} />
          <Route path="/:administrationId/report/revenue_by_contact/:contactId" element={<ContactReportPage key="revenue-one" kind="revenue_by_contact" />} />
          <Route path="/:administrationId/report/expenses_by_contact/:contactId" element={<ContactReportPage key="expenses-one" kind="expenses_by_contact" />} />
          <Route path="/:administrationId/report/revenue_by_project" element={<ProjectReportPage key="revenue" kind="revenue_by_project" />} />
          <Route path="/:administrationId/report/expenses_by_project" element={<ProjectReportPage key="expenses" kind="expenses_by_project" />} />
          <Route path="/:administrationId/report/assets_report" element={<AssetsReportPage />} />
          <Route path="/:administrationId/report/debtors_aging" element={<AgingReportPage key="debtors" kind="debtors_aging" />} />
          <Route path="/:administrationId/report/creditors_aging" element={<AgingReportPage key="creditors" kind="creditors_aging" />} />
          <Route path="/:administrationId/report/journal_entries" element={<LedgerEntriesPage />} />
          <Route path="/:administrationId/documents" element={<DocumentsPage />} />
          <Route path="/:administrationId/documents/new" element={<NewDocumentPage />} />
          <Route path="/:administrationId/documents/:documentId" element={<DocumentPage />} />
          <Route path="/:administrationId/documents/purchase_invoices" element={<TypeListRedirect type="purchase_invoice" />} />
          <Route path="/:administrationId/documents/purchase_invoices/:documentId" element={<TypedDocumentRedirect />} />
          <Route path="/:administrationId/documents/receipts" element={<TypeListRedirect type="receipt" />} />
          <Route path="/:administrationId/documents/receipts/:documentId" element={<TypedDocumentRedirect />} />
          <Route path="/:administrationId/documents/typeless_documents" element={<TypeListRedirect type={null} />} />
          <Route path="/:administrationId/documents/general_journal_documents" element={<JournalDocumentsPage />} />
          <Route path="/:administrationId/documents/general_journal_documents/new" element={<JournalDocumentPage />} />
          <Route path="/:administrationId/documents/general_journal_documents/:documentId" element={<JournalDocumentPage />} />
        </Route>
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
          <Route path="new" element={<DocumentStyleGeneralPage />} />
          <Route path=":styleId" element={<Navigate to="edit" replace />} />
          <Route path=":styleId/edit" element={<DocumentStyleEditRedirect />} />
          <Route path=":styleId/edit/:tab" element={<DocumentStyleEditorPage />} />
          <Route path=":styleId/edit/:tab/:language" element={<DocumentStyleEditorPage />} />
          <Route path=":styleId/edit_general" element={<DocumentStyleGeneralPage />} />
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
          <Route path="blueprints" element={<AddCategoryPage />} />
          <Route path="blueprint_ledger_accounts/new" element={<ThemePage />} />
          <Route path="new" element={<SingleCategoryPage />} />
          <Route path="scheme" element={<SchemePage />} />
          <Route path=":ledgerAccountId" element={<LedgerAccountFormPage />} />
        </Route>
        <Route path="/:administrationId/ledger_account_templates" element={<TemplatesRedirect />} />
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
