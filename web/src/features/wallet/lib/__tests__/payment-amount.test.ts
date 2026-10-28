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

For commercial licensing, please contact support@quantumnous.com.
*/
import { describe, expect, test } from 'vitest'

import { formatPaymentAmount } from '../format'

// The wallet shows two currencies at once on a dollar-priced site: "$10 of
// credit" and the CNY the gateway charges. Only the label tells them apart, so
// the charge must carry its own symbol instead of inheriting the display one.
describe('formatPaymentAmount', () => {
  test('marks a CNY charge with the yuan sign', () => {
    expect(formatPaymentAmount(73, 'CNY')).toBe('¥73')
    expect(formatPaymentAmount(7.3, 'CNY')).toBe('¥7.3')
  })

  test('leaves an unlabelled amount alone when the currency is not set', () => {
    expect(formatPaymentAmount(10)).toBe('10')
    expect(formatPaymentAmount(10, 'USD')).toBe('10')
  })
})
