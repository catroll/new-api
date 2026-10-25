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

import { useStatus } from '@/hooks/use-status'
import { useAuthStore } from '@/stores/auth-store'

import { PhoneBindDialog } from './phone-bind-dialog'

const REMINDER_STORAGE_KEY = 'sms-phone-reminder-shown'

/**
 * Non-blocking reminder for accounts without a phone while the site requires
 * one. It opens once per login: dismissing it only marks the current tab, so
 * the next sign-in asks again.
 */
export function PhoneReminder() {
  const { status } = useStatus()
  const user = useAuthStore((state) => state.auth.user)
  const setUser = useAuthStore((state) => state.auth.setUser)
  const [open, setOpen] = useState(false)

  const requirePhone = Boolean(
    status?.sms_require_phone ?? status?.data?.sms_require_phone
  )

  useEffect(() => {
    if (!requirePhone || !user || user.has_phone) return
    if (sessionStorage.getItem(REMINDER_STORAGE_KEY) === '1') return
    setOpen(true)
  }, [requirePhone, user])

  const handleOpenChange = (next: boolean) => {
    setOpen(next)
    if (!next) {
      sessionStorage.setItem(REMINDER_STORAGE_KEY, '1')
    }
  }

  const handleSuccess = () => {
    sessionStorage.setItem(REMINDER_STORAGE_KEY, '1')
    if (user) {
      setUser({ ...user, has_phone: true })
    }
  }

  return (
    <PhoneBindDialog
      open={open}
      onOpenChange={handleOpenChange}
      onSuccess={handleSuccess}
    />
  )
}
