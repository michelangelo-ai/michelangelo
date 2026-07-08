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

    // Comment on the line above
    {
      code: `// cast: narrowing from union — checked by caller
const x = foo as Bar;`,
    },

    // Multi-line // comment block — marker on the first line
    {
      code: `// cast: narrowing from union
// checked by caller at the call site
const x = foo as Bar;`,
    },

    // Multi-line // comment block — marker on a later line, not the one touching the code
    {
      code: `// this narrows a union down to one member
// cast: checked by caller before this point
const x = foo as Bar;`,
    },

    // Double assertion: inner is unknown (safe), outer has a leading comment
    {
      code: `// cast: required for generic override
const x = (foo as unknown as Bar);`,
    },

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

    // Trailing same-line comment doesn't count — must be a leading block
    {
      code: 'const x = foo as Bar; // cast: Bar is the only concrete type here',
      errors: [{ messageId: 'missingCastComment', data: { type: 'Bar' } }],
    },

    // Unrelated comment on preceding line doesn't count
    {
      code: `// not a cast comment
const x = foo as Bar;`,
      errors: [{ messageId: 'missingCastComment', data: { type: 'Bar' } }],
    },

    // Multi-line comment block with no cast marker anywhere
    {
      code: `// this narrows a union down to one member
// checked by caller before this point
const x = foo as Bar;`,
      errors: [{ messageId: 'missingCastComment', data: { type: 'Bar' } }],
    },

    // Comment too far above (blank line separates)
    {
      code: `// cast: reason

const x = foo as Bar;`,
      errors: [{ messageId: 'missingCastComment', data: { type: 'Bar' } }],
    },

    // Blank line inside what looks like a comment block still breaks the chain
    {
      code: `// cast: reason

// unrelated comment
const x = foo as Bar;`,
      errors: [{ messageId: 'missingCastComment', data: { type: 'Bar' } }],
    },
  ],
});
