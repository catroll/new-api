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
import i18next from 'i18next'
import { useState } from 'react'
import { toast } from 'sonner'

import { useCountdown } from '@/hooks/use-countdown'
import { handleServerError } from '@/lib/handle-server-error'
import { AuthOperationError } from '@/lib/secure-verification'
import { createServerError } from '@/lib/server-error-message'

import { sendSmsCode } from '../api'
import { SMS_CODE_COUNTDOWN } from '../constants'
import type { SmsCodePurpose } from '../types'

interface UseSmsVerificationOptions {
  turnstileToken?: string
  validateTurnstile?: () => boolean
}

/**
 * Send an SMS verification code and drive the resend countdown. Mirrors
 * use-email-verification, but with a purpose because the same endpoint serves
 * login, registration and password reset.
 */
export function useSmsVerification(options?: UseSmsVerificationOptions) {
  const [isSending, setIsSending] = useState(false)
  const {
    secondsLeft,
    isActive,
    start: startCountdown,
  } = useCountdown({ initialSeconds: SMS_CODE_COUNTDOWN })

  const sendCode = async (phone: string, purpose: SmsCodePurpose = 'login') => {
    if (!phone.trim()) {
      toast.error(i18next.t('Please enter your phone number'))
      return false
    }
    if (options?.validateTurnstile && !options.validateTurnstile()) {
      return false
    }

    setIsSending(true)
    try {
      const res = await sendSmsCode({
        phone: phone.trim(),
        purpose,
        turnstile: options?.turnstileToken,
      })
      if (res?.success) {
        startCountdown()
        toast.success(i18next.t('Verification code sent'))
        return true
      }
      handleServerError(
        createServerError(res, i18next.t('Failed to send verification code'))
      )
      return false
    } catch (_error) {
      handleServerError(
        AuthOperationError.from(
          _error,
          i18next.t('Failed to send verification code')
        )
      )
      return false
    } finally {
      setIsSending(false)
    }
  }

  return {
    isSending,
    secondsLeft,
    isActive,
    sendCode,
  }
}
