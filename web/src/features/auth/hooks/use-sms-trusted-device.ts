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
import { useEffect, useState } from 'react'

import { getSmsDeviceTrust } from '../api'

/**
 * Reports whether this browser may request a login code for the phone number
 * without the image challenge. The answer comes from the HttpOnly trusted
 * device cookie, which JavaScript cannot read, so it has to be asked for.
 */
export function useSmsTrustedDevice(phone: string, enabled: boolean) {
  const [trusted, setTrusted] = useState(false)

  useEffect(() => {
    setTrusted(false)
    if (!enabled) return
    const trimmed = phone.trim()
    if (trimmed.length < 7) return

    const controller = new AbortController()
    const timer = window.setTimeout(() => {
      getSmsDeviceTrust(trimmed, controller.signal)
        .then((response) => {
          if (!controller.signal.aborted) {
            setTrusted(Boolean(response.success && response.data?.trusted))
          }
        })
        .catch(() => {
          // A failed probe must fall back to showing the challenge.
        })
    }, 400)

    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  }, [phone, enabled])

  return trusted
}
