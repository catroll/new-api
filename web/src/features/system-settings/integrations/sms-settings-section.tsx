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
import { Loader2, Send } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { PasswordInput } from '@/components/password-input'
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
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  SettingsControlGroup,
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'
import {
  createSmsSchema,
  smsEditableFields,
  smsOptionKey,
  type SmsConfiguration,
  type SmsFormValues,
} from './sms-settings-schema'

type SmsSettingsSectionProps = {
  defaultValues: SmsConfiguration
}

type SmsTestResponse = {
  success: boolean
  message: string
  data?: {
    sms_ids?: string[] | null
    status_code?: number
  }
}

// Trim text fields and drop trailing slashes so an unchanged value is not
// written back as a new option.
function normalizeSmsConfiguration(values: SmsConfiguration): SmsConfiguration {
  return {
    ...values,
    api_base: values.api_base.trim().replace(/\/+$/, ''),
    sms_user: values.sms_user.trim(),
    sms_key: values.sms_key.trim(),
    sign_name: values.sign_name.trim(),
    template_param_key: values.template_param_key.trim(),
    allowed_country_codes: values.allowed_country_codes.trim(),
  }
}

export function SmsSettingsSection({ defaultValues }: SmsSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const smsSchema = createSmsSchema(t)

  const initialValues = useMemo(
    () => ({
      sms: {
        ...defaultValues,
        // The API never returns the stored key, so the input always starts
        // empty and a blank value keeps the existing one.
        sms_key: '',
      },
    }),
    [defaultValues]
  )

  const form = useForm<SmsFormValues>({
    resolver: zodResolver(smsSchema),
    defaultValues: initialValues,
  })

  const [provider, apiMode, smsKeyConfigured, msgType] = useWatch({
    control: form.control,
    name: ['sms.provider', 'sms.api', 'sms.sms_key_configured', 'sms.msg_type'],
  })
  const isSaving = form.formState.isSubmitting || updateOption.isPending
  useResetForm(form, isSaving ? undefined : initialValues)

  const [testPhone, setTestPhone] = useState('')
  const [isTesting, setIsTesting] = useState(false)

  const onSubmit = async (values: SmsFormValues) => {
    const next = normalizeSmsConfiguration(values.sms)
    const baseline = normalizeSmsConfiguration(initialValues.sms)
    const updates: Array<{ key: string; value: string | number | boolean }> = []

    for (const field of smsEditableFields) {
      if (field === 'enabled') continue
      if (next[field] === baseline[field]) continue
      updates.push({ key: smsOptionKey(field), value: next[field] })
    }

    if (next.sms_key) {
      updates.push({ key: smsOptionKey('sms_key'), value: next.sms_key })
    }

    // Enable last: the server refuses to switch SMS on while the credentials
    // it needs are still missing.
    if (next.enabled !== baseline.enabled) {
      updates.push({ key: smsOptionKey('enabled'), value: next.enabled })
    }

    try {
      for (const update of updates) {
        await updateOption.mutateAsync(update)
      }
      if (next.sms_key) {
        form.setValue('sms.sms_key', '')
        form.setValue('sms.sms_key_configured', true)
      }
    } catch (error) {
      handleServerError(error)
    }
  }

  const sendTestSms = async () => {
    const phone = testPhone.trim()
    if (!phone || isTesting) return
    setIsTesting(true)
    try {
      const response = await api.post<SmsTestResponse>('/api/option/sms/test', {
        phone,
      })
      const result = requireServerSuccess(response.data)
      const smsIds = result.data?.sms_ids ?? []
      toast.success(t('Test SMS sent'), {
        description:
          smsIds.length > 0
            ? `${t('Message ids')}: ${smsIds.join(', ')}`
            : undefined,
      })
    } catch (error) {
      handleServerError(error)
    } finally {
      setIsTesting(false)
    }
  }

  return (
    <SettingsSection title={t('SMS')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={isSaving}
          />

          <FormField
            control={form.control}
            name='sms.enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable SMS')}</FormLabel>
                  <FormDescription>
                    {t('Send verification codes and notifications by SMS')}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={isSaving}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <SettingsControlGroup>
            <FormField
              control={form.control}
              name='sms.login_enabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Allow SMS code sign-in')}</FormLabel>
                    <FormDescription>
                      {t('Users can sign in with a code sent to their phone')}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      disabled={isSaving}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />

            <FormField
              control={form.control}
              name='sms.require_phone'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>
                      {t('Require a phone number at registration')}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'Registration requires a phone number and a dismissible reminder is shown after login. This cannot be combined with email verification.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      disabled={isSaving}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
          </SettingsControlGroup>

          <FormField
            control={form.control}
            name='sms.provider'
            render={({ field }) => (
              <FormItem data-settings-form-span='full'>
                <FormLabel>{t('SMS provider')}</FormLabel>
                <FormControl>
                  <RadioGroup
                    value={field.value}
                    onValueChange={field.onChange}
                    disabled={isSaving}
                    className='grid-cols-1 gap-3 sm:grid-cols-2'
                  >
                    <Label
                      className='flex cursor-pointer items-center gap-3 rounded-lg border p-4'
                      htmlFor='sms-provider-sendcloud'
                    >
                      <RadioGroupItem
                        value='sendcloud'
                        id='sms-provider-sendcloud'
                      />
                      {t('SendCloud SMS')}
                    </Label>
                    <Label
                      className='flex cursor-pointer items-center gap-3 rounded-lg border p-4'
                      htmlFor='sms-provider-console'
                    >
                      <RadioGroupItem
                        value='console'
                        id='sms-provider-console'
                      />
                      {t('Console (debug only)')}
                    </Label>
                  </RadioGroup>
                </FormControl>
                <FormDescription>
                  {provider === 'console'
                    ? t(
                        'The console provider logs each message instead of sending it, so credentials and signature are ignored.'
                      )
                    : t('Only the selected provider sends SMS.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          {provider === 'sendcloud' && (
            <>
              <div className='grid gap-6 md:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='sms.sms_user'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('SendCloud SMS user')}</FormLabel>
                      <FormControl>
                        <Input
                          autoComplete='off'
                          placeholder='sms_user'
                          {...field}
                          onChange={(event) =>
                            field.onChange(event.target.value)
                          }
                          disabled={isSaving}
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Account used to authenticate with the SMS provider'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='sms.sms_key'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('SMS key')}</FormLabel>
                      <FormControl>
                        <PasswordInput
                          autoComplete='new-password'
                          placeholder={t('Enter new key to update')}
                          {...field}
                          disabled={isSaving}
                        />
                      </FormControl>
                      <FormDescription>
                        {smsKeyConfigured
                          ? t(
                              'A key is configured. Leave this blank to keep the current key.'
                            )
                          : t('Leave blank to keep the existing key')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='sms.sign_id'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Signature ID')}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          min={0}
                          step={1}
                          {...safeNumberFieldProps(field)}
                          disabled={isSaving}
                        />
                      </FormControl>
                      <FormDescription>
                        {t('0 means the signature name is used instead')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='sms.sign_name'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Signature name')}</FormLabel>
                      <FormControl>
                        <Input
                          autoComplete='off'
                          placeholder='New API'
                          {...field}
                          onChange={(event) =>
                            field.onChange(event.target.value)
                          }
                          disabled={isSaving}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <FormField
                control={form.control}
                name='sms.api'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('SMS API mode')}</FormLabel>
                    <FormControl>
                      <Select
                        items={[
                          {
                            value: 'sendCode',
                            label: t('Send verification code'),
                          },
                          {
                            value: 'send',
                            label: t('Send template message'),
                          },
                        ]}
                        value={field.value}
                        onValueChange={field.onChange}
                        disabled={isSaving}
                      >
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent alignItemWithTrigger={false}>
                          <SelectGroup>
                            <SelectItem value='sendCode'>
                              {t('Send verification code')}
                            </SelectItem>
                            <SelectItem value='send'>
                              {t('Send template message')}
                            </SelectItem>
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                    </FormControl>
                    <FormDescription>
                      {t(
                        'sendCode uses the built-in code template; send uses your own template.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {apiMode === 'send' && (
                <>
                  <FormField
                    control={form.control}
                    name='sms.template_id'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Template ID')}</FormLabel>
                        <FormControl>
                          <Input
                            type='number'
                            min={1}
                            step={1}
                            {...safeNumberFieldProps(field)}
                            disabled={isSaving}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='sms.template_param_key'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Template parameter key')}</FormLabel>
                        <FormControl>
                          <Input
                            autoComplete='off'
                            placeholder='code'
                            {...field}
                            onChange={(event) =>
                              field.onChange(event.target.value)
                            }
                            disabled={isSaving}
                          />
                        </FormControl>
                        <FormDescription>
                          {t(
                            'Template variable that carries the verification code'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </>
              )}

              <FormField
                control={form.control}
                name='sms.api_base'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('SMS API base URL')}</FormLabel>
                    <FormControl>
                      <Input
                        autoComplete='off'
                        placeholder='https://api.sendcloud.net/smsapi'
                        {...field}
                        onChange={(event) => field.onChange(event.target.value)}
                        disabled={isSaving}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Base URL of the provider API. Trailing slashes are removed automatically.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormItem>
                <FormLabel>{t('Message type')}</FormLabel>
                <FormControl>
                  <RadioGroup
                    value={String(msgType)}
                    onValueChange={(value) =>
                      form.setValue('sms.msg_type', value === '2' ? 2 : 0, {
                        shouldDirty: true,
                      })
                    }
                    disabled={isSaving}
                    className='gap-3'
                  >
                    <div className='flex items-center gap-2'>
                      <RadioGroupItem value='0' id='sms-msg-type-domestic' />
                      <Label
                        htmlFor='sms-msg-type-domestic'
                        className='cursor-pointer font-normal'
                      >
                        {t('Domestic (0)')}
                      </Label>
                    </div>
                    <div className='flex items-center gap-2'>
                      <RadioGroupItem
                        value='2'
                        id='sms-msg-type-international'
                      />
                      <Label
                        htmlFor='sms-msg-type-international'
                        className='cursor-pointer font-normal'
                      >
                        {t('International (2)')}
                      </Label>
                    </div>
                  </RadioGroup>
                </FormControl>
              </FormItem>
            </>
          )}

          <FormField
            control={form.control}
            name='sms.allowed_country_codes'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Allowed country codes')}</FormLabel>
                <FormControl>
                  <Input
                    autoComplete='off'
                    placeholder='+86'
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                    disabled={isSaving}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Comma-separated calling codes, for example +86,+1. Leave blank to allow every code.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className='grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3'>
            <FormField
              control={form.control}
              name='sms.code_ttl_seconds'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Code validity (seconds)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={isSaving}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('How long a verification code stays valid')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='sms.resend_seconds'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Resend cooldown (seconds)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={isSaving}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Minimum wait before a code can be requested again')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='sms.max_attempts'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Maximum attempts per code')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={isSaving}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Wrong entries allowed before a code is invalidated')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='sms.phone_daily_limit'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Daily limit per phone')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={isSaving}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Codes sent to one phone per day')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='sms.ip_hourly_limit'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Hourly limit per IP')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={isSaving}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Codes requested from one IP per hour')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='sms.daily_quota'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Daily quota')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={isSaving}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Messages the whole site may send per day')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div
            data-settings-form-span='full'
            className='min-w-0 space-y-2 rounded-xl border px-3 py-2.5'
          >
            <Label htmlFor='sms-test-phone'>{t('Send test SMS')}</Label>
            <div className='flex flex-col gap-2 sm:flex-row'>
              <Input
                id='sms-test-phone'
                autoComplete='off'
                inputMode='tel'
                placeholder='+8613800138000'
                value={testPhone}
                onChange={(event) => setTestPhone(event.target.value)}
                disabled={isTesting}
                className='sm:max-w-64'
              />
              <Button
                type='button'
                variant='outline'
                onClick={sendTestSms}
                disabled={isTesting || !testPhone.trim()}
              >
                {isTesting ? (
                  <Loader2 data-icon='inline-start' className='animate-spin' />
                ) : (
                  <Send data-icon='inline-start' />
                )}
                <span>{t('Send test SMS')}</span>
              </Button>
            </div>
            <p className='text-muted-foreground text-xs'>
              {t(
                'A real message is sent with the saved configuration, so save your changes first.'
              )}
            </p>
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
