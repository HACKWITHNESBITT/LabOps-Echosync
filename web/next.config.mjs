/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  output: "standalone",
  async rewrites() {
    return [
      {
        source: "/v1/:path*",
        destination: "http://127.0.0.1:8080/v1/:path*",
      },
      {
        source: "/health",
        destination: "http://127.0.0.1:8080/health",
      },
      {
        source: "/metrics",
        destination: "http://127.0.0.1:8080/metrics",
      },
    ];
  },
};

export default nextConfig;