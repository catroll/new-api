# An always-rendered empty state collides with `getByText('—')` elsewhere in the same row

Signal: `quota-display.test.tsx` fails with

```
TestingLibraryElementError: Found multiple elements with the text: —
  at within(row).getByText('—')
```

after the user table gained a contact column that always renders and shows `—`
when an account has no phone or email. The row already had an `—` from the
invitation cell, which the layout test asserted as unique.

## Check

When adding a column or cell that renders for every row:

1. Grep the feature's tests for placeholder assertions inside a row:
   `grep -rn "getByText('—')" web/src/features`.
2. Either give the test fixture a value for the new cell, or scope the
   assertion to the column under test.
3. Do not weaken the assertion to `getAllByText('—')` or delete it; the test is
   asserting that the invitation details are absent, and that still matters.

For the 2026-11-16 contact column the fix was adding
`phone: '+8613800138000'` to the `renderUsersList` fixture in
`web/src/features/users/components/__tests__/quota-display.test.tsx`, leaving
the invitation `—` assertion intact.
