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
import { Loader2 } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { bindPhone, sendBindPhoneCode } from '@/features/auth/api'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { useCountdown } from '@/hooks/use-countdown'
import { useStatus } from '@/hooks/use-status'
import { handleServerError } from '@/lib/handle-server-error'
import { AuthOperationError } from '@/lib/secure-verification'
import { createServerError } from '@/lib/server-error-message'

type PhoneBindDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

/**
 * Bind a phone number to the signed-in account. The code is sent without a
 * step-up proof; the proof is requested only for the bind itself, so the user
 * passes one verification dialog instead of two.
 */
export function PhoneBindDialog(props: PhoneBindDialogProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const [phone, setPhone] = useState('')
  const [code, setCode] = useState('')
  const [isSending, setIsSending] = useState(false)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const {
    secondsLeft,
    isActive: countdownActive,
    start: startCountdown,
  } = useCountdown({ initialSeconds: 60 })
  const { requestVerification, isActive, dialogProps } = useSecureVerification()

  const countryCode = String(
    status?.sms_default_country_code ??
      status?.data?.sms_default_country_code ??
      '+86'
  )

  useEffect(() => {
    if (!props.open) {
      setCode('')
      setIsSubmitting(false)
    }
  }, [props.open])

  async function handleSendCode() {
    if (!phone.trim()) {
      toast.error(t('Please enter your phone number'))
      return
    }
    setIsSending(true)
    try {
      const res = await sendBindPhoneCode(phone.trim())
      if (res?.success) {
        startCountdown()
        toast.success(t('Verification code sent'))
      } else {
        handleServerError(
          createServerError(res, t('Failed to send verification code'))
        )
      }
    } catch (error) {
      handleServerError(
        AuthOperationError.from(error, t('Failed to send verification code'))
      )
    } finally {
      setIsSending(false)
    }
  }

  async function handleSubmit() {
    const trimmedPhone = phone.trim()
    const trimmedCode = code.trim()
    if (!trimmedPhone || !trimmedCode) {
      toast.error(t('Please enter your phone number and the verification code'))
      return
    }
    setIsSubmitting(true)
    try {
      const proof = await requestVerification({
        scope: 'account.binding.bind',
        context: { provider: 'phone', phone: trimmedPhone },
      })
      if (!proof) return
      const res = await bindPhone(
        { phone: trimmedPhone, code: trimmedCode },
        proof.proof_token
      )
      if (res?.success) {
        toast.success(t('Phone number bound successfully'))
        props.onSuccess?.()
        props.onOpenChange(false)
      } else {
        handleServerError(
          createServerError(res, t('Failed to bind phone number'))
        )
      }
    } catch (error) {
      handleServerError(AuthOperationError.from(error))
    } finally {
      setIsSubmitting(false)
    }
  }

  let codeAction: ReactNode = t('Send code')
  if (isSending) {
    codeAction = <Loader2 className='h-4 w-4 animate-spin' />
  } else if (countdownActive) {
    codeAction = t('Resend ({{seconds}}s)', { seconds: secondsLeft })
  }

  return (
    <>
      <Dialog
        open={props.open}
        onOpenChange={props.onOpenChange}
        title={t('Bind phone number')}
        description={t(
          'Bind a phone number to sign in with a code and to recover your password by SMS.'
        )}
        contentClassName='max-w-sm'
        headerClassName='text-left'
        contentHeight='auto'
        bodyClassName='space-y-4'
        footer={
          <>
            <Button
              type='button'
              variant='outline'
              onClick={() => props.onOpenChange(false)}
              disabled={isSubmitting}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='button'
              onClick={handleSubmit}
              disabled={isSubmitting || !phone.trim() || !code.trim()}
            >
              {isSubmitting ? <Loader2 className='h-4 w-4 animate-spin' /> : null}
              {t('Bind')}
            </Button>
          </>
        }
      >
        <div className='grid gap-2'>
          <Label htmlFor='phone-bind-number'>{t('Phone number')}</Label>
          <Input
            id='phone-bind-number'
            value={phone}
            onChange={(event) => setPhone(event.target.value)}
            placeholder={`${countryCode} 13800138000`}
            inputMode='tel'
            autoComplete='tel'
          />
          <p className='text-muted-foreground text-xs'>
            {t('Mainland numbers can be typed without the country code.')}
          </p>
        </div>
        <div className='grid gap-2'>
          <Label htmlFor='phone-bind-code'>{t('Verification code')}</Label>
          <div className='flex gap-2'>
            <Input
              id='phone-bind-code'
              value={code}
              onChange={(event) => setCode(event.target.value)}
              placeholder={t('Enter the verification code')}
              autoComplete='one-time-code'
            />
            <Button
              type='button'
              variant='outline'
              className='shrink-0'
              disabled={isSending || countdownActive || !phone.trim()}
              onClick={handleSendCode}
            >
{codeAction}
            </Button>
          </div>
        </div>
      </Dialog>
      {isActive && <SecureVerificationDialog {...dialogProps} />}
    </>
  )
}
