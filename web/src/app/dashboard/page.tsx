import Image from "next/image";
import Link from "next/link";
import styles from "./dashboard.module.css";

const navigation = [
  "Overview",
  "Workspaces",
  "Knowledge Graph",
  "Code Graph",
  "Devices",
  "Usage",
  "Security",
  "Account",
];

const migration = [
  { label: "Web foundation", state: "Ready", note: "Next.js App Router, TypeScript, design tokens and route shell." },
  { label: "Backend contract", state: "Next", note: "Expose minimal Go HTTP DTOs without moving authorization truth into Next.js." },
  { label: "Auth parity", state: "Queued", note: "Preserve existing session, CSRF, rate-limit and redirect behavior before cutover." },
  { label: "Dashboard parity", state: "Queued", note: "Move real server-backed cards route by route; no mocked live telemetry." },
];

export default function DashboardPage() {
  return (
    <main className={styles.shell}>
      <aside className={styles.sidebar}>
        <Link className={styles.brand} href="/">
          <span className={styles.brandMark} aria-hidden="true">
            <Image src="/codelocal-icon.png" alt="" width={34} height={34} priority />
          </span>
          <span>CodeLocal</span>
        </Link>

        <div className={styles.navLabel}>Control plane</div>
        <nav className={styles.nav} aria-label="Dashboard preview navigation">
          {navigation.map((item, index) => (
            <span className={index === 0 ? styles.activeNav : undefined} key={item}>
              <i aria-hidden="true" />
              {item}
            </span>
          ))}
        </nav>

        <div className={styles.sidebarNotice}>
          <span>Migration shell</span>
          <p>This route is presentation-only until Go API/auth parity is connected.</p>
        </div>
      </aside>

      <section className={styles.content}>
        <header className={styles.header}>
          <div>
            <span className={styles.eyebrow}>Next.js migration · Slice 1</span>
            <h1>Overview</h1>
            <p>New browser architecture without changing the current production authority model.</p>
          </div>
          <a className={styles.productionLink} href="https://codelocal.cloud/dashboard">Current production dashboard</a>
        </header>

        <section className={styles.heroPanel}>
          <div>
            <span className={styles.eyebrow}>Boundary status</span>
            <h2>UI foundation is live in the repo. Production data is intentionally not wired yet.</h2>
            <p>
              The next migration step is to expose explicit Go backend DTOs and preserve current auth/security semantics.
              Until then, this shell does not claim device, workspace, token or Project Brain activity.
            </p>
          </div>
          <div className={styles.boundaryDiagram} aria-label="Web and backend ownership diagram">
            <span>Next.js web</span>
            <b>HTTP contracts</b>
            <span>Go backend</span>
            <b>authenticated runtime</b>
            <span>local machine</span>
          </div>
        </section>

        <section className={styles.grid}>
          <article className={styles.panel}>
            <div className={styles.panelHead}>
              <div>
                <span className={styles.eyebrow}>Migration track</span>
                <h3>Route-family rollout</h3>
              </div>
              <span className={styles.badge}>NO CUTOVER YET</span>
            </div>
            <div className={styles.migrationList}>
              {migration.map((item) => (
                <div className={styles.migrationRow} key={item.label}>
                  <span className={styles.stateDot} data-state={item.state.toLowerCase()} />
                  <div>
                    <strong>{item.label}</strong>
                    <p>{item.note}</p>
                  </div>
                  <span className={styles.state}>{item.state}</span>
                </div>
              ))}
            </div>
          </article>

          <article className={styles.panel}>
            <div className={styles.panelHead}>
              <div>
                <span className={styles.eyebrow}>Authority map</span>
                <h3>What stays where</h3>
              </div>
            </div>
            <dl className={styles.authorityList}>
              <div><dt>Web presentation</dt><dd>Next.js / TypeScript</dd></div>
              <div><dt>Cloud authority</dt><dd>Go backend</dd></div>
              <div><dt>Local execution</dt><dd>Go runtime / CLI</dd></div>
              <div><dt>Desktop + mobile</dt><dd>Flutter / Dart</dd></div>
              <div><dt>macOS deep integration</dt><dd>Swift native bridge</dd></div>
            </dl>
          </article>
        </section>
      </section>
    </main>
  );
}
