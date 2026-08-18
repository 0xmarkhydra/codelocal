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
          // proxies this explicit versioned API family to the Go authority.
          source: "/api/v1/:path*",
          destination: `${backend}/api/v1/:path*`,
        },
      ],
      fallback: [
        {
          // Incremental migration: any route not yet owned by Next.js remains
          // served by the existing Go application (login/signup/account/etc.).
          source: "/:path*",
          destination: `${backend}/:path*`,
        },
      ],
    };
  },
};

export default nextConfig;
