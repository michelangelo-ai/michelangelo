/**
 * @fileoverview Disallows non-function exports in __fixtures__ files.
 *
 * Fixture files must export factory functions, not plain values. A shared
 * constant gives every test the same object reference — mutations in one
 * test bleed into others. A factory function returns a fresh copy per call.
 *
 * @see testing-standards skill — "Anti-Patterns" section
 */

/** @type {import('eslint').Rule.RuleModule} */
const rule = {
  meta: {
    type: 'suggestion',
    docs: {
      description: 'Disallow non-function exports in fixture files',
      recommended: true,
    },
    messages: {
      noFixtureConstant: [
        "'{{ name }}' exports a plain value. Fixture files must only export factory functions.",
        'Wrap it in a factory so each test gets a fresh copy:',
        '',
        '  export function create{{ name }}(overrides = {}) {',
        '    return { ...defaults, ...overrides };',
        '  }',
        '',
        'If this export is intentional, suppress with:',
        '  // eslint-disable-next-line local/no-fixture-constants',
      ].join('\n'),
    },
    schema: [],
  },

  create(context) {
    function isFunction(node) {
      if (!node) return false;
      return node.type === 'ArrowFunctionExpression' || node.type === 'FunctionExpression';
    }

    return {
      ExportNamedDeclaration(node) {
        const { declaration } = node;
        if (!declaration || declaration.type !== 'VariableDeclaration') return;

        for (const declarator of declaration.declarations) {
          const { init, id } = declarator;
          const name = id.type === 'Identifier' ? id.name : '<destructured>';

          if (!isFunction(init)) {
            context.report({
              node: declarator,
              messageId: 'noFixtureConstant',
              data: { name },
            });
          }
        }
      },
    };
  },
};

export default rule;
