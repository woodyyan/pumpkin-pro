/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  swcMinify: true,

  // ── Test-file exclusion for webpack builds ────────────────────────
  //
  // Test files under lib/__tests__/ and components/__tests__/ import
  // from 'node:test' / 'node:assert/strict' which webpack 5 cannot
  // resolve ("UnhandledSchemeError").  Since no production code imports
  // test files, we safely map those protocols to empty modules so that
  // webpack's static analysis pass doesn't choke on them.
  webpack: (config) => {
    config.resolve.alias = {
      ...config.resolve.alias,
      // Map node: protocol imports to safe fallbacks
      'node:assert/strict': require.resolve('assert'),
      'node:test': false,
    };
    return config;
  },
};

module.exports = nextConfig;
