import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "CodeLocal",
    short_name: "CodeLocal",
    description: "AI coding workspace connected to your authorized local runtime.",
    start_url: "/dashboard",
    scope: "/",
    display: "standalone",
    background_color: "#07101a",
    theme_color: "#07101a",
    orientation: "any",
    icons: [{ src: "/codelocal-icon.png", sizes: "any", type: "image/png", purpose: "any maskable" }],
  };
}
