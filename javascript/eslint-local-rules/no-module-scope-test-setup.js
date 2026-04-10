/**
 * @fileoverview Disallows module-level variable declarations used for test setup.
 *
 * Wrappers, props, and component configurations defined at the top of a test
 * file accumulate invisible shared state across tests, making failures harder
 * to reason about. Inline everything inside each test instead.
 */

/** @type {import('eslint').Rule.RuleModule} */
const rule = {
  meta: {
    type: 'suggestion',
    docs: {
      description:
        'Disallow module-level variable declarations for test setup (wrappers, props, options)',
      recommended: true,
    },
    messages: {
      noModuleScopeWrapper: [
        'buildWrapper() must not be called at module scope.',
        'Move it inside each test so every test is self-contained:',
        '',
        '  it("renders", () => {',
        '    render(<Foo />, buildWrapper([getBaseProviderWrapper()]));',
        '  });',
        '',
      ].join('\n'),

      noModuleScopeWrapperHelper: [
        "'{{ name }}' wraps buildWrapper() at module scope.",
        'Move the buildWrapper() call inline into each test so every test is self-contained:',
        '',
        '  it("renders", () => {',
        '    render(<Foo />, buildWrapper([getBaseProviderWrapper()]));',
        '  });',
        '',
      ].join('\n'),

      noModuleScopeSetupConst: [
        "'{{ name }}' is declared at module scope but looks like test setup (props, options, config).",
        'Inline it inside each test so every test is self-contained:',
        '',
        '  it("renders", () => {',
        '    render(<Foo options={[{ value: "a", label: "Option A" }]} />);',
        '  });',
        '',
        'If tests share a precondition, move render() into beforeEach and query with screen:',
        '',
        '  describe("disabled state", () => {',
        '    beforeEach(() => { render(<Foo disabled />); });',
        '    it("shows label", () => { screen.getByText("Disabled"); });',
        '  });',
        '',
        'If this declaration is intentional (e.g. a domain constant shared across tests), suppress with:',
        '  // eslint-disable-next-line local/no-module-scope-test-setup',
      ].join('\n'),
    },
    schema: [],
  },

  create(context) {
    const TEST_HOOKS = new Set(['it', 'test', 'beforeEach', 'afterEach', 'beforeAll', 'afterAll']);

    /**
     * Returns the callee name for a CallExpression, handling both plain
     * identifiers (describe) and member expressions (describe.each).
     */
    function getCalleeName(callExpr) {
      const { callee } = callExpr;
      if (callee.type === 'Identifier') return callee.name;
      if (callee.type === 'MemberExpression' && callee.object?.type === 'Identifier') {
        return callee.object.name;
      }
      return null;
    }

    /**
     * Returns true when a CallExpression with the given callee name has a
     * parent describe — i.e. it's nested, not the outermost file wrapper.
     */
    function hasParentDescribe(callExpr) {
      let current = callExpr.parent;
      while (current) {
        if (current.type === 'Program') return false;
        if (current.type === 'CallExpression') {
          const name = getCalleeName(current);
          if (name === 'describe') return true;
        }
        current = current.parent;
      }
      return false;
    }

    /**
     * Returns true when `node` sits at module scope. The outermost describe()
     * in a file is just a wrapper — variables there are shared across all
     * tests. Nested describes are semantic groups where shared state is the
     * intended pattern.
     *
     * Walk up the AST:
     * - test hook (it/test/beforeEach/…) → inside a test → false
     * - nested describe (has a parent describe) → semantic group → false
     * - top-level describe (no parent describe) → file wrapper → true
     * - Program → module scope → true
     */
    /**
     * Returns true when a function node is a standalone function — not a
     * callback argument to describe/it/beforeEach/etc. Standalone functions
     * (like component definitions) create real scope boundaries.
     */
    function isStandaloneFunction(node) {
      return node.parent?.type !== 'CallExpression';
    }

    function isModuleScope(node) {
      let current = node.parent;
      while (current) {
        if (current.type === 'Program') return true;
        if (
          (current.type === 'FunctionDeclaration' ||
            current.type === 'FunctionExpression' ||
            current.type === 'ArrowFunctionExpression') &&
          isStandaloneFunction(current)
        ) {
          return false;
        }
        if (current.type === 'CallExpression') {
          const name = getCalleeName(current);
          if (name && TEST_HOOKS.has(name)) return false;
          if (name === 'describe') return !hasParentDescribe(current);
        }
        current = current.parent;
      }
      return true;
    }

    /**
     * Returns true when the initializer (or any nested call) invokes buildWrapper.
     */
    function callsBuildWrapper(node) {
      if (!node) return false;
      if (node.type === 'CallExpression') {
        const { callee } = node;
        if (callee.type === 'Identifier' && callee.name === 'buildWrapper') return true;
        // Check arguments recursively in case it's wrapped
        return node.arguments.some(callsBuildWrapper);
      }
      if (node.type === 'ArrayExpression') {
        return node.elements.some(callsBuildWrapper);
      }
      return false;
    }

    function bodyCallsBuildWrapper(node) {
      if (!node) return false;
      if (callsBuildWrapper(node)) return true;
      if (node.type === 'BlockStatement') {
        return node.body.some(bodyCallsBuildWrapper);
      }
      if (node.type === 'ReturnStatement') {
        return bodyCallsBuildWrapper(node.argument);
      }
      if (node.type === 'ExpressionStatement') {
        return bodyCallsBuildWrapper(node.expression);
      }
      return false;
    }

    /**
     * Heuristic: does this initializer look like test setup data?
     * Matches object literals, array literals, and JSX — the common shapes
     * for props / options / config objects.
     */
    function looksLikeSetupData(init) {
      if (!init) return false;
      return (
        init.type === 'ObjectExpression' ||
        init.type === 'ArrayExpression' ||
        init.type === 'JSXElement' ||
        init.type === 'JSXFragment'
      );
    }

    return {
      FunctionDeclaration(node) {
        if (!isModuleScope(node)) return;
        const name = node.id?.name ?? '<anonymous>';
        if (bodyCallsBuildWrapper(node.body)) {
          context.report({
            node,
            messageId: 'noModuleScopeWrapperHelper',
            data: { name },
          });
        }
      },

      VariableDeclaration(node) {
        if (!isModuleScope(node)) return;

        for (const declarator of node.declarations) {
          const { init, id } = declarator;
          const name = id.type === 'Identifier' ? id.name : '<destructured>';

          if (callsBuildWrapper(init)) {
            context.report({
              node: declarator,
              messageId: 'noModuleScopeWrapper',
            });
            continue;
          }

          if (
            init &&
            (init.type === 'ArrowFunctionExpression' || init.type === 'FunctionExpression') &&
            bodyCallsBuildWrapper(init.body)
          ) {
            context.report({
              node: declarator,
              messageId: 'noModuleScopeWrapperHelper',
              data: { name },
            });
            continue;
          }

          if (looksLikeSetupData(init)) {
            context.report({
              node: declarator,
              messageId: 'noModuleScopeSetupConst',
              data: { name },
            });
          }
        }
      },
    };
  },
};

export default rule;
