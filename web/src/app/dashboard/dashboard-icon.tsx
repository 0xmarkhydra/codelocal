type IconName = "link" | "copy" | "info" | "book" | "plug" | "check" | "grid" | "monitor" | "moon" | "circle" | "graph" | "shield" | "bolt" | "tokens" | "calls" | "activity" | "layers" | "code" | "arrow";

export function DashboardIcon({ name, size = 18 }: { name: IconName; size?: number }) {
  const common = { width: size, height: size, viewBox: "0 0 24 24", fill: "none", stroke: "currentColor", strokeWidth: 1.8, strokeLinecap: "round" as const, strokeLinejoin: "round" as const, "aria-hidden": true };
  switch (name) {
    case "link": return <svg {...common}><path d="M10 13a5 5 0 0 0 7.1.1l2-2a5 5 0 0 0-7.1-7.1l-1.1 1.1"/><path d="M14 11a5 5 0 0 0-7.1-.1l-2 2A5 5 0 0 0 12 20l1.1-1.1"/></svg>;
    case "copy": return <svg {...common}><rect x="8" y="8" width="11" height="11" rx="2"/><path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2"/></svg>;
    case "info": return <svg {...common}><circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/></svg>;
    case "book": return <svg {...common}><path d="M4 5.5A3.5 3.5 0 0 1 7.5 2H11v17H7.5A3.5 3.5 0 0 0 4 22zM20 5.5A3.5 3.5 0 0 0 16.5 2H13v17h3.5A3.5 3.5 0 0 1 20 22z"/></svg>;
    case "plug": return <svg {...common}><path d="M9 3v5m6-5v5M7 8h10v2a5 5 0 0 1-5 5v6M9 21h6"/></svg>;
    case "check": return <svg {...common}><circle cx="12" cy="12" r="9"/><path d="m8 12 2.5 2.5L16 9"/></svg>;
    case "grid": return <svg {...common}><rect x="4" y="4" width="6" height="6" rx="1"/><rect x="14" y="4" width="6" height="6" rx="1"/><rect x="4" y="14" width="6" height="6" rx="1"/><rect x="14" y="14" width="6" height="6" rx="1"/></svg>;
    case "monitor": return <svg {...common}><rect x="3" y="4" width="18" height="13" rx="2"/><path d="M8 21h8m-4-4v4"/></svg>;
    case "moon": return <svg {...common}><path d="M20 15.2A8.5 8.5 0 1 1 8.8 4a7 7 0 0 0 11.2 11.2z"/></svg>;
    case "circle": return <svg {...common}><circle cx="12" cy="12" r="8"/></svg>;
    case "graph": return <svg {...common}><circle cx="5" cy="12" r="2"/><circle cx="12" cy="5" r="2"/><circle cx="19" cy="12" r="2"/><circle cx="12" cy="19" r="2"/><path d="m6.5 10.5 4-4m3 0 4 4m0 3-4 4m-3 0-4-4"/></svg>;
    case "shield": return <svg {...common}><path d="M12 3 4 7v5c0 5 3.4 8.1 8 9 4.6-.9 8-4 8-9V7z"/><path d="m9 12 2 2 4-4"/></svg>;
    case "bolt": return <svg {...common}><path d="m13 2-8 12h7l-1 8 8-12h-7z"/></svg>;
    case "tokens": return <svg {...common}><path d="M12 3 5 7l7 4 7-4-7-4Z"/><path d="m5 12 7 4 7-4M5 17l7 4 7-4"/></svg>;
    case "calls": return <svg {...common}><path d="M7 4h4l2 5-3 2a14 14 0 0 0 3 3l2-3 5 2v4c0 2-1 3-3 3C9 20 4 15 4 7c0-2 1-3 3-3Z"/></svg>;
    case "activity": return <svg {...common}><path d="M3 12h4l2-6 4 12 2-6h6"/></svg>;
    case "layers": return <svg {...common}><path d="m12 3 8 4-8 4-8-4 8-4Z"/><path d="m4 12 8 4 8-4M4 17l8 4 8-4"/></svg>;
    case "code": return <svg {...common}><path d="m8 8-4 4 4 4m8-8 4 4-4 4m-3-10-2 12"/></svg>;
    case "arrow": return <svg {...common}><path d="M5 12h14m-5-5 5 5-5 5"/></svg>;
  }
}
