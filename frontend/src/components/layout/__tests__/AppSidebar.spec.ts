import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')
const stylePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../style.css')
const styleSource = readFileSync(stylePath, 'utf8')

describe('AppSidebar custom SVG styles', () => {
  it('does not override uploaded SVG fill or stroke colors', () => {
    expect(componentSource).toContain('.sidebar-svg-icon {')
    expect(componentSource).toContain('color: currentColor;')
    expect(componentSource).toContain('display: block;')
    expect(componentSource).not.toContain('stroke: currentColor;')
    expect(componentSource).not.toContain('fill: none;')
  })
})

describe('AppSidebar header styles', () => {
  it('does not clip the version badge dropdown', () => {
    const sidebarHeaderBlockMatch = styleSource.match(/\.sidebar-header\s*\{[\s\S]*?\n {2}\}/)
    const sidebarBrandBlockMatch = componentSource.match(/\.sidebar-brand\s*\{[\s\S]*?\n\}/)

    expect(sidebarHeaderBlockMatch).not.toBeNull()
    expect(sidebarBrandBlockMatch).not.toBeNull()
    expect(sidebarHeaderBlockMatch?.[0]).not.toContain('@apply overflow-hidden;')
    expect(sidebarBrandBlockMatch?.[0]).not.toContain('overflow: hidden;')
  })

  it('links the logo and site name to the role-specific dashboard', () => {
    expect(componentSource).toContain("const homePath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))")
    expect(componentSource.match(/:to="homePath"/g)).toHaveLength(2)
    expect(componentSource).toContain(':tabindex="sidebarCollapsed ? -1 : undefined"')
  })
})

describe('AppSidebar admin personal menu', () => {
  it('shows the regular dashboard under My Account for admins', () => {
    const personalNavItemsBlockMatch = componentSource.match(
      /const personalNavItems = computed\(\(\): NavItem\[\] => \{[\s\S]*?const adminNavItems = computed/
    )

    expect(personalNavItemsBlockMatch).not.toBeNull()
    const personalNavItemsBlock = personalNavItemsBlockMatch?.[0] ?? ''
    const dashboardIndex = personalNavItemsBlock.indexOf("path: '/dashboard'")
    const modelsIndex = personalNavItemsBlock.indexOf("path: '/models'")

    expect(dashboardIndex).toBeGreaterThanOrEqual(0)
    expect(modelsIndex).toBeGreaterThanOrEqual(0)
    expect(dashboardIndex).toBeLessThan(modelsIndex)
  })
})
