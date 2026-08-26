import type { SVGProps } from "react";

export type AppIconName =
  | "chat"
  | "brain"
  | "connection"
  | "folder"
  | "device"
  | "invite"
  | "user"
  | "admin"
  | "trash"
  | "download"
  | "paperclip"
  | "send"
  | "search"
  | "chevron-down"
  | "chevron-left"
  | "chevron-right"
  | "close"
  | "plus"
  | "minus"
  | "target"
  | "code"
  | "file"
  | "module"
  | "memory"
  | "skill"
  | "database"
  | "runtime"
  | "thanh-giong"
  | "shield"
  | "refresh";

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
      {name === "chat" && <><path d="M7.5 8.5h9M7.5 12.5h6.2" /><path d="M20.25 11.5a8.25 8.25 0 0 1-8.25 8.25H9.7L5.25 22v-3.35A8.25 8.25 0 1 1 20.25 11.5Z" /></>}
      {name === "brain" && <><path d="M9.35 4.25A3.6 3.6 0 0 0 4.1 7.45c0 .75.22 1.48.63 2.08A3.8 3.8 0 0 0 6.1 16.9 3.65 3.65 0 0 0 12 19.2V6.35a3.05 3.05 0 0 0-2.65-2.1Z" /><path d="M14.65 4.25a3.6 3.6 0 0 1 5.25 3.2c0 .75-.22 1.48-.63 2.08a3.8 3.8 0 0 1-1.37 7.37A3.65 3.65 0 0 1 12 19.2V6.35a3.05 3.05 0 0 1 2.65-2.1Z" /><path d="M7.8 9.25H12m-5.1 4.4H12m4.2-4.4H12m5.1 4.4H12" /></>}
      {name === "connection" && <><path d="m9.45 14.55-1.8 1.8a3.55 3.55 0 0 1-5.02-5.02l3.05-3.05A3.55 3.55 0 0 1 10.7 8" /><path d="m14.55 9.45 1.8-1.8a3.55 3.55 0 1 1 5.02 5.02l-3.05 3.05A3.55 3.55 0 0 1 13.3 16" /><path d="m8.6 15.4 6.8-6.8" /></>}
      {name === "folder" && <><path d="M3.25 7.25A2.25 2.25 0 0 1 5.5 5h4.1l2.15 2.4h6.75a2.25 2.25 0 0 1 2.25 2.25v7.1A2.25 2.25 0 0 1 18.5 19H5.5a2.25 2.25 0 0 1-2.25-2.25Z" /><path d="M3.5 9h17" /></>}
      {name === "device" && <><rect x="3.25" y="4.25" width="17.5" height="12.25" rx="2.1" /><path d="M8.25 20h7.5M12 16.5V20" /></>}
      {name === "invite" && <><circle cx="8.75" cy="8" r="3.5" /><path d="M2.75 20.25a6 6 0 0 1 12 0M18.5 8.25v6.5m-3.25-3.25h6.5" /></>}
      {name === "user" && <><circle cx="12" cy="8" r="3.75" /><path d="M5.25 20.25a6.75 6.75 0 0 1 13.5 0" /></>}
      {name === "admin" && <><path d="M12 3.25 4.4 7.1v4.75c0 4.7 3.13 7.7 7.6 8.9 4.47-1.2 7.6-4.2 7.6-8.9V7.1Z" /><path d="M9 12h6m-3-3v6" /></>}
      {name === "trash" && <><path d="M4.5 7.25h15M9 7.25V4.5h6v2.75m-8 0 .85 12.25h8.3L17 7.25M10 10.75v5.5m4-5.5v5.5" /></>}
      {name === "download" && <><path d="M12 3.75v10.5m0 0 4-4m-4 4-4-4" /><path d="M5 17.25v2h14v-2" /></>}
      {name === "paperclip" && <path d="m8.25 12.5 6.7-6.7a3.05 3.05 0 0 1 4.3 4.3l-8.2 8.2a5 5 0 1 1-7.1-7.05l7.85-7.85" />}
      {name === "send" && <><path d="m5 12 7-7 7 7" /><path d="M12 5v14" /></>}
      {name === "search" && <><circle cx="10.75" cy="10.75" r="6" /><path d="m15.25 15.25 4.5 4.5" /></>}
      {name === "chevron-down" && <path d="m7.25 9.5 4.75 4.75 4.75-4.75" />}
      {name === "chevron-left" && <path d="m14.75 6.5-5.5 5.5 5.5 5.5" />}
      {name === "chevron-right" && <path d="m9.25 6.5 5.5 5.5-5.5 5.5" />}
      {name === "close" && <path d="m7 7 10 10M17 7 7 17" />}
      {name === "plus" && <path d="M12 5.5v13M5.5 12h13" />}
      {name === "minus" && <path d="M5.5 12h13" />}
      {name === "target" && <><circle cx="12" cy="12" r="6.75" /><circle cx="12" cy="12" r="2.25" /><path d="M12 2.75v2M12 19.25v2M2.75 12h2M19.25 12h2" /></>}
      {name === "code" && <><path d="m8.5 6-5 6 5 6M15.5 6l5 6-5 6" /><path d="m13.5 4.5-3 15" /></>}
      {name === "file" && <><path d="M6.25 3.5h7l4.5 4.5v12.5H6.25Z" /><path d="M13.25 3.5V8h4.5" /></>}
      {name === "module" && <><path d="m12 3.5 7.25 4.1v8.8L12 20.5l-7.25-4.1V7.6Z" /><path d="m4.75 7.6 7.25 4.15 7.25-4.15M12 11.75v8.75" /></>}
      {name === "memory" && <><path d="M7 7.25h10A2.75 2.75 0 0 1 19.75 10v4A2.75 2.75 0 0 1 17 16.75H7A2.75 2.75 0 0 1 4.25 14v-4A2.75 2.75 0 0 1 7 7.25Z" /><path d="M8 4.25v3M12 4.25v3M16 4.25v3M8 16.75v3M12 16.75v3M16 16.75v3" /></>}
      {name === "skill" && <><path d="m12 3.5 1.85 4.7 4.65 1.85-4.65 1.85L12 16.6l-1.85-4.7-4.65-1.85 4.65-1.85Z" /><path d="m18.5 15.5.85 2.15 2.15.85-2.15.85-.85 2.15-.85-2.15-2.15-.85 2.15-.85Z" /></>}
      {name === "database" && <><ellipse cx="12" cy="6" rx="7.5" ry="3" /><path d="M4.5 6v6c0 1.65 3.35 3 7.5 3s7.5-1.35 7.5-3V6M4.5 12v6c0 1.65 3.35 3 7.5 3s7.5-1.35 7.5-3v-6" /></>}
      {name === "runtime" && <><rect x="4" y="4" width="16" height="16" rx="4" /><path d="m9 8.5 6 3.5-6 3.5Z" /></>}
      {name === "thanh-giong" && <><path d="M12 2.75 5.25 6.3v5.15c0 4.25 2.7 7.2 6.75 8.8 4.05-1.6 6.75-4.55 6.75-8.8V6.3Z" /><path d="M15.7 8.35a4.65 4.65 0 1 0 .1 6.55" /><path d="M12 6v6h4.6" /></>}
      {name === "shield" && <><path d="M12 3.25 4.5 7v5c0 4.4 2.95 7.35 7.5 8.75 4.55-1.4 7.5-4.35 7.5-8.75V7Z" /><path d="m9.2 12 1.8 1.8 3.9-4" /></>}
      {name === "refresh" && <><path d="M19.5 7.8V3.75l-2.2 2.2A7.5 7.5 0 1 0 19 14.7" /><path d="M15.5 7.8h4V3.75" /></>}
    </svg>
  );
}
