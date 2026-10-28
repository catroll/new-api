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

For commercial licensing, please contact support@quantumnous.com
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { PricingSection } from '../pricing-section'

function Fixture(props: { displayInCurrency?: boolean }) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setContainer} />
      <SettingsPageProvider actionsContainer={container}>
        <PricingSection
          defaultValues={{
            QuotaPerUnit: 500000,
            USDExchangeRate: 7.3,
            PaymentCurrency: 'CNY',
            PriceFollowsExchangeRate: false,
            ExchangeRateRefreshMinutes: 0,
            ExchangeRateApiUrl: 'https://open.er-api.com/v6/latest/USD',
            ExchangeRateUpdatedAt: 0,
            DisplayInCurrencyEnabled: props.displayInCurrency ?? true,
            DisplayTokenStatEnabled: true,
            general_setting: {
              quota_display_type: 'CNY',
              custom_currency_symbol: '¤',
              custom_currency_exchange_rate: 1,
            },
          }}
        />
      </SettingsPageProvider>
    </>
  )
}

async function renderSettings(displayInCurrency = true) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => <Fixture displayInCurrency={displayInCurrency} />,
    }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return screen.findByRole('spinbutton', { name: 'CNY per USD' })
}

beforeEach(() => {
  vi.restoreAllMocks()
})

// SettingsSwitchContent renders its children and nothing else: a switch given
// title/description props instead of children renders with no text at all, which
// is how "Price follows the exchange rate" shipped unlabelled.
test('the price-following switch carries a label and an explanation', async () => {
  await renderSettings()

  const toggle = screen.getByRole('switch', {
    name: 'Price follows the exchange rate',
  })
  expect(toggle).not.toBeChecked()
  expect(
    screen.getByText(/Overwrite Price with each refresh/)
  ).toBeInTheDocument()
})

// Hidden while true, this toggle could be switched on and never off again —
// this page is the only place that edits it.
test('the currency-display switch stays reachable once enabled', async () => {
  await renderSettings(true)
  expect(
    screen.getByRole('switch', { name: 'Display in Currency' })
  ).toBeInTheDocument()
})

test('refreshing the rate updates the field it just changed', async () => {
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: { rate: 6.681, price: 6.681, updated_at: 1793000000 },
    },
  })
  const user = userEvent.setup()
  const input = await renderSettings()
  expect(input).toHaveValue(7.3)
  expect(screen.getByText('Never updated')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Refresh now' }))

  await waitFor(() => expect(input).toHaveValue(6.681))
  expect(api.post).toHaveBeenCalledWith('/api/option/exchange_rate/refresh')
  // The button updates a value it does not own the way a save does, so the page
  // has to say how fresh that value is.
  expect(screen.getByText(/Last updated:/)).toBeInTheDocument()
})
