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
import { useCallback, useEffect, useState } from 'react'
import { toast } from 'sonner'

import { useSystemConfigStore } from '@/stores/system-config-store'

import { getCaptcha, type CaptchaChallenge } from '../api'

/**
 * Drives the self-hosted image challenge. The backend keeps the answer and
 * accepts an "id:answer" token once, so `reset()` is required after any request
 * that reaches the server.
 */
export function useCaptcha() {
  // The status fetch updates this persisted store, so forms work even inside
  // dialogs rendered without a React Query provider (for example tests).
  const isEnabled = useSystemConfigStore((state) => state.config.captchaCheck)
  const [challenge, setChallenge] = useState<CaptchaChallenge | null>(null)
  const [code, setCode] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    if (!isEnabled) {
      setChallenge(null)
      setCode('')
      return
    }
    setIsLoading(true)
    setError('')
    try {
      const res = await getCaptcha()
      if (res.success && res.data) {
        setChallenge(res.data)
        setCode('')
      } else {
        setError(res.message || i18next.t('Failed to load the human check'))
      }
    } catch {
      setError(i18next.t('Failed to load the human check'))
    } finally {
      setIsLoading(false)
    }
  }, [isEnabled])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const token =
    isEnabled && challenge && code.trim()
      ? `${challenge.id}:${code.trim()}`
      : ''

  const ready = !isEnabled || Boolean(token)

  const validate = useCallback(() => {
    if (isEnabled && !token) {
      toast.info(i18next.t('Please complete the human verification first'))
      return false
    }
    return true
  }, [isEnabled, token])

  const reset = useCallback(() => {
    setCode('')
    void refresh()
  }, [refresh])

  return {
    isEnabled,
    challenge,
    code,
    setCode,
    token,
    ready,
    isLoading,
    error,
    refresh,
    reset,
    validate,
  }
}
