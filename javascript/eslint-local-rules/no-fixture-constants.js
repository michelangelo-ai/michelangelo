/**
 * @fileoverview Disallows non-function exports in __fixtures__ files.
 *
 * Constants in fixture files could be shared across tests. This accumulates
 * invisible shared state across tests, making failures harder to reason about.
 * Imported fixture constants also make reasoning about expectations more difficult,
 * since there is redirection between the fixture and the test.
 *
 * Inline everything inside each test instead.
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
        "'{{ name }}' exports a plain value. Fixture files must only export functions. For",
        ' values that are shared across tests, use a factory function.',
        '',
        '  export function create{{ name }}(overrides = {}) {',
        '    return { ...defaults, ...overrides };',
        '  }',
        '',
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
