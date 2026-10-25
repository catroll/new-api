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
import * as z from 'zod'

// Every field name is the Go json tag of the SMSSettings group, so the option
// key is exactly `sms.<field>`.
export const smsConfigurationSchema = z.object({
  enabled: z.boolean(),
  login_enabled: z.boolean(),
  require_phone: z.boolean(),
  provider: z.enum(['sendcloud', 'console']),
  api: z.enum(['sendCode', 'send']),
  api_base: z.string(),
  sms_user: z.string(),
  sms_key: z.string(),
  // Read-only: the option API never returns the stored key, only whether one
  // is configured.
  sms_key_configured: z.boolean(),
  sign_id: z.number().int().min(0),
  sign_name: z.string(),
  template_id: z.number().int().min(0),
  template_param_key: z.string(),
  // 0 = domestic, 2 = international, matching the SendCloud message type.
  msg_type: z.union([z.literal(0), z.literal(2)]),
  allowed_country_codes: z.string(),
  code_ttl_seconds: z.number().int().positive(),
  resend_seconds: z.number().int().positive(),
  max_attempts: z.number().int().positive(),
  phone_daily_limit: z.number().int().positive(),
  ip_hourly_limit: z.number().int().positive(),
  daily_quota: z.number().int().min(0),
})

export type SmsConfiguration = z.infer<typeof smsConfigurationSchema>
export type SmsSettingField = keyof SmsConfiguration

// Fields persisted as their own option key. `sms_key` is saved separately
// (blank keeps the stored key) and `sms_key_configured` is never written.
export const smsEditableFields = [
  'enabled',
  'login_enabled',
  'require_phone',
  'provider',
  'api',
  'api_base',
  'sms_user',
  'sign_id',
  'sign_name',
  'template_id',
  'template_param_key',
  'msg_type',
  'allowed_country_codes',
  'code_ttl_seconds',
  'resend_seconds',
  'max_attempts',
  'phone_daily_limit',
  'ip_hourly_limit',
  'daily_quota',
] as const satisfies readonly SmsSettingField[]

export function smsOptionKey(field: SmsSettingField) {
  return `sms.${field}` as const
}

export const createSmsSchema = (t: (key: string) => string) =>
  z.object({
    sms: smsConfigurationSchema.superRefine((values, context) => {
      if (values.api === 'send' && values.template_id <= 0) {
        context.addIssue({
          code: 'custom',
          path: ['template_id'],
          message: t('A template id is required when the send API is used'),
        })
      }

      const countryCodes = values.allowed_country_codes.trim()
      if (
        countryCodes &&
        !/^(?:\+\d{1,4})(?:,\s*\+\d{1,4})*$/.test(countryCodes)
      ) {
        context.addIssue({
          code: 'custom',
          path: ['allowed_country_codes'],
          message: t(
            'Use comma-separated country calling codes such as +86,+1'
          ),
        })
      }

      // An installation that cannot send must not be reported as enabled. The
      // console provider sends nothing, so it needs no credentials.
      if (!values.enabled || values.provider !== 'sendcloud') return
      if (!values.sms_user.trim()) {
        context.addIssue({
          code: 'custom',
          path: ['sms_user'],
          message: t('Enter the SMS user before enabling SMS'),
        })
      }
      if (!values.sms_key_configured && !values.sms_key.trim()) {
        context.addIssue({
          code: 'custom',
          path: ['sms_key'],
          message: t('Enter the SMS key before enabling SMS'),
        })
      }
      if (values.sign_id <= 0 && !values.sign_name.trim()) {
        context.addIssue({
          code: 'custom',
          path: ['sign_id'],
          message: t(
            'Enter a signature ID or signature name before enabling SMS'
          ),
        })
      }
    }),
  })

export type SmsFormValues = z.infer<ReturnType<typeof createSmsSchema>>
