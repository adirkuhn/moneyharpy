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
import { ManagementSettingsPage } from './settings/ManagementSettingsPage'
import { PlaceholderSettingsPage } from './settings/PlaceholderSettingsPage'
import { SettingsLayout } from './settings/SettingsLayout'
import { UserPage } from './user/UserPage'

export function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
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
          <Route path=":section" element={<PlaceholderSettingsPage />} />
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
