module.exports = {
  root: true,
  env: {
    node: true,
    browser: true,
    es2021: true,
    'vue/setup-compiler-macros': true,
  },
  extends: [
    'plugin:vue/vue3-essential',
    'eslint:recommended',
    'plugin:prettier/recommended',
  ],
  parserOptions: {
    parser: '@babel/eslint-parser',
    requireConfigFile: false,
    ecmaVersion: 2020,
    sourceType: 'module',
    babelOptions: {
      plugins: ['@vue/babel-plugin-jsx'],
    },
  },
  rules: {
    'no-console': process.env.NODE_ENV === 'production' ? 'warn' : 'off',
    'no-debugger': process.env.NODE_ENV === 'production' ? 'warn' : 'off',
    'space-before-function-paren': 'off',
    'vue/multi-word-component-names': 'off',
    'prettier/prettier': 'off',
    'no-unused-vars': 'off',
    'no-empty': 'off',
    'no-useless-escape': 'off',
    'no-restricted-imports': [
      'error',
      {
        patterns: [
          {
            group: [
              'element-plus',
              'element-plus/*',
              '@element-plus/*',
              'bootstrap',
              'bootstrap/*',
              '@tabler/core',
              '@tabler/core/*',
            ],
            message:
              'Legacy UI libraries are frozen; use TDesign (tdesign-vue-next / tdesign-icons-vue-next) in new code.',
          },
        ],
      },
    ],
    'no-restricted-syntax': [
      'error',
      {
        selector:
          'ImportExpression[source.value=/^(element-plus(\\x2f.*)?|@element-plus\\x2f.*|bootstrap(\\x2f.*)?|@tabler\\x2fcore(\\x2f.*)?)$/]',
        message:
          'Legacy UI libraries are frozen; use TDesign (tdesign-vue-next / tdesign-icons-vue-next) in new code.',
      },
      {
        selector:
          'ImportExpression[source.type="TemplateLiteral"][source.quasis.0.value.cooked=/^(element-plus(\\x2f.*)?|@element-plus\\x2f.*|bootstrap(\\x2f.*)?|@tabler\\x2fcore(\\x2f.*)?)$/]',
        message:
          'Legacy UI libraries are frozen; use TDesign (tdesign-vue-next / tdesign-icons-vue-next) in new code.',
      },
      {
        selector:
          'CallExpression[callee.name="require"][arguments.0.value=/^(element-plus(\\x2f.*)?|@element-plus\\x2f.*|bootstrap(\\x2f.*)?|@tabler\\x2fcore(\\x2f.*)?)$/]',
        message:
          'Legacy UI libraries are frozen; use TDesign (tdesign-vue-next / tdesign-icons-vue-next) in new code.',
      },
      {
        selector:
          'CallExpression[callee.name="require"][arguments.0.type="TemplateLiteral"][arguments.0.quasis.0.value.cooked=/^(element-plus(\\x2f.*)?|@element-plus\\x2f.*|bootstrap(\\x2f.*)?|@tabler\\x2fcore(\\x2f.*)?)$/]',
        message:
          'Legacy UI libraries are frozen; use TDesign (tdesign-vue-next / tdesign-icons-vue-next) in new code.',
      },
    ],
  },
  overrides: [
    {
      files: [
        'src/App.vue',
        'src/components/Confirm/index.vue',
        'src/components/LangSelect/index.vue',
        'src/layout/index.vue',
        'src/plugins/element.js',
        'src/utils/request.js',
        'projects/vgpu/views/task/admin/top.vue',
      ],
      rules: {
        'no-restricted-imports': 'off',
      },
    },
  ],
  globals: {
    globals: 'readonly',
    // Vue 3.3+ compiler macros, not covered by the installed
    // eslint-plugin-vue's `vue/setup-compiler-macros` env.
    defineOptions: 'readonly',
    defineModel: 'readonly',
    defineSlots: 'readonly',
  },
};
