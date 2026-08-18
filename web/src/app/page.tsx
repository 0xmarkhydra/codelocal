import Image from "next/image";
import Link from "next/link";
import styles from "./page.module.css";

const surfaces = [
  { label: "Backend", value: "Go", detail: "Cloud API · MCP · Project Brain" },
  { label: "Web", value: "Next.js", detail: "Dashboard · Admin · Public product" },
  { label: "App", value: "Flutter", detail: "iOS · Android" },
  { label: "Desktop", value: "Flutter", detail: "macOS · Windows · Linux" },
  { label: "CLI", value: "Go", detail: "Local runtime · workspace execution" },
  { label: "Native", value: "OS bridge", detail: "Swift and bounded platform APIs" },
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
          <a href="https://codelocal.cloud">Current production</a>
          <Link className={styles.navButton} href="/dashboard">Open new shell</Link>
        </nav>
      </header>

      <section className={styles.hero}>
        <div className={styles.heroCopy}>
          <div className={styles.eyebrow}>Universal MCP · Local execution · Durable Project Brain</div>
          <h1>Your AI client reasons. CodeLocal gives it controlled hands and project memory.</h1>
          <p>
            One product layer for cloud identity, local workspace execution, reusable project intelligence,
            and a growing family of web, mobile and desktop clients.
          </p>
          <div className={styles.actions}>
            <Link className={styles.primaryButton} href="/dashboard">Explore the new dashboard shell</Link>
            <a className={styles.secondaryButton} href="https://codelocal.cloud">Use current production</a>
          </div>
          <p className={styles.migrationNote}>
            This Next.js surface is being migrated incrementally. Existing Go-rendered production routes remain the fallback until parity is verified.
          </p>
        </div>

        <div className={styles.systemFrame} aria-label="CodeLocal architecture overview">
          <div className={styles.frameTop}>
            <span>PRODUCT STACK</span>
            <span className={styles.frameState}>ARCHITECTURE · NOT LIVE TELEMETRY</span>
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
            <span>AI clients</span><b>→</b><span>Go backend</span><b>→</b><span>local runtime</span><b>→</b><span>authorized workspace</span>
          </div>
        </div>
      </section>

      <section className={styles.principles}>
        <div>
          <span className={styles.sectionLabel}>Architecture rule</span>
          <h2>Different surfaces, one authority model.</h2>
        </div>
        <div className={styles.principleGrid}>
          <article>
            <strong>Cloud authority stays in Go.</strong>
            <p>Identity, authorization, billing, Project Brain and durable state remain backend responsibilities.</p>
          </article>
          <article>
            <strong>Execution stays local.</strong>
            <p>CLI/runtime controls filesystem, Git, terminal, MCP extensions and native computer capabilities on the authorized machine.</p>
          </article>
          <article>
            <strong>Clients stay replaceable.</strong>
            <p>Next.js and Flutter are presentation/product surfaces over stable contracts, not duplicate implementations of security truth.</p>
          </article>
        </div>
      </section>
    </main>
  );
}
