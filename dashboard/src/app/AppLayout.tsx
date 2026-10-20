import {
  ApiOutlined,
  AppstoreOutlined,
  AuditOutlined,
  ClusterOutlined,
  DashboardOutlined,
  HistoryOutlined,
  KeyOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  MoonOutlined,
  SafetyCertificateOutlined,
  SunOutlined,
  TeamOutlined,
  UserOutlined,
  GroupOutlined,
  DesktopOutlined,
} from '@ant-design/icons'
import { Breadcrumb, Button, Dropdown, Flex, Layout, Menu, Tooltip, Typography, theme } from 'antd'
import type { MenuProps } from 'antd'
import { useEffect, useState, type ReactNode } from 'react'
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../features/auth/AuthProvider'
import { PASSWORD_PATH } from '../features/auth/RequireAuth'
import { usePermissionSync } from '../features/auth/usePermissionSync'
import { usePermissions } from '../features/auth/usePermissions'
import { useTheme } from '../theme/ThemeProvider'
import type { ThemePreference } from '../theme/tokens'
import { breadcrumbsFor } from './breadcrumbs'
import { selectedNavKey, visibleNav } from './nav'
import type { NavEntry } from './nav'

const { Header, Sider, Content } = Layout

const SIDER_KEY = 'fluxor-sider-collapsed'

const icons: Record<string, ReactNode> = {
  '/': <DashboardOutlined />,
  '/clusters': <ClusterOutlined />,
  '/applications': <AppstoreOutlined />,
  '/events': <HistoryOutlined />,
  '/admin/users': <UserOutlined />,
  '/admin/teams': <TeamOutlined />,
  '/admin/groups': <GroupOutlined />,
  '/admin/access': <SafetyCertificateOutlined />,
  '/admin/audit': <AuditOutlined />,
}

const toItem = (e: NavEntry) => ({ key: e.key, icon: icons[e.key], label: e.label })

const themeIcon: Record<ThemePreference, ReactNode> = {
  system: <DesktopOutlined />,
  light: <SunOutlined />,
  dark: <MoonOutlined />,
}

const themeLabel: Record<ThemePreference, string> = {
  system: 'Sistem teması',
  light: 'Açık tema',
  dark: 'Koyu tema',
}

export function AppLayout() {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const { token } = theme.useToken()
  const { logout } = useAuth()
  const { user } = usePermissions()
  const { preference, cyclePreference } = useTheme()
  usePermissionSync()

  const [collapsed, setCollapsed] = useState(() => {
    try {
      return localStorage.getItem(SIDER_KEY) === '1'
    } catch {
      return false
    }
  })

  useEffect(() => {
    try {
      localStorage.setItem(SIDER_KEY, collapsed ? '1' : '0')
    } catch {
      /* ignore */
    }
  }, [collapsed])

  const nav = visibleNav(user?.permissions)
  const items: MenuProps['items'] = [
    ...nav.main.map(toItem),
    ...(nav.admin.length > 0
      ? [{ type: 'group' as const, key: 'admin', label: 'Yönetim', children: nav.admin.map(toItem) }]
      : []),
  ]

  const crumbs = breadcrumbsFor(pathname)

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider
        collapsible
        collapsed={collapsed}
        onCollapse={setCollapsed}
        trigger={null}
        width={232}
        collapsedWidth={72}
        style={{
          borderInlineEnd: `1px solid ${token.colorBorderSecondary}`,
          position: 'sticky',
          top: 0,
          height: '100vh',
          overflow: 'auto',
        }}
      >
        <Flex
          align="center"
          gap={10}
          style={{
            height: 56,
            paddingInline: collapsed ? 20 : 16,
            borderBottom: `1px solid ${token.colorBorderSecondary}`,
          }}
        >
          <Flex
            align="center"
            justify="center"
            style={{
              width: 28,
              height: 28,
              borderRadius: 6,
              background: token.colorPrimary,
              color: '#fff',
              fontWeight: 700,
              fontSize: 14,
              flexShrink: 0,
            }}
          >
            F
          </Flex>
          {!collapsed && (
            <Typography.Text strong style={{ fontSize: 16, letterSpacing: '-0.02em' }}>
              Fluxor
            </Typography.Text>
          )}
        </Flex>
        <Menu
          mode="inline"
          selectedKeys={[selectedNavKey(pathname)]}
          items={items}
          onClick={({ key }) => navigate(key)}
          style={{ borderInlineEnd: 0, paddingBlock: 8 }}
        />
      </Sider>
      <Layout>
        <Header
          style={{
            height: 56,
            lineHeight: '56px',
            paddingInline: 16,
            borderBottom: `1px solid ${token.colorBorderSecondary}`,
            position: 'sticky',
            top: 0,
            zIndex: 10,
            display: 'flex',
            alignItems: 'center',
          }}
        >
          <Flex justify="space-between" align="center" style={{ width: '100%' }} gap={12}>
            <Flex align="center" gap={4}>
              <Button
                type="text"
                icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
                onClick={() => setCollapsed((c) => !c)}
                aria-label={collapsed ? 'Menüyü aç' : 'Menüyü daralt'}
              />
              <Breadcrumb
                items={crumbs.map((c, i) => ({
                  title:
                    i < crumbs.length - 1 ? (
                      <Link to={c.path}>{c.label}</Link>
                    ) : (
                      c.label
                    ),
                }))}
              />
            </Flex>
            <Flex align="center" gap={4}>
              <Tooltip title={themeLabel[preference]}>
                <Button
                  type="text"
                  icon={themeIcon[preference]}
                  onClick={cyclePreference}
                  aria-label={themeLabel[preference]}
                />
              </Tooltip>
              <Dropdown
                trigger={['click']}
                menu={{
                  items: [
                    { key: 'tokens', icon: <ApiOutlined />, label: "API token'larım" },
                    { key: 'password', icon: <KeyOutlined />, label: 'Parolayı değiştir' },
                    { type: 'divider' },
                    { key: 'logout', icon: <LogoutOutlined />, label: 'Çıkış yap', danger: true },
                  ],
                  onClick: ({ key }) => {
                    if (key === 'password') navigate(PASSWORD_PATH)
                    if (key === 'tokens') navigate('/account/tokens')
                    if (key === 'logout') void logout().then(() => navigate('/login', { replace: true }))
                  },
                }}
              >
                <Button type="text" icon={<UserOutlined />}>
                  {user?.displayName || user?.username}
                </Button>
              </Dropdown>
            </Flex>
          </Flex>
        </Header>
        <Content style={{ padding: 24, maxWidth: 1400, width: '100%', marginInline: 'auto' }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  )
}
