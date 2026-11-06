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
import { Loader2, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import type { useCaptcha } from '../hooks/use-captcha'

type CaptchaState = ReturnType<typeof useCaptcha>

interface CaptchaProps {
  captcha: CaptchaState
  className?: string
  /** Extra explanation, for example when the challenge gates SMS sending. */
  hint?: string
}

export function Captcha({ captcha, className, hint }: CaptchaProps) {
  const { t } = useTranslation()
  if (!captcha.isEnabled) return null

  let challengeContent
  if (captcha.challenge) {
    challengeContent = (
      <img
        src={captcha.challenge.image}
        alt={t('Human verification image')}
        className='h-12 w-auto select-none'
        draggable={false}
      />
    )
  } else if (captcha.isLoading) {
    challengeContent = (
      <Loader2 className='text-muted-foreground h-5 w-5 animate-spin' />
    )
  } else {
    challengeContent = (
      <span className='text-muted-foreground px-3 text-xs'>
        {t('Challenge unavailable')}
      </span>
    )
  }

  return (
    <div className={className}>
      <div className='flex items-center gap-2'>
        <div className='flex h-12 min-w-32 items-center justify-center overflow-hidden rounded-md border bg-white'>
          {challengeContent}
        </div>
        <Button
          type='button'
          variant='outline'
          size='icon'
          aria-label={t('Refresh challenge')}
          onClick={() => void captcha.refresh()}
          disabled={captcha.isLoading}
        >
          <RefreshCw className={captcha.isLoading ? 'animate-spin' : ''} />
        </Button>
      </div>
      <Input
        className='mt-2'
        value={captcha.code}
        onChange={(event) => captcha.setCode(event.target.value)}
        placeholder={t('Enter the characters in the image')}
        autoComplete='off'
        autoCapitalize='none'
        spellCheck={false}
        inputMode='text'
        aria-label={t('Human verification')}
      />
      <p className='text-muted-foreground mt-1 text-xs'>
        {hint ?? t('Complete the image challenge to continue')}
      </p>
      {captcha.error ? (
        <p className='text-destructive mt-1 text-xs'>{captcha.error}</p>
      ) : null}
    </div>
  )
}
