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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { STATUS_QUERY_KEY } from '@/lib/status-query'
import { useAuthStore } from '@/stores/auth-store'

import { PhoneReminder } from '../components/phone-reminder'

let client: QueryClient

beforeEach(() => {
  sessionStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, username: 'user', role: 1 })
})

afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.getState().auth.reset('idle')
})

function renderReminder() {
  return render(
    <QueryClientProvider client={client}>
      <PhoneReminder />
    </QueryClientProvider>
  )
}

it('reminds an account without a phone and stays quiet after dismissal', async () => {
  client.setQueryData(STATUS_QUERY_KEY, { sms_require_phone: true })
  const user = userEvent.setup()
  const { unmount } = renderReminder()

  expect(await screen.findByText('Bind phone number')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() =>
    expect(screen.queryByText('Bind phone number')).not.toBeInTheDocument()
  )
  expect(sessionStorage.getItem('sms-phone-reminder-shown')).toBe('1')

  // Reloading the same tab must not nag again.
  unmount()
  renderReminder()
  await new Promise((resolve) => setTimeout(resolve, 50))
  expect(screen.queryByText('Bind phone number')).not.toBeInTheDocument()
})

it('does not remind when the site policy is off', async () => {
  client.setQueryData(STATUS_QUERY_KEY, { sms_require_phone: false })
  renderReminder()
  await new Promise((resolve) => setTimeout(resolve, 50))
  expect(screen.queryByText('Bind phone number')).not.toBeInTheDocument()
})

it('does not remind an account that already has a phone', async () => {
  client.setQueryData(STATUS_QUERY_KEY, { sms_require_phone: true })
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'user', role: 1, has_phone: true })
  renderReminder()
  await new Promise((resolve) => setTimeout(resolve, 50))
  expect(screen.queryByText('Bind phone number')).not.toBeInTheDocument()
})

afterEach(() => {
  vi.restoreAllMocks()
})
