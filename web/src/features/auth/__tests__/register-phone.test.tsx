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

import { api } from '@/lib/api'
import { STATUS_QUERY_KEY } from '@/lib/status-query'

import { SignUpForm } from '../sign-up/components/sign-up-form'

const { redirectToLogin } = vi.hoisted(() => ({ redirectToLogin: vi.fn() }))

vi.mock('@/features/auth/hooks/use-auth-redirect', () => ({
  useAuthRedirect: () => ({ handleLoginResult: vi.fn(), redirectToLogin }),
}))

let client: QueryClient

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(STATUS_QUERY_KEY, {
    register_enabled: true,
    password_register_enabled: true,
    email_verification: false,
    sms_require_phone: true,
    sms_default_country_code: '+86',
    turnstile_check: false,
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  redirectToLogin.mockReset()
  vi.restoreAllMocks()
})

it('requires and sends a phone number when the site policy is on', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, message: '', data: null },
  })
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, message: '', data: null },
  })
  const user = userEvent.setup()

  render(
    <QueryClientProvider client={client}>
      <SignUpForm />
    </QueryClientProvider>
  )

  await user.type(screen.getByLabelText('Username'), 'new-user')
  await user.type(
    screen.getByLabelText('Password', { selector: 'input' }),
    'Test-pass-123'
  )
  await user.type(screen.getByLabelText('Confirm password'), 'Test-pass-123')
  await user.type(screen.getByLabelText('Phone number'), '13800138000')

  await user.click(screen.getByRole('button', { name: 'Send code' }))
  await waitFor(() =>
    expect(get).toHaveBeenCalledWith(
      '/api/verification/sms',
      expect.objectContaining({
        params: expect.objectContaining({
          phone: '13800138000',
          purpose: 'register',
        }),
      })
    )
  )

  await user.type(screen.getByLabelText('Verification code'), '654321')
  await user.click(screen.getByRole('button', { name: 'Create account' }))

  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/user/register',
      expect.objectContaining({
        username: 'new-user',
        phone: '13800138000',
        phone_code: '654321',
      }),
      expect.anything()
    )
  )
  await waitFor(() => expect(redirectToLogin).toHaveBeenCalled())
})
