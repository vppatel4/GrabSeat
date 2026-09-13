/** @type {import('next').NextConfig} */
const nextConfig = {
  // Standalone output keeps the production Docker image tiny — Next bundles only
  // the files the server actually needs.
  output: "standalone",
  reactStrictMode: true,
};

module.exports = nextConfig;
