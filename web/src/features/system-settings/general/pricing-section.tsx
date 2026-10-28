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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useState } from 'react'
import type { Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { DEFAULT_CURRENCY_CONFIG } from '@/stores/system-config-store'

import { refreshExchangeRate } from '../api'
import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'

const createPricingSchema = (t: (key: string) => string) =>
  z
    .object({
      QuotaPerUnit: z.coerce.number().min(0, t('Value must be at least 0')),
      USDExchangeRate: z.coerce
        .number()
        .min(0.0001, t('Exchange rate must be greater than 0')),
      PaymentCurrency: z.enum(['CNY', 'USD']),
      PriceFollowsExchangeRate: z.boolean(),
      ExchangeRateRefreshMinutes: z.coerce
        .number()
        .int()
        .min(0, t('Value must be at least 0')),
      ExchangeRateApiUrl: z.string().max(512),
      DisplayInCurrencyEnabled: z.boolean(),
      DisplayTokenStatEnabled: z.boolean(),
      general_setting: z.object({
        quota_display_type: z.enum(['USD', 'CNY', 'TOKENS', 'CUSTOM']),
        custom_currency_symbol: z.string().max(8).optional(),
        custom_currency_exchange_rate: z.coerce
          .number()
          .min(0.0001, t('Exchange rate must be greater than 0'))
          .optional(),
      }),
    })
    .superRefine((data, ctx) => {
      const displayType = data.general_setting.quota_display_type

      if (displayType === 'CUSTOM') {
        if (!data.general_setting.custom_currency_symbol?.trim()) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            path: ['general_setting', 'custom_currency_symbol'],
            message: t('Custom currency symbol is required'),
          })
        }

        if (data.general_setting.custom_currency_exchange_rate == null) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            path: ['general_setting', 'custom_currency_exchange_rate'],
            message: t('Exchange rate is required'),
          })
        }
      }
    })

type PricingFormValues = z.infer<ReturnType<typeof createPricingSchema>>

type PricingSectionProps = {
  defaultValues: PricingFormValues
}

export function PricingSection({ defaultValues }: PricingSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const pricingSchema = createPricingSchema(t)

  const { form, handleSubmit, handleReset, isDirty, isSubmitting } =
    useSettingsForm<PricingFormValues>({
      resolver: zodResolver(pricingSchema) as Resolver<
        PricingFormValues,
        unknown,
        PricingFormValues
      >,
      defaultValues,
      onSubmit: async (_data, changedFields) => {
        for (const [key, value] of Object.entries(changedFields)) {
          if (value === undefined || value === null) continue
          if (typeof value === 'object') continue

          let serialized: string | boolean = value as string | boolean

          if (typeof value === 'boolean') {
            serialized = String(value)
          } else if (typeof value === 'number') {
            serialized = Number.isFinite(value) ? String(value) : '0'
          }

          await updateOption.mutateAsync({
            key,
            value: serialized,
          })
        }
      },
    })

  const queryClient = useQueryClient()
  const [rateUpdatedAt, setRateUpdatedAt] = useState(
    defaultValues.ExchangeRateUpdatedAt ?? 0
  )

  // The refresh writes USDExchangeRate on the server, so the input that shows it
  // has to move with it. Leaving the stale value in the form is not merely
  // cosmetic: the field would stay dirty and the next save would write the old
  // rate back over the fresh one.
  const refreshRate = useMutation({
    mutationFn: refreshExchangeRate,
    onSuccess: (data) => {
      if (!data.success) {
        toast.error(data.message || t('Refresh failed'))
        return
      }
      const rate = data.data?.rate
      if (typeof rate === 'number') {
        form.setValue('USDExchangeRate', rate, { shouldDirty: false })
        setRateUpdatedAt(data.data?.updated_at ?? Math.floor(Date.now() / 1000))
        // Everything outside this page reads the rate from /api/status.
        queryClient.invalidateQueries({ queryKey: ['status'] })
      }
      toast.success(t('Rate updated to {{rate}}', { rate: rate ?? '-' }))
    },
    onError: () => toast.error(t('Refresh failed')),
  })

  // Refreshing uses the saved endpoint, so an unsaved address would silently
  // fetch from the old one.
  const endpointDirty = Boolean(form.formState.dirtyFields.ExchangeRateApiUrl)

  const displayType = form.watch('general_setting.quota_display_type') ?? 'USD'
  const showTokensOnlyOption = displayType === 'TOKENS'
  const showQuotaPerUnit =
    displayType === 'TOKENS' ||
    defaultValues.QuotaPerUnit !== DEFAULT_CURRENCY_CONFIG.quotaPerUnit
  // Hidden while it was true, which made the toggle a one-way door: this is the
  // only place that edits DisplayInCurrencyEnabled.
  const showDisplayInCurrencyOption = true

  return (
    <>
      <FormNavigationGuard when={isDirty} />

      {/* The sidebar calls this page "Currency & Display"; the section title used
          to say "Pricing & Display", so the same page had two names. */}
      <SettingsSection title={t('Currency & Display')}>
        <Form {...form}>
          <SettingsForm onSubmit={handleSubmit}>
            <SettingsPageFormActions
              onSave={handleSubmit}
              onReset={handleReset}
              isSaving={updateOption.isPending || isSubmitting}
              isResetDisabled={!isDirty}
            />
            <FormDirtyIndicator isDirty={isDirty} />
            {showQuotaPerUnit && (
              <FormField
                control={form.control}
                name='QuotaPerUnit'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Quota Per Unit')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        step='0.01'
                        value={field.value as number}
                        disabled
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Number of tokens per unit quota')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            <FormField
              control={form.control}
              name='general_setting.quota_display_type'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Display Mode')}</FormLabel>
                  <Select
                    items={[
                      { value: 'USD', label: t('USD') },
                      { value: 'CNY', label: t('CNY') },
                      { value: 'CUSTOM', label: t('Custom Currency') },
                      { value: 'TOKENS', label: t('Tokens Only') },
                    ]}
                    value={field.value}
                    onValueChange={field.onChange}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue placeholder={t('Select display mode')} />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        <SelectItem value='USD'>{t('USD')}</SelectItem>
                        <SelectItem value='CNY'>{t('CNY')}</SelectItem>
                        <SelectItem value='CUSTOM'>
                          {t('Custom Currency')}
                        </SelectItem>
                        {showTokensOnlyOption && (
                          <SelectItem value='TOKENS'>
                            {t('Tokens Only')}
                          </SelectItem>
                        )}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    {t('Choose how quota values are shown to users')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            {displayType !== 'TOKENS' && (
              <FormField
                control={form.control}
                name='USDExchangeRate'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {displayType === 'CNY'
                        ? t('CNY per USD')
                        : t('USD Exchange Rate')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        step='0.01'
                        {...safeNumberFieldProps(field)}
                      />
                    </FormControl>
                    <FormDescription>
                      {displayType === 'CNY'
                        ? t(
                            'One unit of quota is shown as this many yuan. The rate job accepts 3.0-15.0.'
                          )
                        : t(
                            'Not used to convert prices in this display mode; it still feeds billing exports and the recharge-price view.'
                          )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            <div className='grid gap-4 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='ExchangeRateRefreshMinutes'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('Rate Refresh Interval (minutes)')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        step='1'
                        min='0'
                        {...safeNumberFieldProps(field)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        '0 disables the job. Enabling it takes effect without a restart.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='ExchangeRateApiUrl'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Rate Endpoint')}</FormLabel>
                    <FormControl>
                      <Input
                        type='text'
                        value={field.value ?? ''}
                        onChange={field.onChange}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                        placeholder='https://open.er-api.com/v6/latest/USD'
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Must answer with a CNY rate, for example {"rates":{"CNY":7.2}}.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <div className='flex flex-wrap items-center gap-3 pt-1'>
              <Button
                type='button'
                variant='outline'
                size='sm'
                disabled={refreshRate.isPending || endpointDirty}
                onClick={() => refreshRate.mutate()}
              >
                <RefreshCw
                  className={refreshRate.isPending ? 'animate-spin' : ''}
                />
                {t('Refresh now')}
              </Button>
              <span className='text-muted-foreground text-xs'>
                {endpointDirty
                  ? t('Save the endpoint first: a refresh uses the saved one.')
                  : t('Fetches the endpoint and writes this field.')}
              </span>
              <span className='text-muted-foreground ml-auto text-xs tabular-nums'>
                {rateUpdatedAt > 0
                  ? t('Last updated: {{time}}', {
                      time: new Date(rateUpdatedAt * 1000).toLocaleString(),
                    })
                  : t('Never updated')}
              </span>
            </div>

            {displayType === 'CUSTOM' && (
              <div className='grid gap-4 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='general_setting.custom_currency_symbol'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Custom Currency Symbol')}</FormLabel>
                      <FormControl>
                        <Input
                          type='text'
                          value={field.value ?? ''}
                          onChange={field.onChange}
                          name={field.name}
                          onBlur={field.onBlur}
                          ref={field.ref}
                          maxLength={8}
                          placeholder={t('e.g. ¥ or HK$')}
                        />
                      </FormControl>
                      <FormDescription>
                        {t('Prefix used when displaying prices')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='general_setting.custom_currency_exchange_rate'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Units per USD')}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          step='0.01'
                          value={field.value ?? ''}
                          onChange={(e) =>
                            field.onChange(
                              e.target.value === ''
                                ? undefined
                                : e.target.valueAsNumber
                            )
                          }
                          name={field.name}
                          onBlur={field.onBlur}
                          ref={field.ref}
                          placeholder={t('e.g. 8 means 1 USD = 8 units')}
                        />
                      </FormControl>
                      <FormDescription>
                        {t('Conversion rate from USD to your custom currency')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            )}

            {showDisplayInCurrencyOption && (
              <FormField
                control={form.control}
                name='DisplayInCurrencyEnabled'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Display in Currency')}</FormLabel>
                      <FormDescription>
                        {displayType === 'TOKENS'
                          ? t(
                              'Tokens-only mode will show raw quota values regardless of this toggle.'
                            )
                          : t('Show prices in currency instead of quota.')}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />
            )}

            <FormField
              control={form.control}
              name='DisplayTokenStatEnabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Display Token Statistics')}</FormLabel>
                    <FormDescription>
                      {t('Show token usage statistics in the UI')}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
            <FormField
              control={form.control}
              name='PaymentCurrency'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Payment Currency')}</FormLabel>
                  <Select
                    items={[
                      { value: 'CNY', label: t('CNY') },
                      { value: 'USD', label: t('USD') },
                    ]}
                    value={field.value}
                    onValueChange={field.onChange}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue
                          placeholder={t('Select payment currency')}
                        />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        <SelectItem value='CNY'>{t('CNY')}</SelectItem>
                        <SelectItem value='USD'>{t('USD')}</SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    {t(
                      'Currency the gateways charge. Independent of the display currency: a dollar-priced catalogue can still collect CNY.'
                    )}
                  </FormDescription>
                  {field.value === 'USD' && (
                    <p className='text-xs text-amber-600 dark:text-amber-500'>
                      {t(
                        'Cross-border acquiring is not enabled yet, so Alipay refuses every order while this is USD.'
                      )}
                    </p>
                  )}
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='PriceFollowsExchangeRate'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>
                      {t('Price follows the exchange rate')}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'Overwrite Price with each refresh, so the CNY charge tracks the market. Leave off on a CNY-native site, where one unit is one yuan.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
          </SettingsForm>
        </Form>
      </SettingsSection>
    </>
  )
}
