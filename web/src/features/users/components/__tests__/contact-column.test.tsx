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
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { cleanup, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, it } from 'vitest'

import type { User } from '../../types'
import { useUsersColumns } from '../users-columns'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  initAsync: false,
})

const user = {
  id: 7,
  username: 'test',
  display_name: '',
  role: 1,
  status: 1,
  quota: 0,
  used_quota: 0,
  request_count: 0,
  group: 'default',
  phone: '+8613800138000',
  email: 'user@example.com',
} as User

function ContactTable(props: { row?: User }) {
  const columns = useUsersColumns().filter((column) => column.id === 'contact')
  const table = useReactTable({
    columns,
    data: [props.row ?? user],
    getCoreRowModel: getCoreRowModel(),
  })

  return (
    <table>
      <thead>
        {table.getHeaderGroups().map((group) => (
          <tr key={group.id}>
            {group.headers.map((header) => (
              <th key={header.id}>
                {flexRender(
                  header.column.columnDef.header,
                  header.getContext()
                )}
              </th>
            ))}
          </tr>
        ))}
      </thead>
      <tbody>
        {table.getRowModel().rows.map((row) => (
          <tr key={row.id}>
            {row.getVisibleCells().map((cell) => (
              <td key={cell.id}>
                {flexRender(cell.column.columnDef.cell, cell.getContext())}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function renderContact(row?: User) {
  render(
    <I18nextProvider i18n={i18n}>
      <ContactTable row={row} />
    </I18nextProvider>
  )
}

afterEach(cleanup)

it('always renders the contact column', () => {
  renderContact()
  expect(
    screen.getByRole('columnheader', { name: 'Contact' })
  ).toBeInTheDocument()
})

it('shows both the phone number and the email when both are bound', () => {
  renderContact()
  expect(screen.getByText('+8613800138000')).toBeInTheDocument()
  expect(screen.getByText('user@example.com')).toBeInTheDocument()
})

it('shows only the phone number when no email is bound', () => {
  renderContact({ ...user, email: '' } as User)
  expect(screen.getByText('+8613800138000')).toBeInTheDocument()
  expect(screen.queryByText('user@example.com')).not.toBeInTheDocument()
})

it('shows only the email when no phone is bound', () => {
  renderContact({ ...user, phone: '' } as User)
  expect(screen.getByText('user@example.com')).toBeInTheDocument()
  expect(screen.queryByText('+8613800138000')).not.toBeInTheDocument()
})

it('renders a dash when the account has no contact binding', () => {
  renderContact({ ...user, phone: '', email: '' } as User)
  expect(screen.getByText('—')).toBeInTheDocument()
})
