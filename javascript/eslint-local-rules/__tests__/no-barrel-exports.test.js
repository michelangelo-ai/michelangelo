import { RuleTester } from 'eslint';

import rule from '../no-barrel-exports.js';

RuleTester.describe = describe;
RuleTester.it = it;

const tester = new RuleTester({
  languageOptions: {
    ecmaVersion: 2020,
    sourceType: 'module',
  },
});

tester.run('no-barrel-exports', rule, {
  valid: [
    {
      name: 'index.ts with no exports',
      filename: 'index.ts',
      code: `const x = 1;`,
    },
    {
      name: 'index.ts with local declaration export',
      filename: 'index.ts',
      code: `export const Foo = 'bar';`,
    },
    {
      name: 'index.ts with local function export',
      filename: 'index.ts',
      code: `export function hello() { return 'world'; }`,
    },
    {
      name: 'non-index file with export-all re-export',
      filename: 'utils.ts',
      code: `export * from './foo';`,
    },
    {
      name: 'non-index file with named re-export',
      filename: 'utils.ts',
      code: `export { Foo } from './foo';`,
    },
  ],

  invalid: [
    {
      name: 'index.ts with export-all re-export',
      filename: 'index.ts',
      code: `export * from './foo';`,
      errors: [{ messageId: 'noBarrelExport' }],
    },
    {
      name: 'index.ts with named re-export',
      filename: 'index.ts',
      code: `export { Foo } from './foo';`,
      errors: [{ messageId: 'noBarrelExport' }],
    },
    {
      name: 'index.tsx with export-all re-export',
      filename: 'index.tsx',
      code: `export * from './components';`,
      errors: [{ messageId: 'noBarrelExport' }],
    },
    {
      name: 'index.ts with multiple re-exports',
      filename: 'index.ts',
      code: `export * from './foo';\nexport { Bar } from './bar';`,
      errors: [{ messageId: 'noBarrelExport' }, { messageId: 'noBarrelExport' }],
    },
  ],
});
