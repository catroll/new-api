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
import { api } from '@/lib/api'

import { SmsResetForm } from '../forgot-password/components/sms-reset-form'

let client: QueryClient

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(STATUS_QUERY_KEY, { turnstile_check: false })
})

afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
})

it('sends a reset code and shows the generated password', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, message: '', data: null },
  })
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, message: '', data: 'new-pass-123' },
  })
  const user = userEvent.setup()

  render(
    <QueryClientProvider client={client}>
      <SmsResetForm countryCode='+86' />
    </QueryClientProvider>
  )

  await user.type(screen.getByLabelText('Phone number'), '13800138000')
  await user.click(screen.getByRole('button', { name: 'Send code' }))

  await waitFor(() =>
    expect(get).toHaveBeenCalledWith(
      '/api/verification/sms',
      expect.objectContaining({
        params: expect.objectContaining({
          phone: '13800138000',
          purpose: 'reset',
        }),
      })
    )
  )

  await user.type(screen.getByLabelText('Verification code'), '123456')
  await user.click(screen.getByRole('button', { name: 'Reset password' }))

  expect(await screen.findByDisplayValue('new-pass-123')).toBeInTheDocument()
  expect(post).toHaveBeenCalledWith('/api/user/reset/sms', {
    phone: '13800138000',
    code: '123456',
  })
})
