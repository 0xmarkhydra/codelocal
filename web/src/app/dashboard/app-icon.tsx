import type { SVGProps } from "react";

export type AppIconName =
  | "chat"
  | "forum"
  | "brain"
  | "connection"
  | "folder"
  | "device"
  | "usage"
  | "invite"
  | "user"
  | "admin"
  | "trash"
  | "download"
  | "paperclip"
  | "send"
  | "stop"
  | "search"
  | "chevron-down"
  | "chevron-left"
  | "chevron-right"
  | "menu"
  | "close"
  | "plus"
  | "minus"
  | "target"
  | "code"
  | "file"
  | "module"
  | "memory"
  | "skill"
  | "plugin"
  | "database"
  | "runtime"
  | "settings"
  | "edit"
  | "check"
  | "codelocal"
  | "shield"
  | "refresh"
  | "image"
  | "upload"
  | "copy"
  | "terminal"
  | "external";

type Props = Omit<SVGProps<SVGSVGElement>, "name"> & {
  name: AppIconName;
  size?: number;
};

export function AppIcon({ name, size = 18, ...props }: Props) {
  const common = {
    width: size,
    height: size,
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: 1.65,
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
    "aria-hidden": props["aria-label"] ? undefined : true,
  };

  return (
    <svg {...common} {...props}>
      {name === "chat" && <><path d="M5.25 5.25h13.5A2.25 2.25 0 0 1 21 7.5v8a2.25 2.25 0 0 1-2.25 2.25H10l-4.75 3v-3A2.25 2.25 0 0 1 3 15.5v-8a2.25 2.25 0 0 1 2.25-2.25Z" /><path d="M7.5 10h9M7.5 13.5h6" /></>}
      {name === "forum" && <><path d="M4.25 5.25h10.5A2.25 2.25 0 0 1 17 7.5v5.25A2.25 2.25 0 0 1 14.75 15H9l-4.75 3v-3.35A2.25 2.25 0 0 1 2 12.4V7.5a2.25 2.25 0 0 1 2.25-2.25Z" /><path d="M17 9.25h2.25A2.25 2.25 0 0 1 21.5 11.5v4a2.25 2.25 0 0 1-2.25 2.25H18V21l-4.25-3.25H11" /><path d="M6.5 9h6M6.5 12h4" /></>}
      {name === "brain" && <><path d="M9.35 4.25A3.6 3.6 0 0 0 4.1 7.45c0 .75.22 1.48.63 2.08A3.8 3.8 0 0 0 6.1 16.9 3.65 3.65 0 0 0 12 19.2V6.35a3.05 3.05 0 0 0-2.65-2.1Z" /><path d="M14.65 4.25a3.6 3.6 0 0 1 5.25 3.2c0 .75-.22 1.48-.63 2.08a3.8 3.8 0 0 1-1.37 7.37A3.65 3.65 0 0 1 12 19.2V6.35a3.05 3.05 0 0 1 2.65-2.1Z" /><path d="M7.8 9.25H12m-5.1 4.4H12m4.2-4.4H12m5.1 4.4H12" /></>}
      {name === "connection" && <><path d="m9.45 14.55-1.8 1.8a3.55 3.55 0 0 1-5.02-5.02l3.05-3.05A3.55 3.55 0 0 1 10.7 8" /><path d="m14.55 9.45 1.8-1.8a3.55 3.55 0 1 1 5.02 5.02l-3.05 3.05A3.55 3.55 0 0 1 13.3 16" /><path d="m8.6 15.4 6.8-6.8" /></>}
      {name === "folder" && <><path d="M3.25 7.25A2.25 2.25 0 0 1 5.5 5h4.1l2.15 2.4h6.75a2.25 2.25 0 0 1 2.25 2.25v7.1A2.25 2.25 0 0 1 18.5 19H5.5a2.25 2.25 0 0 1-2.25-2.25Z" /><path d="M3.5 9h17" /></>}
      {name === "device" && <><rect x="3.25" y="4.25" width="17.5" height="12.25" rx="2.1" /><path d="M8.25 20h7.5M12 16.5V20" /></>}
      {name === "usage" && <><path d="M4.25 20V10.75h4V20M10 20V4.25h4V20m1.75 0v-7h4v7" /><path d="M2.75 20.25h18.5" /></>}
      {name === "invite" && <><circle cx="8.75" cy="8" r="3.5" /><path d="M2.75 20.25a6 6 0 0 1 12 0M18.5 8.25v6.5m-3.25-3.25h6.5" /></>}
      {name === "user" && <><circle cx="12" cy="8" r="3.75" /><path d="M5.25 20.25a6.75 6.75 0 0 1 13.5 0" /></>}
      {name === "admin" && <><path d="M12 3.25 4.4 7.1v4.75c0 4.7 3.13 7.7 7.6 8.9 4.47-1.2 7.6-4.2 7.6-8.9V7.1Z" /><path d="M9 12h6m-3-3v6" /></>}
      {name === "trash" && <><path d="M4.5 7.25h15M9 7.25V4.5h6v2.75m-8 0 .85 12.25h8.3L17 7.25M10 10.75v5.5m4-5.5v5.5" /></>}
      {name === "download" && <><path d="M12 3.75v10.5m0 0 4-4m-4 4-4-4" /><path d="M5 17.25v2h14v-2" /></>}
      {name === "paperclip" && <path d="m8.25 12.5 6.7-6.7a3.05 3.05 0 0 1 4.3 4.3l-8.2 8.2a5 5 0 1 1-7.1-7.05l7.85-7.85" />}
      {name === "send" && <><path d="m5 12 7-7 7 7" /><path d="M12 5v14" /></>}
      {name === "stop" && <rect x="7" y="7" width="10" height="10" rx="1.5" />}
      {name === "search" && <><circle cx="10.75" cy="10.75" r="6" /><path d="m15.25 15.25 4.5 4.5" /></>}
      {name === "chevron-down" && <path d="m7.25 9.5 4.75 4.75 4.75-4.75" />}
      {name === "chevron-left" && <path d="m14.75 6.5-5.5 5.5 5.5 5.5" />}
      {name === "chevron-right" && <path d="m9.25 6.5 5.5 5.5-5.5 5.5" />}
      {name === "menu" && <path d="M4 7h16M4 12h16M4 17h16" />}
      {name === "close" && <path d="m7 7 10 10M17 7 7 17" />}
      {name === "plus" && <path d="M12 5.5v13M5.5 12h13" />}
      {name === "minus" && <path d="M5.5 12h13" />}
      {name === "target" && <><circle cx="12" cy="12" r="6.75" /><circle cx="12" cy="12" r="2.25" /><path d="M12 2.75v2M12 19.25v2M2.75 12h2M19.25 12h2" /></>}
      {name === "code" && <><path d="m8.5 6-5 6 5 6M15.5 6l5 6-5 6" /><path d="m13.5 4.5-3 15" /></>}
      {name === "file" && <><path d="M6.25 3.5h7l4.5 4.5v12.5H6.25Z" /><path d="M13.25 3.5V8h4.5" /></>}
      {name === "module" && <><path d="m12 3.5 7.25 4.1v8.8L12 20.5l-7.25-4.1V7.6Z" /><path d="m4.75 7.6 7.25 4.15 7.25-4.15M12 11.75v8.75" /></>}
      {name === "memory" && <><path d="M7 7.25h10A2.75 2.75 0 0 1 19.75 10v4A2.75 2.75 0 0 1 17 16.75H7A2.75 2.75 0 0 1 4.25 14v-4A2.75 2.75 0 0 1 7 7.25Z" /><path d="M8 4.25v3M12 4.25v3M16 4.25v3M8 16.75v3M12 16.75v3M16 16.75v3" /></>}
      {name === "skill" && <><path d="m12 3.5 1.85 4.7 4.65 1.85-4.65 1.85L12 16.6l-1.85-4.7-4.65-1.85 4.65-1.85Z" /><path d="m18.5 15.5.85 2.15 2.15.85-2.15.85-.85 2.15-.85-2.15-2.15-.85 2.15-.85Z" /></>}
      {name === "plugin" && <><path d="M9.25 4.25h3.1a2.4 2.4 0 1 1 4.55 1.1v3.4h2.85v3.05a2.4 2.4 0 1 0 0 4.45v3.5h-3.5a2.4 2.4 0 1 0-4.45 0H8.75v-3.1a2.4 2.4 0 1 1-1.1-4.55h-3.4V8.75h3.4a2.4 2.4 0 1 1 1.6-4.5Z" /></>}
      {name === "database" && <><ellipse cx="12" cy="6" rx="7.5" ry="3" /><path d="M4.5 6v6c0 1.65 3.35 3 7.5 3s7.5-1.35 7.5-3V6M4.5 12v6c0 1.65 3.35 3 7.5 3s7.5-1.35 7.5-3v-6" /></>}
      {name === "runtime" && <><rect x="4" y="4" width="16" height="16" rx="4" /><path d="m9 8.5 6 3.5-6 3.5Z" /></>}
      {name === "settings" && <><circle cx="12" cy="12" r="3.25" /><path d="M19.4 13.2a7.8 7.8 0 0 0 0-2.4l2-1.55-2-3.45-2.45 1a8.15 8.15 0 0 0-2.05-1.2L14.55 3h-5.1L9.1 5.6a8.15 8.15 0 0 0-2.05 1.2l-2.45-1-2 3.45 2 1.55a7.8 7.8 0 0 0 0 2.4l-2 1.55 2 3.45 2.45-1a8.15 8.15 0 0 0 2.05 1.2l.35 2.6h5.1l.35-2.6a8.15 8.15 0 0 0 2.05-1.2l2.45 1 2-3.45Z" /></>}
      {name === "edit" && <><path d="m4.5 19.5 3.7-.75L18.8 8.15a2.05 2.05 0 0 0-2.9-2.9L5.3 15.85Z" /><path d="m13.95 7.2 2.85 2.85M4.5 19.5l.8-3.65" /></>}
      {name === "check" && <path d="m5.25 12.25 4.3 4.3 9.2-9.2" />}
      {name === "codelocal" && <><path d="M12 2.7 5.2 6.15v5.1c0 4.25 2.72 7.25 6.8 8.95 4.08-1.7 6.8-4.7 6.8-8.95v-5.1Z" /><path d="M12 6.15v10.7" /><path d="m8.8 10.15 3.2-3 3.2 3" /><path d="M9.2 13.7h5.6" /><path d="M7.65 7.4h8.7" /></>}
      {name === "shield" && <><path d="M12 3.25 4.5 7v5c0 4.4 2.95 7.35 7.5 8.75 4.55-1.4 7.5-4.35 7.5-8.75V7Z" /><path d="m9.2 12 1.8 1.8 3.9-4" /></>}
      {name === "refresh" && <><path d="M19.5 7.8V3.75l-2.2 2.2A7.5 7.5 0 1 0 19 14.7" /><path d="M15.5 7.8h4V3.75" /></>}
      {name === "image" && <><rect x="3.25" y="4.25" width="17.5" height="15.5" rx="2.25" /><circle cx="8.25" cy="9" r="1.6" /><path d="m4.5 17 4.65-4.6 3.2 3 2.15-2.1 5 4.7" /></>}
      {name === "upload" && <><path d="M12 15.25V4.75m0 0-4 4m4-4 4 4" /><path d="M5 17.25v2h14v-2" /></>}
      {name === "copy" && <><rect x="8" y="8" width="11.5" height="11.5" rx="2" /><path d="M16 8V6.5a2 2 0 0 0-2-2H6.5a2 2 0 0 0-2 2V14a2 2 0 0 0 2 2H8" /></>}
      {name === "terminal" && <><polyline points="4 17 10 11 4 5" /><line x1="12" y1="19" x2="20" y2="19" /></>}
      {name === "external" && <><path d="M13 5h6v6M19 5l-8 8" /><path d="M10 7H6.5A1.5 1.5 0 0 0 5 8.5v9A1.5 1.5 0 0 0 6.5 19h9a1.5 1.5 0 0 0 1.5-1.5V14" /></>}
    </svg>
  );
}
