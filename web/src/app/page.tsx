"use client";

import Image from "next/image";
import Link from "next/link";
import { useState } from "react";
import styles from "./page.module.css";

const aiClients = ["ChatGPT / Codex", "Claude", "Kimi", "DeepSeek"] as const;
const machineTools = ["Files", "Git", "Terminal", "Browser / Computer"] as const;

const brainTags = ["Rules", "Decisions", "Memory", "Learned skills", "Knowledge graph", "Semantic index"] as const;
const verificationTags = ["Diagnostics", "Tests", "Git diff", "Quality policy"] as const;
const experienceTags = ["Provenance", "Revisions", "Safe promotion"] as const;

const setupSteps = [
  { title: "Install CodeLocal", description: "Install the local runtime on the computer where your project lives.", command: "npm install -g codelocal@latest" },
  { title: "Pair your machine", description: "Sign in once to establish the trusted connection to this computer." },
  { title: "Grant a workspace", description: "Choose exactly which project an AI client may understand and operate on." },
  { title: "Connect your AI", description: "Use one MCP endpoint from ChatGPT, Claude, Codex or another compatible client." },
] as const;

function Signal({ label }: { label: string }) {
  return (
    <div className={styles.signal} aria-hidden="true">
      <span>{label}</span>
      <i className={styles.signalLine}><b /></i>
      <em>→</em>
    </div>
  );
}

function ClientMark({ index }: { index: number }) {
  return <span className={styles.clientMark}>{String(index + 1).padStart(2, "0")}</span>;
}

function NeuralGraph() {
  return (
    <div className={styles.neuralGraph} aria-hidden="true">
      <div className={styles.neuralGlow} />
      <svg className={styles.neuralSvg} viewBox="0 0 760 620" focusable="false">
        <defs>
          <linearGradient id="home-edge-a" x1="0" y1="0" x2="1" y2="1">
            <stop offset="0" stopColor="#32d6ff" />
            <stop offset="0.5" stopColor="#8b5cf6" />
            <stop offset="1" stopColor="#ff4fd8" />
          </linearGradient>
          <linearGradient id="home-edge-b" x1="1" y1="0" x2="0" y2="1">
            <stop offset="0" stopColor="#7cffb2" />
            <stop offset="0.55" stopColor="#37b6ff" />
            <stop offset="1" stopColor="#8b5cf6" />
          </linearGradient>
          <radialGradient id="home-core" cx="50%" cy="50%" r="50%">
            <stop offset="0" stopColor="#ffffff" />
            <stop offset="0.18" stopColor="#9be7ff" />
            <stop offset="0.55" stopColor="#6c70ff" />
            <stop offset="1" stopColor="#9b38ff" stopOpacity="0" />
          </radialGradient>
        </defs>

        <g className={`${styles.neuralLayer} ${styles.neuralLayerA}`}>
          <path d="M89 356 C178 274 214 204 319 182 S505 193 648 102" />
          <path d="M94 356 C176 380 241 443 348 421 S514 347 670 381" />
          <path d="M184 119 C255 205 262 310 361 326 S540 300 626 240" />
          <path d="M153 507 C242 439 269 356 361 326 S519 196 650 177" />
          <path d="M319 182 C357 244 377 263 361 326 S397 430 474 494" />
          <path d="M361 326 C445 329 503 337 574 291" />
        </g>

        <g className={`${styles.neuralLayer} ${styles.neuralLayerB}`}>
          <path d="M184 119 C238 98 294 88 353 102 S470 155 523 123" />
          <path d="M89 356 C145 313 191 303 239 321 S294 344 361 326" />
          <path d="M153 507 C215 484 270 497 326 529 S429 537 474 494" />
          <path d="M523 123 C557 181 578 218 574 291 S609 347 670 381" />
          <path d="M239 321 C246 259 270 216 319 182" />
          <path d="M348 421 C393 456 430 469 474 494" />
        </g>

        <g className={styles.neuralPulses}>
          <circle cx="89" cy="356" r="7" />
          <circle cx="184" cy="119" r="6" />
          <circle cx="153" cy="507" r="6" />
          <circle cx="239" cy="321" r="7" />
          <circle cx="319" cy="182" r="9" />
          <circle cx="348" cy="421" r="7" />
          <circle className={styles.neuralCore} cx="361" cy="326" r="17" />
          <circle cx="474" cy="494" r="8" />
          <circle cx="523" cy="123" r="7" />
          <circle cx="574" cy="291" r="9" />
          <circle cx="626" cy="240" r="6" />
          <circle cx="648" cy="102" r="5" />
          <circle cx="650" cy="177" r="6" />
          <circle cx="670" cy="381" r="7" />
        </g>
      </svg>

      <span className={styles.neuralTag}>Knowledge</span>
      <span className={styles.neuralTag}>Code graph</span>
      <span className={styles.neuralTag}>Skills</span>
      <span className={styles.neuralTag}>Memory</span>
      <span className={styles.neuralTag}>Workflows</span>
      <span className={styles.neuralTag}>Git</span>
      <span className={styles.neuralTag}>LSP</span>
      <span className={styles.neuralTag}>Decisions</span>
      <span className={styles.neuralCaption}>Living project intelligence</span>
    </div>
  );
}

function CopyCommand({ command }: { command: string }) {
  const [copied, setCopied] = useState(false);

  function copy() {
    navigator.clipboard.writeText(command).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <div style={{ display: "inline-flex", alignItems: "center", gap: "8px", marginTop: "10px" }}>
      <code className={styles.setupCommand}>{command}</code>
      <button
        type="button"
        onClick={copy}
        style={{
          minHeight: "28px",
          padding: "0 10px",
          border: "1px solid rgba(120,160,255,0.25)",
          borderRadius: "7px",
          background: "rgba(83,128,239,0.12)",
          color: "#e2ecfa",
          fontSize: "11px",
          cursor: "pointer",
          fontWeight: 550,
          transition: "all 140ms ease"
        }}
      >
        {copied ? "Copied ✓" : "Copy"}
      </button>
    </div>
  );
}

function ArchitectureMap() {
  return (
    <section className={styles.architecture} id="system" aria-label="CodeLocal architecture map">
      <div className={styles.architectureAura} aria-hidden="true" />

      <header className={styles.architectureHead}>
        <div>
          <span className={styles.kicker}>Architecture map</span>
          <h2>AI client → CodeLocal → your machine</h2>
          <p>MCP carries the request. CodeLocal resolves context, enforces policy and executes locally.</p>
        </div>
        <span className={styles.runtimeReady}><i /> Local runtime ready</span>
      </header>

      <div className={styles.flowLegend} aria-label="Request lifecycle">
        <span><b>01</b> Connect</span><i>→</i><span><b>02</b> Govern</span><i>→</i><span><b>03</b> Execute locally</span>
      </div>

      <div className={styles.requestPlane}>
        <article className={`${styles.archCard} ${styles.clientsCard}`}>
          <span className={styles.cardLabel}>AI clients</span>
          <div className={styles.clientRows}>
            {aiClients.map((client, index) => (
              <div key={client}>
                <ClientMark index={index} />
                <strong>{client}</strong>
              </div>
            ))}
          </div>
        </article>

        <Signal label="MCP · OAuth" />

        <article className={`${styles.archCard} ${styles.coreCard}`}>
          <div className={styles.coreHalo} aria-hidden="true" />
          <div className={styles.coreLogo}>
            <Image src="/codelocal-icon.png" alt="" width={44} height={44} priority />
          </div>
          <strong className={styles.coreTitle}>CodeLocal</strong>
          <p>Universal MCP control layer<br />project resolution · bounded context · routing</p>
          <div className={styles.policyStrip}>Approval + security policy<br />outrank automation</div>
        </article>

        <Signal label="Secure RPC · authenticated" />

        <article className={`${styles.archCard} ${styles.machineCard}`}>
          <span className={styles.cardLabel}>Your machine · local runtime</span>
          <div className={styles.machineGrid}>
            {machineTools.map((tool, index) => <span key={tool} data-accent={index < 3 ? "true" : "false"}>{tool}</span>)}
          </div>
          <div className={styles.machineMeta}>
            <span>LSP + code index</span>
            <span>Local approvals</span>
          </div>
        </article>
      </div>

      <div className={styles.intelligencePlane}>
        <article>
          <header><span>01 · Context</span><em>Durable intelligence</em></header>
          <strong>Project Brain</strong>
          <div className={styles.tagCloud}>{brainTags.map((tag) => <span key={tag}>{tag}</span>)}</div>
        </article>
        <article>
          <header><span>02 · Prove</span><em>Evidence</em></header>
          <strong>Verification</strong>
          <div className={`${styles.tagCloud} ${styles.verifyTags}`}>{verificationTags.map((tag) => <span key={tag}>{tag}</span>)}</div>
        </article>
        <article>
          <header><span>03 · Reuse</span><em>Reuse</em></header>
          <strong>Verified experience</strong>
          <div className={`${styles.tagCloud} ${styles.experienceTags}`}>{experienceTags.map((tag) => <span key={tag}>{tag}</span>)}</div>
        </article>
      </div>

      <div className={styles.durableFlow}>
        <span>Local result</span><i>→</i><strong>verify</strong><i>→</i><strong>keep durable knowledge</strong><i>→</i><span>next AI session</span>
      </div>

      <footer className={styles.cloudBoundary}>
        <span className={styles.cloudBadge}>CodeLocal cloud</span>
        <p><strong>Cloud stores:</strong> identity, routing, logical project identity and sanitized durable Project Brain knowledge. <strong>Local stays local:</strong> raw source, secrets, terminal execution, device credentials and machine-specific indexes.</p>
      </footer>
    </section>
  );
}

export default function Home() {
  return (
    <main className={styles.page}>
      <header className={styles.nav}>
        <Link className={styles.brand} href="/" aria-label="CodeLocal home">
          <Image src="/codelocal-icon.png" alt="" width={30} height={30} priority />
          <span>CodeLocal</span>
        </Link>

        <nav className={styles.navCapsule} aria-label="Primary navigation">
          <a href="#system">Product</a>
          <Link href="/security">Security</Link>
          <Link href="/support">Support</Link>
        </nav>

        <div className={styles.navActions}>
          <Link href="/login">Sign in</Link>
          <Link className={styles.navButton} href="/dashboard">Open app</Link>
        </div>
      </header>

      <section className={styles.hero}>
        <div className={styles.heroAtmosphere} aria-hidden="true"><i /><i /><i /></div>
        <NeuralGraph />
        <div className={styles.heroCopy}>
          <span className={styles.heroPill}>Local-first AI infrastructure</span>
          <h1>Your project has a brain.<br /><span>Every AI can plug into it.</span></h1>
          <p>Knowledge, code relationships, skills and verified experience stay connected while approved actions run safely on your machine.</p>
          <div className={styles.actions}>
            <Link className={styles.primaryButton} href="/dashboard">Open CodeLocal</Link>
            <a className={styles.secondaryButton} href="#system">See the architecture</a>
          </div>
          <div className={styles.heroMeta}>
            <span>MCP-native</span><i />
            <span>Workspace scoped</span><i />
            <span>Local execution</span>
          </div>
        </div>
      </section>

      <ArchitectureMap />

      <section className={styles.valueSection}>
        <div>
          <span className={styles.kicker}>Why it matters</span>
          <h2>AI should not rediscover your project every session.</h2>
        </div>
        <div className={styles.valueGrid}>
          <article><span>01</span><strong>Remember</strong><p>Structure, decisions, skills and useful context remain available across compatible AI clients.</p></article>
          <article><span>02</span><strong>Prove</strong><p>Diagnostics, tests and diffs separate verified project knowledge from guesses.</p></article>
          <article><span>03</span><strong>Reuse</strong><p>Verified experience can guide the next task without granting broad access to your machine.</p></article>
        </div>
      </section>

      <section className={styles.setup} id="how-it-works">
        <div className={styles.setupIntro}>
          <span className={styles.kicker}>Start with one project</span>
          <h2>Connect in four clear steps.</h2>
          <p>No mystery permissions and no need to teach every AI the project again from scratch.</p>
          <Link className={styles.textLink} href="/security">Read the security model <span aria-hidden="true">↗</span></Link>
        </div>
        <ol className={styles.setupList}>
          {setupSteps.map((step, index) => (
            <li key={step.title}>
              <span>{String(index + 1).padStart(2, "0")}</span>
              <div>
                <strong>{step.title}</strong>
                <p>{step.description}</p>
                {"command" in step && <CopyCommand command={step.command} />}
              </div>
            </li>
          ))}
        </ol>
      </section>
    </main>
  );
}
