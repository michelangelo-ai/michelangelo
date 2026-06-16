import { RuleTester } from 'eslint';

import rule from '../filename-matches-export.js';

RuleTester.describe = describe;
RuleTester.it = it;

const tester = new RuleTester({
  languageOptions: {
    ecmaVersion: 2020,
    sourceType: 'module',
    parserOptions: {
      ecmaFeatures: { jsx: true },
    },
  },
});

tester.run('filename-matches-export', rule, {
  valid: [
    {
      name: 'filename stem matches function export',
      filename: 'button-group.tsx',
      code: `export function ButtonGroup() { return null; }`,
    },
    {
      name: 'filename stem matches const arrow export',
      filename: 'form-control.tsx',
      code: `export const FormControl = () => null;`,
    },
    {
      name: 'index.tsx is always skipped',
      filename: 'index.tsx',
      code: `export function Anything() { return null; }`,
    },
    {
      name: 'styled-components.tsx is skipped',
      filename: 'styled-components.tsx',
      code: `export const Foo = () => null; export const Bar = () => null;`,
    },
    {
      name: 'file with only lowercase exports is exempt (utility)',
      filename: 'helpers.tsx',
      code: `export function formatDate() { return ''; }`,
    },
    {
      name: 'file with only ALL_CAPS exports is exempt (constants)',
      filename: 'icons.tsx',
      code: `export const ICONS = {};`,
    },
    {
      name: '.ts file is ignored',
      filename: 'button-group.ts',
      code: `export function ButtonGroup() {}`,
    },
    {
      name: 'file exports expected name alongside others',
      filename: 'table-pagination.tsx',
      code: `export function TablePagination() { return null; } export function LoadingButton() { return null; }`,
    },
  ],

  invalid: [
    {
      name: 'function export name does not match filename stem',
      filename: 'button-group.tsx',
      code: `export function BtnGroup() { return null; }`,
      errors: [{ messageId: 'filenameMismatch' }],
    },
    {
      name: 'const export name does not match filename stem',
      filename: 'form-control.tsx',
      code: `export const FormCtrl = () => null;`,
      errors: [{ messageId: 'filenameMismatch' }],
    },
    {
      name: 'export matches a different PascalCase name but not the expected one',
      filename: 'theme-provider.tsx',
      code: `export function OtherProvider() { return null; }`,
      errors: [{ messageId: 'filenameMismatch' }],
    },
  ],
});
