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
import { zodResolver } from '@hookform/resolvers/zod'
import { CheckIcon, CopyIcon, Loader2 } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { z } from 'zod'

import { Turnstile } from '@/components/turnstile'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { resetPasswordBySms } from '@/features/auth/api'
import { forgotPasswordSmsFormSchema } from '@/features/auth/constants'
import { useSmsVerification } from '@/features/auth/hooks/use-sms-verification'
import { useTurnstile } from '@/features/auth/hooks/use-turnstile'
import { copyToClipboard } from '@/lib/copy-to-clipboard'
import { handleServerError } from '@/lib/handle-server-error'
import { AuthOperationError } from '@/lib/secure-verification'
import { createServerError } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

export function SmsResetForm({
  countryCode,
  className,
  ...props
}: React.HTMLAttributes<HTMLFormElement> & { countryCode: string }) {
  const { t } = useTranslation()
  const [isLoading, setIsLoading] = useState(false)
  const [newPassword, setNewPassword] = useState('')
  const [copied, setCopied] = useState(false)

  const {
    isTurnstileEnabled,
    turnstileSiteKey,
    turnstileToken,
    setTurnstileToken,
    validateTurnstile,
  } = useTurnstile()
  const {
    isSending,
    secondsLeft,
    isActive,
    sendCode: sendResetCode,
  } = useSmsVerification({ turnstileToken, validateTurnstile })

  const form = useForm<z.infer<typeof forgotPasswordSmsFormSchema>>({
    resolver: zodResolver(forgotPasswordSmsFormSchema),
    defaultValues: { phone: '', code: '' },
  })
  const phoneValue = useWatch({ control: form.control, name: 'phone' })

  async function sendCode() {
    if (await sendResetCode(form.getValues('phone') || '', 'reset')) {
      setTurnstileToken('')
    }
  }

  async function onSubmit(data: z.infer<typeof forgotPasswordSmsFormSchema>) {
    setIsLoading(true)
    try {
      const res = await resetPasswordBySms({
        phone: data.phone,
        code: data.code,
      })
      if (res?.success) {
        setNewPassword(String(res.data ?? ''))
        toast.success(t('Password reset'))
      } else {
        handleServerError(createServerError(res, t('Request failed')))
      }
    } catch (error) {
      handleServerError(AuthOperationError.from(error))
    } finally {
      setIsLoading(false)
    }
  }

  async function handleCopy() {
    if (await copyToClipboard(newPassword)) {
      setCopied(true)
      toast.success(t('Password copied to clipboard'))
      setTimeout(() => setCopied(false), 2000)
    }
  }

  let codeAction: ReactNode = t('Send code')
  if (isSending) {
    codeAction = <Loader2 className='h-4 w-4 animate-spin' />
  } else if (isActive) {
    codeAction = t('Resend ({{seconds}}s)', { seconds: secondsLeft })
  }

  if (newPassword) {
    return (
      <div className={cn('grid gap-4', className)}>
        <p className='text-muted-foreground text-sm'>
          {t('Your password has been reset. Save the new password below.')}
        </p>
        <div className='space-y-2'>
          <Label htmlFor='sms-reset-password'>{t('New password')}</Label>
          <div className='flex gap-2'>
            <Input
              id='sms-reset-password'
              value={newPassword}
              disabled
              className='font-mono'
            />
            <Button
              type='button'
              size='icon'
              variant='outline'
              onClick={handleCopy}
            >
              {copied ? (
                <CheckIcon className='h-4 w-4' />
              ) : (
                <CopyIcon className='h-4 w-4' />
              )}
            </Button>
          </div>
        </div>
      </div>
    )
  }

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className={cn('grid gap-4', className)}
        {...props}
      >
        <FormField
          control={form.control}
          name='phone'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Phone number')}</FormLabel>
              <FormControl>
                <Input
                  placeholder={`${countryCode} 13800138000`}
                  inputMode='tel'
                  autoComplete='tel'
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name='code'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Verification code')}</FormLabel>
              <div className='flex gap-2'>
                <FormControl>
                  <Input
                    placeholder={t('Enter the verification code')}
                    autoComplete='one-time-code'
                    {...field}
                  />
                </FormControl>
                <Button
                  type='button'
                  variant='outline'
                  className='shrink-0'
                  disabled={isSending || isActive || !phoneValue?.trim()}
                  onClick={sendCode}
                >
{codeAction}
                </Button>
              </div>
              <FormMessage />
            </FormItem>
          )}
        />

        <Button type='submit' className='w-full' disabled={isLoading}>
          {isLoading ? <Loader2 className='animate-spin' /> : null}
          {t('Reset password')}
        </Button>

        {isTurnstileEnabled && (
          <div className='mt-2'>
            <Turnstile siteKey={turnstileSiteKey} onVerify={setTurnstileToken} />
          </div>
        )}
      </form>
    </Form>
  )
}
