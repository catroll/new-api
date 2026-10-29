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
import { KeyRound, Loader2, ShieldCheck, Smartphone } from 'lucide-react'
import { useEffect, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { AuthOperationError } from '@/lib/secure-verification'

import type { PasskeyDomains } from '../../passkey/assertion'
import { PasskeyDomainSelector } from '../../passkey/components/passkey-domain-selector'
import { sendVerificationSMS } from '../api'
import type {
  SecureVerificationState,
  VerificationInput,
  VerificationMethod,
  VerificationOperation,
} from '../types'

interface SecureVerificationDialogProps {
  state: SecureVerificationState
  passkeyDomains?: PasskeyDomains | null
  onVerify: (input?: VerificationInput) => void | Promise<void>
  onCancel: () => void
  onRetry: () => void
  onInputChange: (input: VerificationInput) => void
}

const methodLabels: Record<VerificationMethod, string> = {
  '2fa': 'Authenticator code',
  passkey: 'Passkey',
  password: 'Password',
  oauth: 'Linked account',
  sms: 'SMS code',
  session: 'Login session',
}

interface SMSDeliveryState {
  status: 'idle' | 'sending' | 'sent'
  seconds: number
  error?: string
}

export function SecureVerificationDialog(props: SecureVerificationDialogProps) {
  const { t } = useTranslation()
  const inputId = useId()
  const state = props.state
  const [sms, setSMS] = useState<SMSDeliveryState>({
    status: 'idle',
    seconds: 0,
  })

  useEffect(() => {
    if (sms.seconds <= 0) return
    const timer = setTimeout(
      () =>
        setSMS((current) => ({
          ...current,
          seconds: Math.max(0, current.seconds - 1),
        })),
      1000
    )
    return () => clearTimeout(timer)
  }, [sms.seconds])

  if (state.phase === 'idle') return null
  const ready =
    state.phase === 'ready' || state.phase === 'verifying' ? state : null
  const input = ready?.input
  const verifying = state.phase === 'verifying'
  const login = state.request.scope === 'auth.login'
  const acceptsBackupCode =
    state.request.scope !== '2fa.backup_codes.regenerate'
  let canVerify = state.phase === 'ready' && Boolean(input)
  if (input?.method === 'password') {
    canVerify = canVerify && input.password.length > 0
  }
  if (input?.method === '2fa') {
    canVerify = canVerify && input.code.trim().length >= 6
  }
  if (input?.method === 'sms') {
    canVerify = canVerify && input.code.trim().length >= 6
  }
  // Linked-account verification starts from the provider button itself.
  const showSubmit = state.phase !== 'error' && input?.method !== 'oauth'
  const error = 'error' in state ? state.error : undefined
  const formId = `${inputId}-form`
  // Login never offers SMS, so a request that carries a login challenge has no
  // operation to send a code for.
  const smsOperation: VerificationOperation | null =
    state.request.scope === 'auth.login' ? null : state.request
  const smsPhone = ready?.requirements.phone_masked
  const sendingSMS = sms.status === 'sending'
  const smsCooldown = sms.seconds > 0

  const sendCode = async () => {
    if (!smsOperation || sendingSMS || smsCooldown) return
    setSMS({ status: 'sending', seconds: 0 })
    try {
      const delivery = await sendVerificationSMS(smsOperation)
      setSMS({
        status: 'sent',
        seconds: delivery.resend_seconds ?? 60,
      })
    } catch (failure) {
      setSMS({
        status: 'idle',
        seconds: 0,
        error: AuthOperationError.from(failure).message,
      })
    }
  }

  const selectMethod = (method: string) => {
    switch (method) {
      case '2fa':
        props.onInputChange({ method, code: '' })
        break
      case 'sms':
        props.onInputChange({ method, code: '' })
        break
      case 'password':
        props.onInputChange({ method, password: '' })
        break
      case 'passkey':
        props.onInputChange({ method })
        break
      case 'oauth':
        props.onInputChange({
          method,
          provider: ready?.requirements.oauth_providers[0]?.slug ?? '',
        })
        break
    }
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onCancel()
      }}
      title={
        <>
          <ShieldCheck className='size-5' />
          {state.request.title ??
            (login ? t('Complete sign-in') : t('Security verification'))}
        </>
      }
      description={
        state.request.description ??
        (login
          ? t('Verify your identity to finish signing in.')
          : t('Confirm your identity before accessing this sensitive action.'))
      }
      contentClassName='sm:max-w-md'
      contentHeight='auto'
      titleClassName='flex items-center gap-2'
      footer={
        <>
          <Button type='button' variant='outline' onClick={props.onCancel}>
            {t('Cancel')}
          </Button>
          {state.phase === 'error' && (
            <Button type='button' onClick={props.onRetry}>
              {t('Retry')}
            </Button>
          )}
          {showSubmit && (
            <Button type='submit' form={formId} disabled={!canVerify}>
              {verifying && <Loader2 className='size-4 animate-spin' />}
              {t('Verify')}
            </Button>
          )}
        </>
      }
    >
      {state.phase === 'loading' && (
        <div role='status' className='flex items-center gap-2 py-4'>
          <Loader2 className='size-4 animate-spin' />
          {t('Loading verification methods...')}
        </div>
      )}
      {error && (
        <p role='alert' className='text-destructive text-sm'>
          {t(error)}
        </p>
      )}
      {ready && (
        <form
          id={formId}
          onSubmit={(event) => {
            event.preventDefault()
            if (canVerify) void props.onVerify()
          }}
          className='space-y-4 py-2'
        >
          {!input ? (
            <div className='space-y-2 text-sm'>
              <p>{t('No verification method is available for this action.')}</p>
              {ready.requirements.methods.map(
                (option) =>
                  option.reason && (
                    <p className='text-muted-foreground' key={option.method}>
                      {t(option.reason)}
                    </p>
                  )
              )}
            </div>
          ) : (
            <Tabs value={input.method} onValueChange={selectMethod}>
              <TabsList>
                {ready.requirements.methods.map((option) => (
                  <TabsTrigger
                    key={option.method}
                    value={option.method}
                    disabled={!option.available || verifying}
                  >
                    {t(methodLabels[option.method])}
                  </TabsTrigger>
                ))}
              </TabsList>
              <TabsContent value='password' className='space-y-2'>
                <Label htmlFor={inputId}>{t('Password')}</Label>
                <Input
                  id={inputId}
                  type='password'
                  autoComplete='current-password'
                  autoFocus
                  disabled={verifying}
                  value={input.method === 'password' ? input.password : ''}
                  onChange={(event) =>
                    props.onInputChange({
                      method: 'password',
                      password: event.target.value,
                    })
                  }
                />
              </TabsContent>
              <TabsContent value='2fa' className='space-y-2'>
                <Label htmlFor={inputId}>
                  {acceptsBackupCode
                    ? t('Authenticator code or backup code')
                    : t('Authenticator code')}
                </Label>
                <Input
                  id={inputId}
                  autoComplete='one-time-code'
                  maxLength={acceptsBackupCode ? 9 : 6}
                  autoFocus
                  disabled={verifying}
                  value={input.method === '2fa' ? input.code : ''}
                  onChange={(event) =>
                    props.onInputChange({
                      method: '2fa',
                      code: event.target.value,
                    })
                  }
                />
                <p className='text-muted-foreground text-sm'>
                  {acceptsBackupCode
                    ? t(
                        'Enter the 6-digit authenticator code or an unused backup code.'
                      )
                    : t('Enter the 6-digit authenticator code.')}
                </p>
              </TabsContent>
              <TabsContent value='sms' className='space-y-3'>
                <p className='text-muted-foreground flex items-center gap-2 text-sm'>
                  <Smartphone className='size-5' />
                  {smsPhone
                    ? t('We will text a 6-digit code to {{phone}}.', {
                        phone: smsPhone,
                      })
                    : t('We will text a 6-digit code to your bound phone.')}
                </p>
                <Button
                  type='button'
                  variant='outline'
                  disabled={verifying || sendingSMS || smsCooldown}
                  onClick={() => void sendCode()}
                >
                  {sendingSMS && <Loader2 className='size-4 animate-spin' />}
                  {smsCooldown
                    ? t('Resend in {{seconds}}s', { seconds: sms.seconds })
                    : t('Send code')}
                </Button>
                {sms.error && (
                  <p role='alert' className='text-destructive text-sm'>
                    {t(sms.error)}
                  </p>
                )}
                <div className='space-y-2'>
                  <Label htmlFor={inputId}>{t('Verification code')}</Label>
                  <Input
                    id={inputId}
                    autoComplete='one-time-code'
                    inputMode='numeric'
                    maxLength={6}
                    autoFocus
                    disabled={verifying}
                    value={input.method === 'sms' ? input.code : ''}
                    onChange={(event) =>
                      props.onInputChange({
                        method: 'sms',
                        code: event.target.value,
                      })
                    }
                  />
                </div>
                {sms.status === 'sent' && (
                  <p className='text-muted-foreground text-sm'>
                    {t('Verification code sent')}
                  </p>
                )}
              </TabsContent>
              <TabsContent value='passkey' className='space-y-3'>
                <PasskeyDomainSelector
                  domains={props.passkeyDomains}
                  value={input.method === 'passkey' ? input.rpID : undefined}
                  onChange={(rpID) =>
                    props.onInputChange({ method: 'passkey', rpID })
                  }
                  disabled={verifying}
                />
                <p className='text-muted-foreground flex items-center gap-2 text-sm'>
                  <KeyRound className='size-5' />
                  {t(
                    'We will prompt your device to confirm using biometrics or your hardware key.'
                  )}
                </p>
              </TabsContent>
              <TabsContent value='oauth' className='space-y-2'>
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Continue with an account already linked to your profile.'
                  )}
                </p>
                <div className='flex flex-wrap gap-2'>
                  {ready.requirements.oauth_providers.map((provider) => (
                    <Button
                      key={provider.slug}
                      type='button'
                      disabled={verifying}
                      onClick={() =>
                        void props.onVerify({
                          method: 'oauth',
                          provider: provider.slug,
                        })
                      }
                    >
                      {verifying &&
                        input.method === 'oauth' &&
                        input.provider === provider.slug && (
                          <Loader2 className='size-4 animate-spin' />
                        )}
                      {t('Continue with {{name}}', { name: provider.name })}
                    </Button>
                  ))}
                </div>
              </TabsContent>
            </Tabs>
          )}
        </form>
      )}
    </Dialog>
  )
}
