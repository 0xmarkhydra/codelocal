import Image from "next/image";
import Link from "next/link";
import styles from "./page.module.css";

const principles = [
  ["Local execution", "Files, Git, terminal, browser and computer actions stay on the paired runtime you control."],
  ["Scoped authority", "AI clients only receive access to the workspaces you explicitly authorize."],
  ["Durable context", "Project Brain keeps verified project knowledge useful across sessions and compatible clients."],
] as const;

const setupSteps = [
  ["Pair the runtime", "Connect the machine where your work actually lives."],
  ["Grant a workspace", "Run codelocal . only inside the project you want to expose."],
  ["Connect your AI", "Use one MCP endpoint from a compatible AI client."],
] as const;

function ControlSurface() {
  return (
    <div className={styles.surface} aria-label="CodeLocal control surface">
      <div className={styles.surfaceTopbar}>
        <div className={styles.windowDots} aria-hidden="true"><i /><i /><i /></div>
        <span>CodeLocal control plane</span>
        <span className={styles.surfaceStatus}>local authority</span>
      </div>

      <div className={styles.surfaceBody}>
        <div className={styles.surfaceColumn}>
          <span className={styles.surfaceLabel}>AI clients</span>
          <div className={styles.clientStack}>
            <div><b>ChatGPT</b><span>MCP client</span></div>
            <div><b>Claude</b><span>MCP client</span></div>
            <div><b>Codex</b><span>MCP client</span></div>
          </div>
        </div>

        <div className={styles.brainCanvas}>
          <svg viewBox="0 0 520 360" role="img" aria-label="Project Brain links code structure, project memory, learned skills and local runtime">
            <defs>
              <radialGradient id="brainGlow">
                <stop offset="0" stopColor="#9aa7ff" stopOpacity=".34" />
                <stop offset="1" stopColor="#9aa7ff" stopOpacity="0" />
              </radialGradient>
            </defs>
            <circle cx="260" cy="180" r="94" fill="url(#brainGlow)" />
            <g className={styles.surfaceEdges}>
              <path d="M260 180 L128 82" />
              <path d="M260 180 L394 86" />
              <path d="M260 180 L112 276" />
              <path d="M260 180 L408 276" />
            </g>
            <g className={styles.surfaceNodes}>
              <circle cx="260" cy="180" r="16" className={styles.coreNode} />
              <circle cx="128" cy="82" r="7" />
              <circle cx="394" cy="86" r="7" />
              <circle cx="112" cy="276" r="7" />
              <circle cx="408" cy="276" r="7" />
            </g>
            <g className={styles.surfaceLabels}>
              <text x="260" y="214" textAnchor="middle">Project Brain</text>
              <text x="128" y="60" textAnchor="middle">Code structure</text>
              <text x="394" y="64" textAnchor="middle">Project memory</text>
              <text x="112" y="306" textAnchor="middle">Learned skills</text>
              <text x="408" y="306" textAnchor="middle">Local runtime</text>
            </g>
          </svg>
          <div className={styles.brainCaption}>
            <span>Project context</span>
            <strong>One brain. Multiple AI clients.</strong>
          </div>
        </div>

        <div className={styles.surfaceColumn}>
          <span className={styles.surfaceLabel}>Local runtime</span>
          <div className={styles.runtimeStack}>
            <div><span>Workspace</span><b>explicitly granted</b></div>
            <div><span>Execution</span><b>your machine</b></div>
            <div><span>Authority</span><b>Go gateway</b></div>
          </div>
        </div>
      </div>
    </div>
  );
}

export default function Home() {
  return (
    <main className={styles.page}>
      <header className={styles.nav}>
        <Link className={styles.brand} href="/" aria-label="CodeLocal home">
          <Image src="/codelocal-icon.png" alt="" width={28} height={28} priority />
          <span>CodeLocal</span>
        </Link>
        <nav className={styles.navActions} aria-label="Primary navigation">
          <Link href="/security">Security</Link>
          <Link href="/support">Support</Link>
          <Link href="/login">Sign in</Link>
          <Link className={styles.navButton} href="/dashboard">Open app</Link>
        </nav>
      </header>

      <section className={styles.hero}>
        <div className={styles.heroCopy}>
          <span className={styles.eyebrow}>Private execution. Durable project intelligence.</span>
          <h1>AI can reason anywhere. Your work should stay where it belongs.</h1>
          <p>CodeLocal gives compatible AI clients controlled access to the projects you choose, while execution and raw source stay on your machine.</p>
          <div className={styles.actions}>
            <Link className={styles.primaryButton} href="/dashboard">Open CodeLocal</Link>
            <a className={styles.secondaryButton} href="#how-it-works">See how it works</a>
          </div>
        </div>

        <ControlSurface />
      </section>

      <section className={styles.principles}>
        {principles.map(([title, description]) => (
          <article key={title}>
            <strong>{title}</strong>
            <p>{description}</p>
          </article>
        ))}
      </section>

      <section className={styles.setup} id="how-it-works">
        <div className={styles.setupIntro}>
          <span>From zero to connected</span>
          <h2>Three explicit steps. No broad machine access.</h2>
          <p>The browser is only the control surface. Identity, authorization and runtime execution remain behind the Go authority boundary.</p>
          <Link className={styles.textLink} href="/security">Read the security model <span aria-hidden="true">↗</span></Link>
        </div>
        <ol className={styles.setupList}>
          {setupSteps.map(([title, description], index) => (
            <li key={title}>
              <span>{String(index + 1).padStart(2, "0")}</span>
              <div><strong>{title}</strong><p>{description}</p></div>
            </li>
          ))}
        </ol>
      </section>
    </main>
  );
}
