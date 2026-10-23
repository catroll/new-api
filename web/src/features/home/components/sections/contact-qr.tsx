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
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'

/**
 * Contact QR code at the bottom of the home page.
 *
 * The QR is a plain file from `web/public`, not a bundled import: deployments
 * replace it with their own code (`deploy/qr-contact-wechat.png` in the release
 * pipeline), so the URL has to stay a stable path on the site root.
 *
 * A transparent QR disappears on a dark theme, which is why the code sits on a
 * fixed white card instead of the themed surface. 256 px source shown at 128 CSS
 * pixels keeps it crisp on 2x screens while staying small enough for a footer.
 */
export function ContactQr() {
  const { t } = useTranslation()

  return (
    <AnimateInView
      as='section'
      className='relative z-10 px-6 pt-4 pb-16 md:pb-20'
    >
      <div className='mx-auto flex max-w-6xl flex-col items-center gap-3 text-center'>
        <p className='text-sm font-medium'>{t('Contact me on WeChat')}</p>
        <div className='rounded-2xl bg-white p-3 shadow-sm ring-1 ring-black/5'>
          <img
            src='/qr-contact-wechat.png'
            alt={t('WeChat QR code')}
            width={128}
            height={128}
            loading='lazy'
            className='size-32 rounded-lg'
          />
        </div>
        <p className='text-muted-foreground/60 max-w-xs text-xs leading-relaxed'>
          {t('Scan the QR code to add me as a friend.')}
        </p>
      </div>
    </AnimateInView>
  )
}
