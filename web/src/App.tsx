import { Navigate, Route, Routes } from 'react-router-dom'
import { AddAdministrationPage } from './administrations/AddAdministrationPage'
import { AdministrationHomePage, AdministrationSettingsPage } from './administrations/AdministrationPages'
import { AdministrationsPage } from './administrations/AdministrationsPage'
import { LoginPage } from './auth/LoginPage'
import { RegisterPage } from './auth/RegisterPage'
import { RequireAuth } from './auth/RequireAuth'
import { InstanceSettingsPage } from './instance/InstanceSettingsPage'
import { AppShell } from './layout/AppShell'
import { NotFoundPage } from './layout/NotFoundPage'
import { PermalinkPage } from './resources/PermalinkPage'
import { UserPage } from './user/UserPage'

export function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      <Route element={<RequireAuth><AppShell /></RequireAuth>}>
        <Route path="/" element={<Navigate to="/administrations" replace />} />
        <Route path="/administrations" element={<AdministrationsPage />} />
        <Route path="/administrations/new" element={<AddAdministrationPage />} />
        <Route path="/user" element={<UserPage />} />
        <Route path="/instance/settings" element={<InstanceSettingsPage />} />
        <Route path="/r/:id" element={<PermalinkPage />} />
        <Route path="/:administrationId" element={<AdministrationHomePage />} />
        <Route path="/:administrationId/settings" element={<AdministrationSettingsPage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  )
}
