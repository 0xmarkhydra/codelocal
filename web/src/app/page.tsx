import Image from "next/image";
import Link from "next/link";
import styles from "./page.module.css";

const principles = [
  ["Local by default", "Files, Git, terminal, browser and computer actions run through the paired runtime on your machine."],
  ["Explicit access", "A workspace exists for an AI client only after you authorize it. No broad filesystem exposure."],
  ["Durable context", "Project Brain carries verified project knowledge across sessions and compatible AI clients."],
] as const;

const setupSteps = [
  ["Pair your machine", "Sign in and approve the CodeLocal runtime you control."],
  ["Grant a project", "Run codelocal . inside only the workspace you want to expose."],
  ["Connect over MCP", "Use one CodeLocal endpoint from a compatible AI client."],
] as const;

function ProjectBrainStructure() {
  return (
    <div className={styles.brainStage} aria-label="Project Brain structure">
      <div className={styles.brainHead}>
        <div>
          <span>Project Brain</span>
          <strong>Context that belongs to the project.</strong>
        </div>
        <small>Structure, not simulated activity</small>
      </div>

      <svg className={styles.brainMap} viewBox="0 0 640 390" role="img" aria-label="Project Brain connects code structure, learned skills, project memory and the local runtime">
        <g className={styles.brainEdges}>
          <path d="M320 198 L148 94" />
          <path d="M320 198 L488 92" />
          <path d="M320 198 L130 286" />
          <path d="M320 198 L510 286" />
          <path d="M148 94 L488 92" />
          <path d="M130 286 L510 286" />
        </g>
        <g className={styles.brainNodes}>
          <circle cx="320" cy="198" r="12" className={styles.brainCore} />
          <circle cx="148" cy="94" r="7" />
          <circle cx="488" cy="92" r="7" />
          <circle cx="130" cy="286" r="7" />
          <circle cx="510" cy="286" r="7" />
        </g>
        <g className={styles.brainLabels}>
          <text x="320" y="229" textAnchor="middle">Project Brain</text>
          <text x="148" y="72" textAnchor="middle">Code structure</text>
          <text x="488" y="70" textAnchor="middle">Project memory</text>
          <text x="130" y="316" textAnchor="middle">Learned skills</text>
          <text x="510" y="316" textAnchor="middle">Local runtime</text>
        </g>
      </svg>

      <div className={styles.brainFoot}>
        <span>AI client</span>
        <i aria-hidden="true">→</i>
        <span>Go authority</span>
        <i aria-hidden="true">→</i>
        <span>authorized workspace</span>
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
          <Link className={styles.navButton} href="/dashboard">Dashboard</Link>
        </nav>
      </header>

      <section className={styles.hero}>
        <div className={styles.heroCopy}>
          <span className={styles.eyebrow}>Local execution · durable project context</span>
          <h1>Your projects stay local. Your AI stays in context.</h1>
          <p>
            CodeLocal is the control layer between compatible AI clients and the machine where your work actually lives. Grant only the projects you choose, keep execution local, and carry project intelligence across sessions.
          </p>
          <div className={styles.actions}>
            <Link className={styles.primaryButton} href="/dashboard">Open CodeLocal</Link>
            <a className={styles.secondaryButton} href="#how-it-works">How it works</a>
          </div>
          <div className={styles.clientLine} aria-label="Compatible client examples">
            <span>Works with MCP-compatible clients</span>
            <b>ChatGPT</b>
            <b>Claude</b>
            <b>Codex</b>
          </div>
        </div>

        <ProjectBrainStructure />
      </section>

      <section className={styles.principles}>
        <div className={styles.sectionIntro}>
          <span>Designed around authority boundaries</span>
          <h2>The control layer between AI and your machine.</h2>
        </div>
        <div className={styles.principleGrid}>
          {principles.map(([title, description]) => (
            <article key={title}>
              <strong>{title}</strong>
              <p>{description}</p>
            </article>
          ))}
        </div>
      </section>

      <section className={styles.setup} id="how-it-works">
        <div className={styles.setupIntro}>
          <span>Three steps</span>
          <h2>One endpoint. Only the projects you grant.</h2>
          <p>CodeLocal keeps the browser simple because the real authority stays in the Go gateway and your paired local runtime.</p>
          <div className={styles.actions}>
            <Link className={styles.primaryButton} href="/dashboard/connect">Connect an AI client</Link>
            <Link className={styles.textLink} href="/security">Read the security model <span aria-hidden="true">↗</span></Link>
          </div>
        </div>

        <ol className={styles.setupList}>
          {setupSteps.map(([title, description], index) => (
            <li key={title}>
              <span>{String(index + 1).padStart(2, "0")}</span>
              <div>
                <strong>{title}</strong>
                <p>{description}</p>
              </div>
            </li>
          ))}
        </ol>
      </section>
    </main>
  );
}