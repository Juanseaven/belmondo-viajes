/** @type {import('next').NextConfig} */
const nextConfig = {
  // Requerido por la etapa final del Dockerfile, que copia .next/standalone.
  output: "standalone",
  reactStrictMode: true,
};

module.exports = nextConfig;
