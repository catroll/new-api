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

import { UserAuthForm } from '../sign-in/components/user-auth-form'

const { handleLoginResult } = vi.hoisted(() => ({
  handleLoginResult: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    to,
    ...rest
  }: {
    children?: React.ReactNode
    to?: string
  }) => (
    <a href={typeof to === 'string' ? to : '#'} {...rest}>
      {children}
    </a>
  ),
}))

vi.mock('@/features/auth/hooks/use-auth-redirect', () => ({
  useAuthRedirect: () => ({ handleLoginResult, redirectToLogin: vi.fn() }),
}))

let client: QueryClient

beforeEach(() => {
  handleLoginResult.mockReset()
  handleLoginResult.mockResolvedValue(true)
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(STATUS_QUERY_KEY, {
    password_login_enabled: true,
    password_login_encryption_enabled: false,
    sms_login_enabled: true,
    sms_default_country_code: '+86',
    turnstile_check: false,
    register_enabled: true,
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
})

it('signs in with a phone number and a code', async () => {
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      message: '',
      data: {
        access_token: 'token',
        user: { id: 1, username: 'user', role: 1 },
      },
    },
  })
  const user = userEvent.setup()

  render(
    <QueryClientProvider client={client}>
      <UserAuthForm redirectTo='/' />
    </QueryClientProvider>
  )

  await user.click(await screen.findByRole('button', { name: 'Code sign in' }))
  await user.type(screen.getByLabelText('Phone number'), '13800138000')
  await user.type(screen.getByLabelText('Verification code'), '123456')
  await user.click(screen.getByRole('button', { name: 'Sign in' }))

  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/user/login/sms',
      { phone: '13800138000', code: '123456' },
      expect.objectContaining({
        params: expect.objectContaining({ turnstile: '' }),
        skipAuthRefresh: true,
      })
    )
  )
  await waitFor(() => expect(handleLoginResult).toHaveBeenCalled())
})
