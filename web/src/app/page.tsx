import Image from "next/image";
import Link from "next/link";
import { ArchitectureModel } from "./_components/architecture-model";
import { AppIcon, type AppIconName } from "./dashboard/app-icon";
import styles from "./page.module.css";

const principles: { icon: AppIconName; label: string; title: string; copy: string }[] = [
  { icon: "brain", label: "PROJECT BRAIN", title: "Context that carries forward.", copy: "Project knowledge, reusable skills and verified experience give your AI a foundation beyond the current conversation." },
  { icon: "device", label: "LOCAL EXECUTION", title: "Your machine. Your control.", copy: "Work with the tools and files on your own device, within explicit workspace and approval boundaries." },
  { icon: "connection", label: "MODEL-NEUTRAL", title: "Change models. Keep context.", copy: "Connect compatible AI clients through MCP. Your project intelligence does not have to start over with each tool." },
];
const productLinks: { icon: AppIconName; title: string; copy: string; href: string }[] = [
  { icon: "folder", title: "Workspaces & devices", copy: "Projects, paired machines and connection status.", href: "/dashboard/workspaces" },
  { icon: "brain", title: "Project Brain", copy: "Project knowledge and the relationships behind your code.", href: "/dashboard/knowledge" },
  { icon: "skill", title: "Skills & experience", copy: "Reusable capabilities and lessons from verified work.", href: "/dashboard/skills" },
];
const steps = [
  { title: "Install the runtime", copy: "On the machine where your code lives.", command: "npm install -g codelocal@latest" },
  { title: "Connect your workspace", copy: "Pair your device and choose your project.", command: "codelocal ." },
  { title: "Bring your AI", copy: "Connect a compatible client through your MCP endpoint.", command: "One project. Shared context." },
];

export default function Home() {
  return (
    <main className={styles.page}>
      <a className={styles.skipLink} href="#main-content">Skip to content</a>
      <header className={styles.nav}>
        <Link className={styles.brand} href="/" aria-label="CodeLocal home">
          <Image src="/codelocal-icon.png" alt="" width={32} height={32} />
          CodeLocal<span className={styles.brandDot}>.</span>
        </Link>
        <nav className={styles.navLinks} aria-label="Primary navigation">
          <a href="#product">Product</a>
          <a href="#system">Architecture</a>
          <Link href="/security">Security</Link>
          <Link href="/blogs">Journal</Link>
        </nav>
        <Link className={styles.navButton} href="/dashboard">
          Open dashboard <AppIcon name="external" size={15} />
        </Link>
      </header>

      <section className={styles.hero} id="main-content">
        <Image className={styles.heroImage} src="/codelocal-sculpture.webp" alt="" fill sizes="100vw" preload />
        <div className={styles.heroCopy}>
          <span className={styles.eyebrow}>PROJECT INTELLIGENCE. LOCAL CONTROL.</span>
          <h1>CodeLocal</h1>
          <h2>A shared intelligence layer for the way you build with AI.</h2>
          <p>Connect your AI to one Project Brain. Keep execution on your machine.</p>
          <div className={styles.actions}>
            <Link className={styles.primaryButton} href="/dashboard">Explore CodeLocal <AppIcon name="external" size={17} /></Link>
            <a className={styles.secondaryButton} href="#system">See how it works <AppIcon name="chevron-down" size={17} /></a>
          </div>
        </div>
        <div className={styles.heroFoot}>
          <span><AppIcon name="shield" size={15} /> Your workspace. Your permission.</span>
          <span>CONTEXT / EXECUTION / MEMORY</span>
        </div>
      </section>

      <section className={styles.clients} aria-label="Compatible clients, not endorsements">
        <span>BUILT TO CONNECT WITH YOUR AI</span>
        <div><strong>ChatGPT</strong><strong>Codex</strong><strong>Claude</strong><span>Compatible MCP clients <AppIcon name="connection" size={18} /></span></div>
      </section>

      <section className={styles.principles} id="product">
        <div className={styles.container}>
          <div className={styles.sectionHeading}>
            <div><span className={styles.eyebrow}>01 / THE MISSING LAYER</span><h2>Better AI.<br />Not another fresh start.</h2></div>
            <p>Models evolve. Tools change.<br />Your project should keep learning.</p>
          </div>
          <div className={styles.principleGrid}>
            {principles.map((item) => (
              <article key={item.label}>
                <div><AppIcon name={item.icon} size={25} /><span>{item.label}</span></div>
                <h3>{item.title}</h3><p>{item.copy}</p>
              </article>
            ))}
          </div>
        </div>
      </section>

      <section className={`${styles.container} ${styles.system}`} id="system">
        <div className={styles.sectionHeading}>
          <div><span className={styles.eyebrow}>02 / ONE CONNECTED SYSTEM</span><h2>Intelligence shared.<br /><span>Boundaries preserved.</span></h2></div>
          <p>AI clients, cloud coordination and local execution. Connected through one project-aware system.</p>
        </div>
        <ArchitectureModel />
        <div className={styles.boundary}>
          <span><AppIcon name="shield" size={22} /> Scoped by design.</span>
          <p>Local tools operate on your machine. Cloud services coordinate identity, routing and sanitized project knowledge.</p>
          <Link href="/security">Security model <AppIcon name="external" size={16} /></Link>
        </div>
      </section>

      <section className={styles.productBand}>
        <div className={`${styles.container} ${styles.productLayout}`}>
          <div>
            <span className={styles.eyebrow}>03 / THE PRODUCT</span>
            <h2>One place to see<br />the whole project.</h2>
            <p>Keep your connected workspaces, project knowledge and access controls in view.</p>
            <Link className={styles.primaryButton} href="/dashboard">Open your dashboard <AppIcon name="external" size={17} /></Link>
          </div>
          <nav className={styles.productLinks} aria-label="Explore the product">
            {productLinks.map((item) => (
              <Link href={item.href} key={item.href}>
                <AppIcon name={item.icon} size={25} />
                <div><h3>{item.title}</h3><p>{item.copy}</p></div>
                <AppIcon name="chevron-right" size={20} />
              </Link>
            ))}
          </nav>
        </div>
      </section>

      <section className={`${styles.container} ${styles.setup}`} id="how-it-works">
        <div className={styles.sectionHeading}>
          <div><span className={styles.eyebrow}>04 / GET CONNECTED</span><h2>Your machine.<br />Three steps to connected.</h2></div>
          <Link className={styles.secondaryButton} href="/dashboard/connect">Connection guide <AppIcon name="external" size={17} /></Link>
        </div>
        <ol className={styles.steps}>
          {steps.map((step, i) => (
            <li key={step.title}><span className={styles.stepNumber}>0{i + 1}</span><h3>{step.title}</h3><p>{step.copy}</p><code>{step.command}</code></li>
          ))}
        </ol>
        <section className={styles.desktop} id="desktop" aria-labelledby="desktop-title">
          <AppIcon name="device" size={35} />
          <div><span className={styles.eyebrow}>ON THE ROADMAP</span><h3 id="desktop-title">CodeLocal Desktop</h3><p>macOS · Windows · Linux</p></div>
          <span className={styles.roadmapStatus}>In development</span>
        </section>
      </section>

      <section className={styles.closing}>
        <div className={styles.container}>
          <span className={styles.eyebrow}>BUILD ON WHAT YOU KNOW</span>
          <h2>The next model will change.<br /><span>Your project stays yours.</span></h2>
          <div className={styles.actions}>
            <Link className={styles.primaryButton} href="/dashboard">Get started <AppIcon name="external" size={17} /></Link>
            <Link className={styles.secondaryButton} href="/support">Talk to us <AppIcon name="chevron-right" size={17} /></Link>
          </div>
        </div>
      </section>
      <footer className={`${styles.container} ${styles.footer}`}>
        <Link className={styles.brand} href="/"><Image src="/codelocal-icon.png" alt="" width={28} height={28} />CodeLocal.</Link>
        <nav aria-label="Footer navigation"><Link href="/blogs">Journal</Link><Link href="/security">Security</Link><Link href="/privacy">Privacy</Link><Link href="/terms">Terms</Link><Link href="/support">Contact</Link><Link href="/login">Sign in</Link></nav>
      </footer>
    </main>
  );
}
