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
import { describe, expect, test } from 'vitest'

import { PAYMENT_TYPES } from '../constants'
import type { TopupInfo } from '../types'
import {
  dispatchSelectedPayment,
  getDefaultPaymentType,
  getMinTopupAmount,
  hasConfigurableTopup,
  isStripePayment,
  isWaffoPayment,
  isWaffoPancakePayment,
} from './payment'

describe('payment type classification', () => {
  test('keeps Waffo and Waffo Pancake on their dedicated flows', () => {
    expect(isWaffoPayment(PAYMENT_TYPES.WAFFO)).toBe(true)
    expect(isWaffoPayment(PAYMENT_TYPES.WAFFO_PANCAKE)).toBe(false)
    expect(isWaffoPancakePayment(PAYMENT_TYPES.WAFFO_PANCAKE)).toBe(true)
    expect(isWaffoPancakePayment(PAYMENT_TYPES.WAFFO)).toBe(false)
    expect(isStripePayment(PAYMENT_TYPES.STRIPE)).toBe(true)
  })
})

describe('payment dispatch', () => {
  test('keeps the selected Waffo method index through confirmation', async () => {
    const calls: string[] = []
    const success = await dispatchSelectedPayment(
      { name: 'Waffo Card', type: PAYMENT_TYPES.WAFFO },
      120,
      3,
      {
        regular: async () => {
          calls.push('regular')
          return false
        },
        waffo: async (amount, index) => {
          calls.push(`waffo:${amount}:${index}`)
          return true
        },
        waffoPancake: async () => {
          calls.push('pancake')
          return false
        },
      }
    )

    expect(success).toBe(true)
    expect(calls).toEqual(['waffo:120:3'])
  })

  test('does not create a Waffo order without a selected method index', async () => {
    let called = false
    const success = await dispatchSelectedPayment(
      { name: 'Waffo Card', type: PAYMENT_TYPES.WAFFO },
      120,
      null,
      {
        regular: async () => false,
        waffo: async () => {
          called = true
          return true
        },
        waffoPancake: async () => false,
      }
    )

    expect(success).toBe(false)
    expect(called).toBe(false)
  })
})

describe('gateway enablement aggregation', () => {
  const base = {
    enable_online_topup: false,
    enable_stripe_topup: false,
    enable_waffo_topup: false,
    enable_waffo_pancake_topup: false,
    pay_methods: [],
    min_topup: 1,
    stripe_min_topup: 1,
  }

  test('counts the direct Alipay gateway as an enabled top-up', () => {
    // Regression: the wallet showed "Online topup is not enabled" while
    // /api/user/topup/info reported the flag, because this aggregation did not
    // list the new gateway.
    expect(
      hasConfigurableTopup({
        ...base,
        enable_alipay_topup: true,
      } as unknown as TopupInfo)
    ).toBe(true)
  })

  test('reports nothing enabled when every flag is false', () => {
    expect(hasConfigurableTopup({ ...base } as unknown as TopupInfo)).toBe(false)
    expect(hasConfigurableTopup(null)).toBe(false)
  })

  test('uses the Alipay minimum when it is the only gateway', () => {
    expect(
      getMinTopupAmount({
        ...base,
        enable_alipay_topup: true,
        alipay_min_topup: 5,
      } as unknown as TopupInfo)
    ).toBe(5)
  })

  test('defaults to the direct Alipay method when it is the only one', () => {
    expect(
      getDefaultPaymentType({
        ...base,
        enable_alipay_topup: true,
      } as unknown as TopupInfo)
    ).toBe(PAYMENT_TYPES.ALIPAY_DIRECT)
  })
})
