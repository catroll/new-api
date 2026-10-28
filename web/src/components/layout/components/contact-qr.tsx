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
 * Stacked rather than side by side: the card is anchored to the right edge, so
 * every pixel of width it takes eats into the content column, while height is
 * free. Text on top, code below, keeps the footprint at ~190px wide.
 */
function ContactCard() {
  const { t } = useTranslation()
  return (
    <div
      data-slot='contact-qr-card'
      className='bg-background/95 border-border/70 w-48 rounded-2xl border p-3 text-center shadow-xl backdrop-blur'
    >
      <p className='text-sm leading-snug font-semibold'>
        {t('Need more models? Contact sales.')}
      </p>
      <p className='text-muted-foreground mt-1 text-xs leading-relaxed'>
        {t('Scan the QR code to add me as a friend.')}
      </p>
      <div className='mx-auto mt-2 w-fit rounded-lg bg-white p-1 ring-1 ring-black/5'>
        <img
          src='/qr-contact-wechat.png'
          alt={t('WeChat QR code')}
          width={80}
          height={80}
          loading='lazy'
          className='size-20 rounded'
        />
      </div>
    </div>
  )
}
