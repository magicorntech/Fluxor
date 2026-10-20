import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Flex, Spin } from 'antd'
import { lazy, Suspense, type ReactNode } from 'react'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { AppLayout } from './app/AppLayout'
import { AuthProvider } from './features/auth/AuthProvider'
import { RequirePermission } from './features/auth/RequirePermission'
import { PASSWORD_PATH, RequireAuth } from './features/auth/RequireAuth'
import { ThemeProvider } from './theme/ThemeProvider'

const LoginPage = lazy(() => import('./features/auth/LoginPage').then((m) => ({ default: m.LoginPage })))
const ChangePasswordPage = lazy(() =>
  import('./features/auth/ChangePasswordPage').then((m) => ({ default: m.ChangePasswordPage })),
)
const OverviewPage = lazy(() => import('./features/overview/OverviewPage').then((m) => ({ default: m.OverviewPage })))
const ClustersPage = lazy(() => import('./features/clusters/ClustersPage').then((m) => ({ default: m.ClustersPage })))
const ApplicationsPage = lazy(() =>
  import('./features/applications/ApplicationsPage').then((m) => ({ default: m.ApplicationsPage })),
)
const EventsPage = lazy(() => import('./features/events/EventsPage').then((m) => ({ default: m.EventsPage })))
const UsersPage = lazy(() => import('./features/admin/UsersPage').then((m) => ({ default: m.UsersPage })))
const TeamsPage = lazy(() => import('./features/admin/TeamsPage').then((m) => ({ default: m.TeamsPage })))
const GroupsPage = lazy(() => import('./features/admin/GroupsPage').then((m) => ({ default: m.GroupsPage })))
const AccessPage = lazy(() => import('./features/admin/AccessPage').then((m) => ({ default: m.AccessPage })))
const AuditPage = lazy(() => import('./features/admin/AuditPage').then((m) => ({ default: m.AuditPage })))
const TokensPage = lazy(() => import('./features/account/TokensPage').then((m) => ({ default: m.TokensPage })))

const queryClient = new QueryClient({
  defaultOptions: { queries: { refetchOnWindowFocus: false } },
})

function PageFallback() {
  return (
    <Flex justify="center" align="center" style={{ minHeight: 240 }}>
      <Spin size="large" />
    </Flex>
  )
}

function Lazy({ children }: { children: ReactNode }) {
  return <Suspense fallback={<PageFallback />}>{children}</Suspense>
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <BrowserRouter>
          <AuthProvider>
            <Routes>
              <Route
                path="/login"
                element={
                  <Lazy>
                    <LoginPage />
                  </Lazy>
                }
              />
              <Route element={<RequireAuth />}>
                <Route element={<AppLayout />}>
                  <Route
                    index
                    element={
                      <Lazy>
                        <OverviewPage />
                      </Lazy>
                    }
                  />
                  <Route
                    path="clusters"
                    element={
                      <Lazy>
                        <ClustersPage />
                      </Lazy>
                    }
                  />
                  <Route
                    path="applications"
                    element={
                      <Lazy>
                        <ApplicationsPage />
                      </Lazy>
                    }
                  />
                  <Route
                    path="events"
                    element={
                      <Lazy>
                        <EventsPage />
                      </Lazy>
                    }
                  />
                  <Route
                    path="admin/users"
                    element={
                      <Lazy>
                        <RequirePermission permission="user:manage">
                          <UsersPage />
                        </RequirePermission>
                      </Lazy>
                    }
                  />
                  <Route
                    path="admin/teams"
                    element={
                      <Lazy>
                        <RequirePermission permission="team:manage">
                          <TeamsPage />
                        </RequirePermission>
                      </Lazy>
                    }
                  />
                  <Route
                    path="admin/groups"
                    element={
                      <Lazy>
                        <RequirePermission permission="group:manage">
                          <GroupsPage />
                        </RequirePermission>
                      </Lazy>
                    }
                  />
                  <Route
                    path="admin/access"
                    element={
                      <Lazy>
                        <RequirePermission permission="rbac:manage">
                          <AccessPage />
                        </RequirePermission>
                      </Lazy>
                    }
                  />
                  <Route
                    path="admin/audit"
                    element={
                      <Lazy>
                        <RequirePermission permission="audit:view">
                          <AuditPage />
                        </RequirePermission>
                      </Lazy>
                    }
                  />
                  <Route
                    path="account/tokens"
                    element={
                      <Lazy>
                        <TokensPage />
                      </Lazy>
                    }
                  />
                  <Route
                    path={PASSWORD_PATH.slice(1)}
                    element={
                      <Lazy>
                        <ChangePasswordPage />
                      </Lazy>
                    }
                  />
                </Route>
              </Route>
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </AuthProvider>
        </BrowserRouter>
      </ThemeProvider>
    </QueryClientProvider>
  )
}
