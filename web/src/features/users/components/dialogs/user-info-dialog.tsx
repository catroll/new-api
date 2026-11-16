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
import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Label } from '@/components/ui/label'
import { formatQuota, formatCompactNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { statusQueryOptions } from '@/lib/status-query'

import { getUser } from '../../api'
import type { User } from '../../types'

interface UserInfoDialogProps {
  userId: number | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

function InfoItem({ label, value }: { label: string; value: string | number }) {
  return (
    <div className='space-y-1.5'>
      <Label className='text-muted-foreground text-xs'>{label}</Label>
      <div className='text-sm font-semibold break-words'>{value}</div>
    </div>
  )
}

/**
 * Read-only user snapshot shared by the usage logs and the user list.
 *
 * The phone and email rows follow the features that can bind them: the SMS
 * flow for phones and email verification for addresses. Both rows stay hidden
 * when the feature is off, so the dialog does not advertise fields the site
 * cannot populate.
 */
export function UserInfoDialog({
  userId,
  open,
  onOpenChange,
}: UserInfoDialogProps) {
  const { t } = useTranslation()
  const [userInfo, setUserInfo] = useState<User | null>(null)
  const [isLoading, setIsLoading] = useState(false)
  const { data: status } = useQuery({
    ...statusQueryOptions,
    enabled: open && !!userId,
  })
  const showPhone = Boolean(status?.sms_enabled)
  const showEmail = Boolean(status?.email_verification)

  const fetchUserInfo = useCallback(
    async (id: number) => {
      setIsLoading(true)
      try {
        const result = await getUser(id)
        if (result.success) {
          setUserInfo(result.data || null)
        } else {
          handleServerError(result, t('Failed to fetch user information'))
        }
      } catch (error) {
        handleServerError(error, t('Failed to fetch user information'))
      } finally {
        setIsLoading(false)
      }
    },
    [t]
  )

  useEffect(() => {
    if (open && userId) {
      fetchUserInfo(userId)
    }
  }, [open, userId, fetchUserInfo])

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('User Information')}
      description={t(
        'View detailed information about this user including balance, usage statistics, and invitation details.'
      )}
      contentClassName='sm:max-w-lg'
      contentHeight='auto'
      bodyClassName='space-y-4'
    >
      {isLoading && (
        <div className='flex items-center justify-center py-8'>
          <Loader2 className='text-muted-foreground size-6 animate-spin' />
        </div>
      )}
      {!isLoading && userInfo && (
        <div className='space-y-4 py-4'>
          {/* Basic Info */}
          <div className='grid grid-cols-2 gap-4'>
            <InfoItem label={t('Username')} value={userInfo.username} />
            {userInfo.display_name && (
              <InfoItem
                label={t('Display Name')}
                value={userInfo.display_name}
              />
            )}
            {showPhone && userInfo.phone && (
              <InfoItem label={t('Phone number')} value={userInfo.phone} />
            )}
            {showEmail && userInfo.email && (
              <InfoItem label={t('Email')} value={userInfo.email} />
            )}
          </div>

          {/* Balance Info */}
          <div className='grid grid-cols-2 gap-4'>
            <InfoItem
              label={t('Balance')}
              value={formatQuota(userInfo.quota)}
            />
            <InfoItem
              label={t('Used Quota')}
              value={formatQuota(userInfo.used_quota)}
            />
          </div>

          {/* Statistics */}
          <div className='grid grid-cols-2 gap-4'>
            <InfoItem
              label={t('Request Count')}
              value={formatCompactNumber(userInfo.request_count)}
            />
            {userInfo.group && (
              <InfoItem label={t('User Group')} value={userInfo.group} />
            )}
          </div>

          {/* Invitation Info */}
          {(userInfo.aff_code ||
            userInfo.aff_count !== undefined ||
            (userInfo.aff_quota !== undefined && userInfo.aff_quota > 0)) && (
            <>
              <div className='grid grid-cols-2 gap-4'>
                {userInfo.aff_code && (
                  <InfoItem
                    label={t('Invitation Code')}
                    value={userInfo.aff_code}
                  />
                )}
                {userInfo.aff_count !== undefined && (
                  <InfoItem
                    label={t('Invited Users')}
                    value={formatCompactNumber(userInfo.aff_count)}
                  />
                )}
              </div>

              {userInfo.aff_quota !== undefined && userInfo.aff_quota > 0 && (
                <InfoItem
                  label={t('Invitation Quota')}
                  value={formatQuota(userInfo.aff_quota)}
                />
              )}
            </>
          )}

          {/* Remark */}
          {userInfo.remark && (
            <div className='space-y-1.5'>
              <Label className='text-muted-foreground text-xs'>
                {t('Remark')}
              </Label>
              <div className='text-sm leading-relaxed font-semibold break-words'>
                {userInfo.remark}
              </div>
            </div>
          )}
        </div>
      )}
      {!isLoading && !userInfo && (
        <div className='text-muted-foreground py-8 text-center text-sm'>
          {t('No user information available')}
        </div>
      )}
    </Dialog>
  )
}
