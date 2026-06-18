import { RuleTester } from 'eslint';

import rule from '../require-cast-comment.js';

RuleTester.describe = describe;
RuleTester.it = it;

const tester = new RuleTester({
  languageOptions: {
    parser: (await import('@typescript-eslint/parser')).default,
  },
});

tester.run('require-cast-comment', rule, {
  valid: [
    // as const — safe, no comment needed
    { code: 'const x = [] as const;' },

    // as unknown — widening, no comment needed
    { code: 'const x = foo as unknown;' },

    // Comment on the same line
    { code: 'const x = foo as Bar; // cast: Bar is the only concrete type here' },

    // Comment on the line above
    {
      code: `// cast: narrowing from union — checked by caller
const x = foo as Bar;`,
    },

    // Double assertion: inner is unknown (safe), outer has comment
    { code: 'const x = (foo as unknown as Bar); // cast: required for generic override' },

    // as const on object literal
    { code: 'const cfg = { a: 1 } as const;' },
  ],

  invalid: [
    // Missing comment — simple assertion
    {
      code: 'const x = foo as Bar;',
      errors: [{ messageId: 'missingCastComment', data: { type: 'Bar' } }],
    },

    // Missing comment — generic type
    {
      code: 'const x = foo as Array<string>;',
      errors: [{ messageId: 'missingCastComment', data: { type: 'Array<string>' } }],
    },

    // Missing comment — inline in expression
    {
      code: 'doSomething(value as SpecificType);',
      errors: [{ messageId: 'missingCastComment', data: { type: 'SpecificType' } }],
    },

    // Unrelated comment on preceding line doesn't count
    {
      code: `// not a cast comment
const x = foo as Bar;`,
      errors: [{ messageId: 'missingCastComment', data: { type: 'Bar' } }],
    },

    // Comment too far above (blank line separates)
    {
      code: `// cast: reason

const x = foo as Bar;`,
      errors: [{ messageId: 'missingCastComment', data: { type: 'Bar' } }],
    },
  ],
});
