/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, within } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, test } from 'vitest'

import { api } from '@/lib/api'

import { UserInfoDialog } from '../user-info-dialog'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  initAsync: false,
})

type MockableApi = {
  get: (url: string) => Promise<{ data: unknown }>
}
const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get

const user = {
  id: 7,
  username: 'support-user',
  display_name: 'Support User',
  role: 1,
  status: 1,
  quota: 0,
  used_quota: 0,
  request_count: 0,
  group: 'default',
  phone: '+8613800138000',
  email: 'support@example.com',
}

function renderDialog() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={i18n}>
        <UserInfoDialog userId={7} open onOpenChange={() => {}} />
      </I18nextProvider>
    </QueryClientProvider>
  )
}

afterEach(() => {
  cleanup()
  apiClient.get = originalGet
})

function mockUser(overrides: Partial<typeof user> = {}) {
  apiClient.get = async (url) => {
    if (url === '/api/user/7') {
      return { data: { success: true, data: { ...user, ...overrides } } }
    }
    throw new Error(`unexpected request ${url}`)
  }
}

test('shows the bound phone number and email', async () => {
  mockUser()
  renderDialog()
  const dialog = await screen.findByRole('dialog', { name: 'User Information' })
  expect(within(dialog).getByText('Phone number')).toBeInTheDocument()
  expect(within(dialog).getByText('+8613800138000')).toBeInTheDocument()
  expect(within(dialog).getByText('Email')).toBeInTheDocument()
  expect(within(dialog).getByText('support@example.com')).toBeInTheDocument()
})

test('shows only the email when no phone is bound', async () => {
  mockUser({ phone: '' })
  renderDialog()
  const dialog = await screen.findByRole('dialog', { name: 'User Information' })
  expect(within(dialog).getByText('support@example.com')).toBeInTheDocument()
  expect(within(dialog).queryByText('Phone number')).not.toBeInTheDocument()
})

test('hides both rows when the account has no contact binding', async () => {
  mockUser({ phone: '', email: '' })
  renderDialog()
  const dialog = await screen.findByRole('dialog', { name: 'User Information' })
  await within(dialog).findByText('support-user')
  expect(within(dialog).queryByText('Phone number')).not.toBeInTheDocument()
  expect(within(dialog).queryByText('Email')).not.toBeInTheDocument()
})
