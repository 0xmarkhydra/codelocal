"use client";

import { useEffect, useId, useRef, useState } from "react";
import { useTranslations } from "@/lib/i18n/provider";
import type { MessageKey } from "@/lib/i18n/messages";
import { AppIcon, type AppIconName } from "../dashboard/app-icon";
import styles from "./architecture-model.module.css";

const stages: {
  title: MessageKey;
  short: MessageKey;
  icon: AppIconName;
  copy: MessageKey;
  tone: string;
}[] = [
  {
    title: "Bring your AI",
    short: "Any compatible client",
    icon: "skill",
    tone: "blue",
    copy: "Connect ChatGPT, Claude, Codex or another compatible AI client through MCP. Your project gets a shared entry point, whichever AI you choose.",
  },
  {
    title: "Connect through CodeLocal",
    short: "One control layer",
    icon: "module",
    tone: "purple",
    copy: "CodeLocal resolves the project, supplies bounded context and routes requests. Workspace authorization and approval policy determine what can run.",
  },
  {
    title: "Run on your machine",
    short: "Local tools. Local control.",
    icon: "terminal",
    tone: "mint",
    copy: "Approved actions run in your local workspace: files, Git, terminal and browser tools. Raw source, secrets and machine-specific indexes stay on your computer.",
  },
  {
    title: "Keep the intelligence",
    short: "Remember. Verify. Reuse.",
    icon: "brain",
    tone: "orange",
    copy: "Project Brain keeps sanitized durable knowledge. Decisions and verified experience become useful context for the next session, without granting new access to your machine.",
  },
];

function Deck({
  x,
  y,
  width,
  height,
  depth = 15,
  paint,
  side = "#291650",
  edge = "#b28be3",
}: {
  x: number;
  y: number;
  width: number;
  height: number;
  depth?: number;
  paint: string;
  side?: string;
  edge?: string;
}) {
  const left = x - width / 2;
  const right = x + width / 2;
  const top = y - height / 2;
  const bottom = y + height / 2;
  return (
    <g strokeLinejoin="round">
      <path
        d={`M${left} ${y} L${x} ${bottom} L${x} ${bottom + depth} L${left} ${y + depth} Z`}
        fill={side}
        stroke={edge}
        strokeOpacity=".2"
      />
      <path
        d={`M${x} ${bottom} L${right} ${y} L${right} ${y + depth} L${x} ${bottom + depth} Z`}
        fill={side}
        stroke={edge}
        strokeOpacity=".25"
      />
      <path
        d={`M${x} ${top} L${right} ${y} L${x} ${bottom} L${left} ${y} Z`}
        fill={paint}
        stroke={edge}
        strokeWidth="1.2"
      />
      <path
        d={`M${left + 9} ${y + 1} L${x} ${bottom - 5} L${right - 9} ${y + 1}`}
        fill="none"
        stroke={edge}
        strokeOpacity=".5"
      />
    </g>
  );
}

export function ArchitectureModel() {
  const { t } = useTranslations();
  const [active, setActive] = useState(1);
  const viewport = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const element = viewport.current;
    if (element)
      element.scrollLeft =
        (550 / 1120) * element.scrollWidth - element.clientWidth / 2;
  }, []);

  function selectStage(index: number) {
    setActive(index);
    const element = viewport.current;
    if (!element) return;
    const center = [182, 550, 924, 550][index];
    element.scrollTo({
      left: (center / 1120) * element.scrollWidth - element.clientWidth / 2,
      behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches
        ? "instant"
        : "smooth",
    });
  }
  const id = useId();
  const fill = (name: string) => `url(#${id}-${name})`;
  const stage = stages[active];

  return (
    <div className={styles.model}>
      <div className={styles.modelBar}>
        <span>
          <i /> {t("The CodeLocal model")}
        </span>
        <span>
          {t("Architecture")} <span aria-hidden="true">↗</span>
        </span>
      </div>
      <div
        ref={viewport}
        className={styles.viewport}
        tabIndex={0}
        role="region"
        aria-label={t("CodeLocal architecture diagram")}
      >
        <svg
          className={styles.diagram}
          viewBox="0 0 1120 600"
          role="img"
          aria-labelledby={`${id}-title ${id}-description`}
        >
          <title id={`${id}-title`}>
            {t("One connected system. Your machine at the center of execution.")}
          </title>
          <desc id={`${id}-description`}>
            {t("AI clients connect by MCP to CodeLocal. CodeLocal routes approved requests to your local machine. Project Brain provides sanitized durable knowledge. Cloud and local execution have separate privacy boundaries.")}
          </desc>
          <defs>
            <clipPath id={`${id}-brand-clip`}>
              <rect width="160" height="160" rx="31" />
            </clipPath>
            <linearGradient id={`${id}-brand-face`} x1="0" y1="0" x2="1" y2="1">
              <stop stopColor="#7183b3" />
              <stop offset=".45" stopColor="#303d60" />
              <stop offset="1" stopColor="#171429" />
            </linearGradient>
            <linearGradient id={`${id}-purple`} x1="0" y1="0" x2="1" y2="1">
              <stop stopColor="#d5b6ff" />
              <stop offset=".46" stopColor="#9660ef" />
              <stop offset="1" stopColor="#482391" />
            </linearGradient>
            <linearGradient id={`${id}-blue`} x1="0" y1="0" x2="1" y2="1">
              <stop stopColor="#b5e5ff" />
              <stop offset=".5" stopColor="#5389da" />
              <stop offset="1" stopColor="#2c3b7d" />
            </linearGradient>
            <linearGradient id={`${id}-mint`} x1="0" y1="0" x2="1" y2="1">
              <stop stopColor="#a0f9db" />
              <stop offset=".5" stopColor="#47b8a1" />
              <stop offset="1" stopColor="#18534e" />
            </linearGradient>
            <linearGradient id={`${id}-orange`} x1="0" y1="0" x2="1" y2="1">
              <stop stopColor="#ffe4b6" />
              <stop offset=".5" stopColor="#df9b70" />
              <stop offset="1" stopColor="#874651" />
            </linearGradient>
            <linearGradient id={`${id}-dark`} x1="0" y1="0" x2="1" y2="1">
              <stop stopColor="#373149" />
              <stop offset="1" stopColor="#181521" />
            </linearGradient>
            <linearGradient id={`${id}-screen`} x1="0" y1="0" x2="1" y2="1">
              <stop stopColor="#1a403c" />
              <stop offset="1" stopColor="#0b1e27" />
            </linearGradient>
            <linearGradient id={`${id}-wire`}>
              <stop stopColor="#70b9fa" />
              <stop offset=".5" stopColor="#c39aff" />
              <stop offset="1" stopColor="#81e4c4" />
            </linearGradient>
            <pattern
              id={`${id}-grid`}
              width="96"
              height="48"
              patternUnits="userSpaceOnUse"
            >
              <path
                d="M0 24 48 0 96 24 48 48Z"
                fill="none"
                stroke="#b9a3d2"
                strokeOpacity=".055"
              />
            </pattern>
            <filter
              id={`${id}-shadow`}
              x="-40%"
              y="-50%"
              width="180%"
              height="220%"
            >
              <feDropShadow
                dx="0"
                dy="18"
                stdDeviation="12"
                floodColor="#03010a"
                floodOpacity=".6"
              />
            </filter>
          </defs>
          <rect width="1120" height="600" fill={fill("grid")} />
          <rect
            x="367"
            y="92"
            width="366"
            height="462"
            rx="28"
            fill="#ab7bff03"
            stroke="#a582d7"
            strokeOpacity=".22"
            strokeDasharray="4 7"
          />
          <foreignObject x="367" y="30" width="366" height="54">
            <div className={styles.zoneLabel}>{t("Cloud: identity, routing, knowledge")}</div>
          </foreignObject>
          <rect
            x="784"
            y="134"
            width="280"
            height="311"
            rx="28"
            fill="#7debc504"
            stroke="#82d8c1"
            strokeOpacity=".22"
            strokeDasharray="4 7"
          />
          <foreignObject x="784" y="77" width="280" height="50">
            <div className={styles.zoneLabel}>{t("Local: your privacy boundary")}</div>
          </foreignObject>
          <g
            className={styles.wires}
            fill="none"
            stroke={fill("wire")}
            strokeWidth="2"
          >
            <path d="M282 292 H333 L362 263 H414" />
            <path d="m402 257 12 6-12 6" />
            <path d="M688 263 H747 L776 292 H817" />
            <path d="m805 286 12 6-12 6" />
            <path d="M550 346 V407" stroke="#d0a2c7" />
            <path d="m544 395 6 12 6-12 M544 358 l6-12 6 12" stroke="#d0a2c7" />
          </g>
          <text
            className={styles.wireLabel}
            x="330"
            y="244"
            textAnchor="middle"
          >
            MCP
          </text>
          <text
            className={styles.wireLabel}
            x="745"
            y="244"
            textAnchor="middle"
          >
            RPC
          </text>
          <text className={styles.wireLabel} x="566" y="385">
            {t("Context")}
          </text>

          <g className={styles.node} data-selected={active === 0}>
            <g filter={fill("shadow")}>
              <Deck
                x={182}
                y={296}
                width={250}
                height={116}
                paint={fill("dark")}
                edge="#7a8dad"
                side="#161c30"
              />
              <Deck
                x={122}
                y={247}
                width={110}
                height={60}
                depth={13}
                paint={fill("blue")}
                side="#23315f"
                edge="#a9d7ff"
              />
              <Deck
                x={221}
                y={259}
                width={110}
                height={60}
                depth={13}
                paint={fill("orange")}
                side="#6c3c39"
                edge="#ffd6a6"
              />
              <Deck
                x={181}
                y={198}
                width={110}
                height={60}
                depth={13}
                paint={fill("purple")}
                edge="#d9c0ff"
              />
              <g className={styles.clientMarks}>
                <g transform="matrix(.9 -.45 .9 .45 106 247)">
                  <AppIcon name="connection" size={25} />
                </g>
                <g transform="matrix(.9 -.45 .9 .45 205 259)">
                  <AppIcon name="skill" size={25} />
                </g>
                <g transform="matrix(.9 -.45 .9 .45 165 198)">
                  <AppIcon name="code" size={25} />
                </g>
              </g>
            </g>
            <text
              className={styles.nodeNumber}
              x="182"
              y="150"
              textAnchor="middle"
              fill="#93baf3"
            >
              01 / AI
            </text>
            <foreignObject x="32" y="365" width="300" height="85">
              <div className={styles.nodeCaption}><strong>{t("Different AIs. One entry.")}</strong><span>ChatGPT · Claude · Codex</span></div>
            </foreignObject>
          </g>

          <g className={styles.node} data-selected={active === 1}>
            <ellipse
              className={styles.selectionRing}
              cx="550"
              cy="299"
              rx="150"
              ry="73"
              fill="none"
              stroke="#b28aee"
              strokeOpacity=".65"
            />
            <g filter={fill("shadow")}>
              <Deck
                x={550}
                y={294}
                width={264}
                height={127}
                paint={fill("dark")}
                edge="#87739f"
              />
              <Deck
                x={550}
                y={269}
                width={242}
                height={116}
                depth={9}
                paint="#9062cb33"
                edge="#b997e6"
              />
              <Deck
                x={550}
                y={235}
                width={224}
                height={110}
                depth={20}
                paint={fill("brand-face")}
                edge="#adb9e5"
                side="#24203e"
              />
              <path
                d="m550 193 91 44-91 44-91-44Z"
                fill="none"
                stroke="#ead7ff"
                strokeOpacity=".4"
              />
              <g transform="matrix(.66 -.325 .66 .325 444.4 235)">
                <image
                  href="/codelocal-icon.png"
                  x="-16"
                  y="-16"
                  width="192"
                  height="192"
                  preserveAspectRatio="xMidYMid meet"
                  clipPath={fill("brand-clip")}
                />
                <rect
                  width="160"
                  height="160"
                  rx="31"
                  fill="none"
                  stroke="#b6c6ff"
                  strokeOpacity=".35"
                  strokeWidth="1"
                />
              </g>
            </g>
            <text
              className={styles.nodeNumber}
              x="550"
              y="130"
              textAnchor="middle"
              fill="#c7a4f5"
            >
              02 / {t("Control layer")}
            </text>
            <text
              className={styles.coreTitle}
              x="550"
              y="161"
              textAnchor="middle"
            >
              CodeLocal
            </text>
          </g>

          <g className={styles.node} data-selected={active === 2}>
            <g filter={fill("shadow")}>
              <Deck
                x={923}
                y={303}
                width={240}
                height={111}
                paint={fill("dark")}
                side="#162e2d"
                edge="#598b82"
              />
              <Deck
                x={923}
                y={289}
                width={190}
                height={89}
                depth={10}
                paint={fill("mint")}
                side="#285e52"
                edge="#b2f4d9"
              />
              <path
                d="M855 188 Q855 179 864 183 L978 224 Q987 227 987 237 V306 L855 257Z"
                fill="#2b665e"
                stroke="#9fead4"
                strokeWidth="2"
              />
              <path
                d="M864 193 978 234 V292 L864 251Z"
                fill={fill("screen")}
                stroke="#82ccb7"
                strokeOpacity=".4"
              />
              <path
                d="m880 213 12 10-12 2 m22 4 23 8"
                fill="none"
                stroke="#9bf1d2"
                strokeWidth="3"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
              <path
                d="m880 240 64 23 m-45-5 47 17"
                stroke="#599f96"
                strokeWidth="2"
              />
              <path
                d="m899 299 20-9 27 10-20 10Z"
                fill="#235b53"
                stroke="#a3e8cd"
                strokeOpacity=".6"
              />
            </g>
            <text
              className={styles.nodeNumber}
              x="924"
              y="168"
              textAnchor="middle"
              fill="#94d9bd"
            >
              03 / {t("Your machine")}
            </text>
            <foreignObject x="794" y="359" width="260" height="60">
              <div className={styles.nodeCaption}><strong>{t("Local power. Your rules.")}</strong><span>{t("Files · Git · Terminal · Browser")}</span></div>
            </foreignObject>
            <path
              d="M915 420v8l8 4 8-4v-8l-8-4Z m4 4 3 3 5-6"
              fill="none"
              stroke="#8fd3b8"
              strokeWidth="1.2"
            />
          </g>

          <g className={styles.node} data-selected={active === 3}>
            <g filter={fill("shadow")}>
              <Deck
                x={550}
                y={485}
                width={245}
                height={78}
                depth={13}
                paint={fill("dark")}
                edge="#a88787"
              />
              <Deck
                x={550}
                y={466}
                width={216}
                height={71}
                depth={11}
                paint={fill("orange")}
                edge="#f3c6aa"
                side="#643a3b"
              />
              <Deck
                x={550}
                y={448}
                width={189}
                height={65}
                depth={11}
                paint={fill("orange")}
                edge="#ffe1bc"
                side="#895044"
              />
              <g fill="none" stroke="#fff0d7" strokeWidth="1.5">
                <path d="m513 448 24-15 31 0 20 15-28 16-30-2Z M537 433l-7 29 M568 433l-8 31 M513 448h75" />
                <circle cx="550" cy="448" r="5" fill="#fff0d7" />
              </g>
            </g>
            <text
              className={styles.nodeNumber}
              x="332"
              y="469"
              textAnchor="end"
              fill="#dfb290"
            >
              04 / PROJECT BRAIN
            </text>
            <foreignObject x="775" y="459" width="305" height="86">
              <div className={`${styles.nodeCaption} ${styles.brainCaption}`}><strong>{t("Intelligence that stays.")}</strong><span>{t("Decisions · Knowledge · Verified experience")}</span></div>
            </foreignObject>
            <path d="M674 478h81" fill="none" stroke="#d9aa9160" />
          </g>
          <foreignObject x="60" y="563" width="1000" height="35">
            <div className={styles.footerLabel}>{t("System model, not live activity")}</div>
          </foreignObject>
        </svg>
      </div>
      <div
        className={styles.controls}
        role="group"
        aria-label={t("Explore each part of the architecture")}
      >
        {stages.map((item, index) => (
          <button
            key={item.title}
            type="button"
            data-tone={item.tone}
            aria-pressed={active === index}
            aria-controls={`${id}-explanation`}
            onClick={() => selectStage(index)}
          >
            <span className={styles.stepIcon}>
              <AppIcon name={item.icon} size={21} />
            </span>
            <span>
              <small>0{index + 1}</small>
              <strong>{t(item.title)}</strong>
              <em>{t(item.short)}</em>
            </span>
            <span className={styles.stepArrow} aria-hidden="true">
              ↗
            </span>
          </button>
        ))}
      </div>
      <div
        className={styles.explanation}
        id={`${id}-explanation`}
        aria-live="polite"
        aria-atomic="true"
      >
        <span data-tone={stage.tone}>
          0{active + 1} / {t(stage.title)}
        </span>
        <p>{t(stage.copy)}</p>
      </div>
    </div>
  );
}
