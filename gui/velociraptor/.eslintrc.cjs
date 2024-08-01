module.exports = {
    env: {
        browser: true,
        node: true,
        es6: true,
     },
    parser: "@babel/eslint-parser",
    parserOptions: {
        requireConfigFile: false,
        babelOptions: {
            presets: ["@babel/preset-react"],
        },
    },
    extends: [
        // By extending from a plugin config, we can get recommended rules without having to add them manually.
        "eslint:recommended",
        "plugin:react/recommended",
        "plugin:import/recommended",
        "plugin:jsx-a11y/recommended",
    ],
    settings: {
        react: {
            // Tells eslint-plugin-react to automatically detect the version of React to use.
            version: "detect",
        },
        // Tells eslint how to resolve imports
        "import/resolver": {
            node: {
                paths: ["src"],
                extensions: [".js", ".jsx"],
            },
        },
    },
    // Add your own rules here to override ones from the extended configs.
    rules: {
        // allows characters like <'`&^"
        "react/no-unescaped-entities": 0,
        "no-extra-semi": 0,
        "react/display-name": 0,
        "no-empty": 0,
        "no-unused-vars": ["error", { "args": "none" }],
        "jsx-a11y/click-events-have-key-events": 0,
        "jsx-a11y/no-noninteractive-element-interactions": 0,
        "jsx-a11y/no-static-element-interactions": 0,
    },
};
