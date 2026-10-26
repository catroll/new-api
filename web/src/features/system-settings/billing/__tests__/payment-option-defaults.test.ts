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

For commercial licensing, please contact support@quantumnous.com.
*/
import { describe, expect, test } from 'vitest'

import { getOptionValue } from '../../hooks/use-system-options'
import { defaultBillingSettings } from '../default-settings'

// getOptionValue only copies a server option when its key already exists in the
// defaults object. A gateway whose keys are missing there saves successfully and
// then reads back empty after a refresh, which is exactly what happened to the
// Alipay fields.
describe('payment settings defaults', () => {
  const alipayOptions = [
    { key: 'AlipayAppId', value: '2021002147687843' },
    { key: 'AlipayPrivateKey', value: 'MIIEvQIBADANBgkqhkiG9w0BAQEFAASC' },
    { key: 'AlipayPublicKey', value: 'MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A' },
    { key: 'AlipaySandbox', value: 'true' },
    { key: 'AlipayMinTopUp', value: '5' },
  ]

  test('Alipay keys survive the merge instead of collapsing to the defaults', () => {
    const merged = getOptionValue(alipayOptions, defaultBillingSettings)

    expect(merged.AlipayAppId).toBe('2021002147687843')
    expect(merged.AlipayPrivateKey).toBe(
      'MIIEvQIBADANBgkqhkiG9w0BAQEFAASC'
    )
    expect(merged.AlipayPublicKey).toBe('MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A')
    expect(merged.AlipaySandbox).toBe(true)
    expect(merged.AlipayMinTopUp).toBe(5)
  })

  test('every payment gateway option the console edits has a default', () => {
    const editedKeys = [
      'AlipayAppId',
      'AlipayPrivateKey',
      'AlipayPublicKey',
      'AlipaySandbox',
      'AlipayMinTopUp',
      'StripeApiSecret',
      'StripeWebhookSecret',
      'StripePriceId',
      'StripeUnitPrice',
      'StripeMinTopUp',
      'StripePromotionCodesEnabled',
      'CreemApiKey',
      'CreemWebhookSecret',
      'CreemTestMode',
      'CreemProducts',
    ]
    for (const key of editedKeys) {
      expect(defaultBillingSettings, key).toHaveProperty(key)
    }
  })
})
