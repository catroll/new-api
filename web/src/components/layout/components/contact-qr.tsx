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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

/**
 * WeChat contact card that floats over every public page.
 *
 * A fixed card on the right edge keeps the contact entry reachable while
 * scrolling, but on a narrow viewport that same card would sit on top of the
 * content, so phones get a round QR button that opens it on tap instead.
 */
export function ContactQr() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  return (
    <>
      <aside
        aria-label={t('WeChat QR code')}
        data-slot='contact-qr-panel'
        className='fixed top-1/2 right-4 z-40 hidden -translate-y-1/2 lg:block'
      >
        <ContactCard />
      </aside>

      <div className='lg:hidden'>
        {open ? (
          <button
            type='button'
            aria-label={t('Close')}
            className='bg-background/40 fixed inset-0 z-40 backdrop-blur-[1px]'
            onClick={() => setOpen(false)}
          />
        ) : null}
        {open ? (
          <div
            className='fixed right-3 bottom-20 z-50'
            data-slot='contact-qr-popover'
          >
            <ContactCard />
          </div>
        ) : null}
        <button
          type='button'
          aria-label={t('WeChat QR code')}
          aria-expanded={open}
          onClick={() => setOpen((value) => !value)}
          data-slot='contact-qr-button'
          className='bg-background fixed right-4 bottom-4 z-50 flex size-12 items-center justify-center rounded-full border shadow-lg'
        >
          <img
            src='/qr-contact-wechat.png'
            alt=''
            width={36}
            height={36}
            loading='lazy'
            className='size-9 rounded bg-white object-contain p-0.5'
          />
        </button>
      </div>
    </>
  )
}

/*
 * The text and the code share one 144px column: the card is anchored to the
 * right edge, so width eats into the content column while height does not, and
 * matching the two widths keeps the code as large as the card can afford
 * without the headline spilling past it.
 */
function ContactCard() {
  const { t } = useTranslation()
  return (
    <div
      data-slot='contact-qr-card'
      className='bg-background/95 border-border/70 rounded-2xl border p-3 text-center shadow-xl backdrop-blur'
    >
      <p className='mx-auto w-36 text-sm leading-snug font-semibold'>
        {t('Need more models? Contact sales.')}
      </p>
      <div className='mx-auto mt-2 w-36 overflow-hidden rounded-lg bg-white ring-1 ring-black/5'>
        <img
          src='/qr-contact-wechat.png'
          alt={t('WeChat QR code')}
          width={144}
          height={144}
          loading='lazy'
          className='block w-full rounded'
        />
      </div>
    </div>
  )
}
