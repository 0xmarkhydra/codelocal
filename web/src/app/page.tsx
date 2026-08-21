import Image from "next/image";
import Link from "next/link";
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
          <Image src="/codelocal-icon.png" alt="" width={28} height={28} priority />
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
        <div className={styles.heroCopy}>
          <span className={styles.heroPill}>Local-first AI infrastructure</span>
          <h1>Your AI changes.<br /><span>Your project understanding stays.</span></h1>
          <p>CodeLocal gives every compatible AI the same durable project brain, then routes approved actions to the machine and workspace you control.</p>
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
                {"command" in step && <code className={styles.setupCommand}>{step.command}</code>}
              </div>
            </li>
          ))}
        </ol>
      </section>
    </main>
  );
}
