import type { NextConfig } from "next";

function backendOrigin() {
  const raw = (process.env.CODELOCAL_BACKEND_URL || "http://127.0.0.1:3333").trim();
  const url = new URL(raw);
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error("CODELOCAL_BACKEND_URL must use http or https");
  }
  url.pathname = "";
  url.search = "";
  url.hash = "";
  return url.toString().replace(/\/$/, "");
}

const nextConfig: NextConfig = {
  output: "standalone",
  turbopack: {
    // This repository intentionally has a separate root lockfile for the Go/npm
    // distribution. Keep Next.js module resolution and file watching scoped to
    // the browser product instead of inferring the monorepo root.
    root: process.cwd(),
  },
  async rewrites() {
    const backend = backendOrigin();
    return {
      beforeFiles: [
        {
          // Browser requests keep their same-origin URL/cookies while Next
          // proxies the explicit versioned API family to the Go authority.
          source: "/api/v1/:path*",
          destination: `${backend}/api/v1/:path*`,
        },
        {
          // Collective preferences are an existing Go-owned mutation/read
          // contract used by the Knowledge page during direct web canaries.
          source: "/api/collective/:path*",
          destination: `${backend}/api/collective/:path*`,
        },
      ],
    };
  },
};

export default nextConfig;
