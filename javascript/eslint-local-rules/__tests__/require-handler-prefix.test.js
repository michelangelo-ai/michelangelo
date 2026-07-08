import { RuleTester } from 'eslint';

import rule from '../require-handler-prefix.js';

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

tester.run('require-handler-prefix', rule, {
  valid: [
    {
      name: 'handle* prefix — basic valid case',
      code: `const handleMenuOpen = () => {}; const C = () => <Button onClick={handleMenuOpen} />;`,
    },
    {
      name: 'passthrough from parameter destructuring',
      code: `const C = ({ onClick }) => <Button onClick={onClick} />;`,
    },
    {
      name: 'passthrough to a different prop name',
      code: `const C = ({ onClose }) => <Modal onDismiss={onClose} />;`,
    },
    {
      name: 'aliased destructuring is still a parameter — exempt',
      code: `const C = ({ onClose: handleClose }) => <Modal onDismiss={handleClose} />;`,
    },
    {
      name: 'inline function — not an identifier, not checked',
      code: `const C = () => <Button onClick={() => setOpen(true)} />;`,
    },
    {
      name: 'non-event prop is ignored',
      code: `const toggleMenu = () => {}; const C = () => <Button label={toggleMenu} />;`,
    },
    {
      name: 'passthrough via nullish coalescing (props ?? {})',
      code: `function getWrapper(props) { const { onSubmit = () => {} } = props ?? {}; return <Form onSubmit={onSubmit} />; }`,
    },
    {
      name: 'indirect passthrough via props object (forwardRef pattern)',
      code: `const C = forwardRef((props, ref) => { const { onClose } = props; return <Modal onEsc={onClose} />; });`,
    },
  ],

  invalid: [
    {
      name: 'verb-named handler without handle* prefix',
      code: `const toggleMenu = () => {}; const C = () => <Button onClick={toggleMenu} />;`,
      errors: [{ messageId: 'requireHandlerPrefix' }],
    },
    {
      name: 'action-named handler without handle* prefix',
      code: `const submitForm = () => {}; const C = () => <Form onSubmit={submitForm} />;`,
      errors: [{ messageId: 'requireHandlerPrefix' }],
    },
    {
      name: 'on*-named locally-defined handler',
      code: `const onSubmit = () => {}; const C = () => <Form onSubmit={onSubmit} />;`,
      errors: [{ messageId: 'requireHandlerPrefix' }],
    },
  ],
});
