import Image from "next/image";
import Link from "next/link";
import styles from "./page.module.css";

const aiClients = ["ChatGPT", "Claude", "Codex"] as const;
const workspaceTools = ["Files", "Git", "Terminal", "Browser", "Computer"] as const;

const capabilities = [
  ["Remember", "Project Brain carries structure, decisions, skills and useful context across sessions."],
  ["Control", "Identity, workspace scope and approvals stay explicit instead of giving an AI broad machine access."],
  ["Execute", "Approved actions run through the paired local runtime where your project and tools already live."],
] as const;

const setupSteps = [
  ["Pair your machine", "Connect the CodeLocal runtime on the computer where your work lives."],
  ["Grant a workspace", "Choose the project an AI client is allowed to understand and operate on."],
  ["Connect your AI", "Use one MCP endpoint from ChatGPT, Claude, Codex or another compatible client."],
] as const;

function FlowConnector() {
  return (
    <div className={styles.flowConnector} aria-hidden="true">
      <span className={styles.flowLine} />
      <i className={styles.flowPulse} />
    </div>
  );
}

function SystemMap() {
  return (
    <section className={styles.systemFrame} aria-label="How CodeLocal works">
      <header className={styles.systemHeader}>
        <div>
          <span className={styles.sectionKicker}>System map</span>
          <strong>How a request moves through CodeLocal</strong>
        </div>
        <span className={styles.localBadge}><i /> Source stays on your machine</span>
      </header>

      <div className={styles.systemMainFlow}>
        <article className={styles.systemNode}>
          <div className={styles.nodeHead}><span>01</span><b>AI clients</b></div>
          <p>Use the AI you already prefer.</p>
          <div className={styles.clientList}>
            {aiClients.map((client) => <span key={client}>{client}</span>)}
          </div>
        </article>

        <FlowConnector />

        <article className={`${styles.systemNode} ${styles.gatewayNode}`}>
          <div className={styles.nodeHead}><span>02</span><b>CodeLocal gateway</b></div>
          <p>One controlled entry point for identity, MCP, workspace scope and approvals.</p>
          <div className={styles.nodeMeta}>
            <span>Identity</span><span>OAuth</span><span>Approvals</span>
          </div>
        </article>

        <FlowConnector />

        <article className={styles.systemNode}>
          <div className={styles.nodeHead}><span>03</span><b>Local runtime</b></div>
          <p>Approved work is routed to the paired runtime running on your machine.</p>
          <div className={styles.nodeMeta}>
            <span>Scoped</span><span>Local</span><span>Observable</span>
          </div>
        </article>

        <FlowConnector />

        <article className={styles.systemNode}>
          <div className={styles.nodeHead}><span>04</span><b>Workspace</b></div>
          <p>Only the project you granted is exposed to the runtime.</p>
          <div className={styles.toolList}>
            {workspaceTools.map((tool) => <span key={tool}>{tool}</span>)}
          </div>
        </article>
      </div>

      <div className={styles.brainLayer}>
        <div className={styles.brainConnection} aria-hidden="true"><i /></div>
        <div className={styles.brainIdentity}>
          <div className={styles.brainMark} aria-hidden="true"><span /><span /><span /><span /></div>
          <div>
            <span className={styles.sectionKicker}>Project Brain</span>
            <strong>The context layer shared across AI clients</strong>
          </div>
        </div>
        <p>Code structure, project memory, learned skills and relationships are reusable instead of being rediscovered every session.</p>
        <div className={styles.brainFacts}>
          <span>Structure</span><span>Memory</span><span>Skills</span><span>Relationships</span>
        </div>
      </div>

      <footer className={styles.systemFooter}>
        <span><i className={styles.requestKey} /> Request &amp; tool path</span>
        <span><i className={styles.contextKey} /> Reusable project context</span>
        <strong>AI changes. Project understanding stays.</strong>
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
        <nav className={styles.navActions} aria-label="Primary navigation">
          <Link href="/security">Security</Link>
          <Link href="/support">Support</Link>
          <Link href="/login">Sign in</Link>
          <Link className={styles.navButton} href="/dashboard">Open app</Link>
        </nav>
      </header>

      <section className={styles.hero}>
        <div className={styles.heroCopy}>
          <span className={styles.eyebrow}>A project brain and local control plane for AI</span>
          <h1>One project brain. Any AI. Your machine.</h1>
          <p>Connect ChatGPT, Claude, Codex and other MCP clients to the same project understanding. CodeLocal remembers how your system works, then routes approved actions to the local runtime you control.</p>
          <div className={styles.actions}>
            <Link className={styles.primaryButton} href="/dashboard">Open CodeLocal</Link>
            <a className={styles.secondaryButton} href="#system">Explore the system</a>
          </div>
          <div className={styles.heroTrust} aria-label="CodeLocal trust model">
            <span>No broad filesystem access</span>
            <span>Workspace scoped</span>
            <span>Local execution</span>
          </div>
        </div>
      </section>

      <div id="system"><SystemMap /></div>

      <section className={styles.capabilities} aria-label="What CodeLocal adds">
        {capabilities.map(([title, description], index) => (
          <article key={title}>
            <span>{String(index + 1).padStart(2, "0")}</span>
            <div><strong>{title}</strong><p>{description}</p></div>
          </article>
        ))}
      </section>

      <section className={styles.setup} id="how-it-works">
        <div className={styles.setupIntro}>
          <span className={styles.sectionKicker}>Start with one project</span>
          <h2>Connect in three explicit steps.</h2>
          <p>No mystery permissions and no need to teach every AI the project again from scratch.</p>
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
