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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { PaymentSettingsSection } from '../payment-settings-section'

const waffoDefaults = {
  WaffoEnabled: false,
  WaffoApiKey: '',
  WaffoPrivateKey: '',
  WaffoPublicCert: '',
  WaffoSandboxPublicCert: '',
  WaffoSandboxApiKey: '',
  WaffoSandboxPrivateKey: '',
  WaffoSandbox: false,
  WaffoMerchantId: '',
  WaffoCurrency: 'USD',
  WaffoUnitPrice: 1,
  WaffoMinTopUp: 1,
  WaffoNotifyUrl: '',
  WaffoReturnUrl: '',
  WaffoPayMethods: '[]',
}

function Fixture(props: { follows?: boolean }) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setContainer} />
      <SettingsPageProvider actionsContainer={container}>
        <PaymentSettingsSection
          defaultValues={{
            PayAddress: '',
            EpayId: '',
            EpayKey: '',
            Price: 7.3,
            PriceFollowsExchangeRate: props.follows ?? false,
            MinTopUp: 1,
            CustomCallbackAddress: 'https://api.example.com',
            PayMethods: '[]',
            AmountOptions: '[]',
            AmountDiscount: '{}',
            StripeApiSecret: '',
            StripeWebhookSecret: '',
            StripePriceId: '',
            StripeUnitPrice: 1,
            StripeMinTopUp: 1,
            StripePromotionCodesEnabled: false,
            CreemApiKey: '',
            CreemWebhookSecret: '',
            CreemTestMode: false,
            CreemProducts: '[]',
            AlipayAppId: '',
            AlipayPrivateKey: '',
            AlipayPublicKey: '',
            AlipaySandbox: false,
            AlipayMinTopUp: 1,
          }}
          waffoDefaultValues={waffoDefaults}
          waffoPancakeDefaultValues={{
            WaffoPancakeMerchantID: '',
            WaffoPancakePrivateKey: '',
            WaffoPancakeReturnURL: '',
          }}
          complianceDefaults={{
            confirmed: true,
            termsVersion: 'v1',
            confirmedAt: 0,
            confirmedBy: 0,
          }}
        />
      </SettingsPageProvider>
    </>
  )
}

function renderSection(follows = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <Fixture follows={follows} />
    </QueryClientProvider>
  )
  return screen.findByRole('spinbutton', { name: /Price/ })
}

beforeEach(() => {
  vi.restoreAllMocks()
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
})

// The switch used to live on the currency page while the value it overwrites
// lived here: an administrator editing the price had no way to see that a job
// would replace it on the next refresh.
test('the sync switch sits beside the price and owns the field', async () => {
  const user = userEvent.setup()
  const input = await renderSection(false)
  expect(input).toBeEnabled()

  const toggle = screen.getByRole('switch', {
    name: 'Keep the price in sync with the exchange rate',
  })
  expect(toggle).not.toBeChecked()

  await user.click(toggle)

  await waitFor(() => expect(input).toBeDisabled())
  await user.click(screen.getByRole('button', { name: 'Save all settings' }))
  await waitFor(() =>
    expect(api.put).toHaveBeenCalledWith('/api/option/', {
      key: 'PriceFollowsExchangeRate',
      value: true,
    })
  )
})

test('a price already maintained by the job opens read-only', async () => {
  const input = await renderSection(true)
  expect(input).toBeDisabled()
  expect(
    screen.getByRole('switch', { name: /Keep the price in sync/ })
  ).toBeChecked()
})
