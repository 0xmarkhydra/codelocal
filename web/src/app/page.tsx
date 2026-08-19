import Image from "next/image";
import Link from "next/link";
import styles from "./page.module.css";

const surfaces = [
  { label: "AI clients", value: "MCP", detail: "ChatGPT · Claude · Codex · compatible clients" },
  { label: "Cloud authority", value: "Go", detail: "Identity · OAuth · routing · Project Brain" },
  { label: "Local runtime", value: "CodeLocal", detail: "Files · Git · terminal · browser · computer" },
  { label: "Project access", value: "Explicit", detail: "Only workspaces you authorize" },
];

export default function Home() {
  return (
    <main className={styles.page}>
      <header className={styles.nav}>
        <Link className={styles.brand} href="/" aria-label="CodeLocal home">
          <span className={styles.brandMark} aria-hidden="true">
            <Image src="/codelocal-icon.png" alt="" width={34} height={34} priority />
          </span>
          <span>CodeLocal</span>
        </Link>
        <nav className={styles.navActions} aria-label="Primary navigation">
          <Link href="/security">Security</Link>
          <Link href="/login">Sign in</Link>
          <Link className={styles.navButton} href="/dashboard">Open dashboard</Link>
        </nav>
      </header>

      <section className={styles.hero}>
        <div className={styles.heroCopy}>
          <div className={styles.eyebrow}>Universal MCP · Local execution · Durable Project Brain</div>
          <h1>Your AI can reason. CodeLocal gives it controlled hands and project memory.</h1>
          <p>
            Connect compatible AI clients to the projects on your own machine without turning your computer into an open remote filesystem. Authorize the folders you want, keep execution local, and carry durable project intelligence across sessions.
          </p>
          <div className={styles.actions}>
            <Link className={styles.primaryButton} href="/dashboard">Open CodeLocal</Link>
            <a className={styles.secondaryButton} href="#setup">Connect an AI client</a>
          </div>
        </div>

        <div className={styles.systemFrame} aria-label="CodeLocal trust boundary">
          <div className={styles.frameTop}>
            <span>LOCAL-FIRST CONTROL PLANE</span>
            <span className={styles.frameState}>REAL AUTHORITY BOUNDARIES</span>
          </div>
          <div className={styles.stackGrid}>
            {surfaces.map((surface) => (
              <article className={styles.stackCard} key={surface.label}>
                <span>{surface.label}</span>
                <strong>{surface.value}</strong>
                <small>{surface.detail}</small>
              </article>
            ))}
          </div>
          <div className={styles.flow}>
            <span>AI client</span><b>→</b><span>Go gateway</span><b>→</b><span>paired runtime</span><b>→</b><span>authorized workspace</span>
          </div>
        </div>
      </section>

      <section className={styles.principles}>
        <div>
          <span className={styles.sectionLabel}>Trust model</span>
          <h2>Cloud coordination. Local execution.</h2>
        </div>
        <div className={styles.principleGrid}>
          <article><strong>Your source stays local.</strong><p>Filesystem and execution run through the paired CodeLocal runtime on the machine you control.</p></article>
          <article><strong>Access is workspace-scoped.</strong><p>A project becomes available only after you explicitly authorize that workspace.</p></article>
          <article><strong>Security truth stays centralized.</strong><p>Identity, OAuth, session security and authorization remain enforced by the Go backend rather than duplicated in the UI.</p></article>
        </div>
      </section>

      <section className={styles.setup} id="setup">
        <div>
          <span className={styles.sectionLabel}>Get connected</span>
          <h2>One MCP endpoint. Your projects stay on your machine.</h2>
          <p>Sign in, pair your machine, authorize a project with <code>codelocal .</code>, then copy your MCP endpoint into a compatible AI client.</p>
          <div className={styles.actions}>
            <Link className={styles.primaryButton} href="/dashboard/connect">Open MCP Connections</Link>
            <Link className={styles.secondaryButton} href="/support">Support</Link>
          </div>
        </div>
        <div className={styles.setupGrid}>
          <article><span>01</span><strong>Pair a machine</strong><small>Approve a local CodeLocal runtime under your account.</small></article>
          <article><span>02</span><strong>Authorize a workspace</strong><small>Run <code>codelocal .</code> only inside the project you want to expose.</small></article>
          <article><span>03</span><strong>Connect over MCP</strong><small>OAuth binds the AI client to your account and permitted workspaces.</small></article>
        </div>
      </section>
    </main>
  );
}
