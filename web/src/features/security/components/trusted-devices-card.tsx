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
import { ShieldKeyIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import {
  getTrustedDevices,
  revokeAllTrustedDevices,
  revokeTrustedDevice,
  type TrustedDevice,
} from '@/features/profile/api'
import { toIntlLocale } from '@/i18n/languages'
import dayjs from '@/lib/dayjs'
import { formatTimestampRelative } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { createServerError } from '@/lib/server-error-message'

import { sessionDevice } from './login-session-utils'

const trustedDeviceQueryKey = ['profile', 'trusted-devices'] as const

export function TrustedDevicesCard() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [revokeTarget, setRevokeTarget] = useState<TrustedDevice | null>(null)
  const [confirmAll, setConfirmAll] = useState(false)

  const devicesQuery = useQuery({
    queryKey: trustedDeviceQueryKey,
    queryFn: async () => {
      const response = await getTrustedDevices()
      if (!response.success) {
        throw createServerError(response, t('Unable to load trusted devices'))
      }
      return response.data ?? []
    },
  })

  const revokeMutation = useMutation({
    mutationFn: async (deviceID: string) => {
      const response = await revokeTrustedDevice(deviceID)
      if (!response.success) {
        throw createServerError(response, t('Failed to forget trusted device'))
      }
    },
    onSuccess: async () => {
      setRevokeTarget(null)
      toast.success(t('Trusted device forgotten'))
      await queryClient.invalidateQueries({ queryKey: trustedDeviceQueryKey })
    },
    onError: (error: Error) => handleServerError(error),
  })

  const revokeAllMutation = useMutation({
    mutationFn: async () => {
      const response = await revokeAllTrustedDevices()
      if (!response.success) {
        throw createServerError(response, t('Failed to forget trusted devices'))
      }
    },
    onSuccess: async () => {
      setConfirmAll(false)
      toast.success(t('All trusted devices forgotten'))
      await queryClient.invalidateQueries({ queryKey: trustedDeviceQueryKey })
    },
    onError: (error: Error) => handleServerError(error),
  })

  const devices = devicesQuery.data ?? []
  let content
  if (devicesQuery.isLoading) {
    content = (
      <div className='flex flex-col gap-3'>
        <Skeleton className='h-16 w-full' />
        <Skeleton className='h-16 w-full' />
      </div>
    )
  } else if (devicesQuery.isError) {
    content = (
      <Empty>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={ShieldKeyIcon} strokeWidth={2} />
          </EmptyMedia>
          <EmptyTitle>{t('Unable to load trusted devices')}</EmptyTitle>
          <EmptyDescription>
            {t('Refresh the list and try again.')}
          </EmptyDescription>
        </EmptyHeader>
        <Button
          type='button'
          variant='outline'
          onClick={() => devicesQuery.refetch()}
        >
          {t('Retry')}
        </Button>
      </Empty>
    )
  } else if (devices.length === 0) {
    content = (
      <Empty>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={ShieldKeyIcon} strokeWidth={2} />
          </EmptyMedia>
          <EmptyTitle>{t('No trusted devices')}</EmptyTitle>
          <EmptyDescription>
            {t(
              'After signing in with an SMS code, this device can request future codes without the image challenge.'
            )}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  } else {
    content = (
      <div className='flex flex-col'>
        {devices.map((device, index) => (
          <div key={device.id}>
            {index > 0 && <Separator />}
            <div className='flex flex-col gap-3 py-4 sm:flex-row sm:items-center'>
              <div className='bg-muted flex size-10 shrink-0 items-center justify-center rounded-lg'>
                <HugeiconsIcon
                  icon={ShieldKeyIcon}
                  className='size-5'
                  strokeWidth={2}
                />
              </div>
              <div className='min-w-0 flex-1'>
                <div className='flex flex-wrap items-center gap-2'>
                  <p className='font-medium'>
                    {sessionDevice(
                      device.user_agent,
                      t('Unknown device'),
                      t('Browser'),
                      0
                    )}
                  </p>
                  {device.current && (
                    <Badge variant='secondary'>{t('Current')}</Badge>
                  )}
                </div>
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t('Trusted {{time}} · Expires {{expires}}', {
                    time: formatTimestampRelative(
                      device.created_at,
                      'seconds',
                      locale
                    ),
                    expires: dayjs
                      .unix(device.expires_at)
                      .format('YYYY-MM-DD HH:mm'),
                  })}
                </p>
              </div>
              <Button
                type='button'
                variant='ghost'
                size='sm'
                disabled={revokeMutation.isPending}
                onClick={() => setRevokeTarget(device)}
              >
                {t('Forget')}
              </Button>
            </div>
          </div>
        ))}
      </div>
    )
  }

  return (
    <>
      <Card data-card-hover='false'>
        <CardHeader>
          <CardTitle>{t('Trusted devices')}</CardTitle>
          <CardDescription>
            {t(
              'Devices that can request SMS login codes without the image challenge.'
            )}
          </CardDescription>
          <CardAction>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={devices.length === 0 || revokeAllMutation.isPending}
              onClick={() => setConfirmAll(true)}
            >
              {t('Forget all')}
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent>{content}</CardContent>
      </Card>

      <ConfirmDialog
        open={Boolean(revokeTarget)}
        onOpenChange={(open) => !open && setRevokeTarget(null)}
        title={t('Forget this trusted device?')}
        desc={t(
          'This device will have to pass the image challenge again before requesting a login code.'
        )}
        confirmText={t('Forget')}
        destructive
        isLoading={revokeMutation.isPending}
        handleConfirm={() => {
          if (revokeTarget) {
            revokeMutation.mutate(revokeTarget.id)
          }
        }}
      />

      <ConfirmDialog
        open={confirmAll}
        onOpenChange={setConfirmAll}
        title={t('Forget all trusted devices?')}
        desc={t(
          'Every device will have to pass the image challenge again before requesting a login code.'
        )}
        confirmText={t('Forget all')}
        destructive
        isLoading={revokeAllMutation.isPending}
        handleConfirm={() => revokeAllMutation.mutate()}
      />
    </>
  )
}
