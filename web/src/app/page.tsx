import Image from "next/image";
import Link from "next/link";
import { getTranslations } from "@/lib/i18n/server";
import { LanguageSelect } from "@/lib/i18n/provider";
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

export default async function Home() {
  const t = await getTranslations();
  return (
    <main className={styles.page}>
      <a className={styles.skipLink} href="#main-content">{t("Skip to content")}</a>
      <header className={styles.nav}>
        <Link className={styles.brand} href="/" aria-label={t("CodeLocal home")}>
          <Image src="/codelocal-icon.png" alt="" width={32} height={32} />
          CodeLocal<span className={styles.brandDot}>.</span>
        </Link>
        <nav className={styles.navLinks} aria-label={t("Primary navigation")}>
          <a href="#product">{t("Product")}</a>
          <a href="#system">{t("Architecture")}</a>
          <Link href="/security">{t("Security")}</Link>
          <Link href="/blogs">{t("Journal")}</Link>
        </nav>
        <div className={styles.navActions}>
          <LanguageSelect />
          <Link className={styles.navButton} href="/dashboard">
            {t("Open dashboard")} <AppIcon name="external" size={15} />
          </Link>
        </div>
      </header>

      <section className={styles.hero} id="main-content">
        <Image className={styles.heroImage} src="/codelocal-sculpture.webp" alt="" fill sizes="100vw" preload />
        <div className={styles.heroCopy}>
          <span className={styles.eyebrow}>{t("Project intelligence. Local control.")}</span>
          <h1>CodeLocal</h1>
          <h2>{t("A shared intelligence layer for the way you build with AI.")}</h2>
          <p>{t("Connect your AI to one Project Brain. Keep execution on your machine.")}</p>
          <div className={styles.actions}>
            <Link className={styles.primaryButton} href="/dashboard">{t("Explore CodeLocal")} <AppIcon name="external" size={17} /></Link>
            <a className={styles.secondaryButton} href="#system">{t("See how it works")} <AppIcon name="chevron-down" size={17} /></a>
          </div>
        </div>
        <div className={styles.heroFoot}>
          <span><AppIcon name="shield" size={15} /> {t("Your workspace. Your permission.")}</span>
          <span>{t("Context / Execution / Memory")}</span>
        </div>
      </section>

      <section className={styles.clients} aria-label={t("Compatible clients, not endorsements")}>
        <span>{t("Built to connect with your AI")}</span>
        <div><strong>ChatGPT</strong><strong>Codex</strong><strong>Claude</strong><span>{t("Compatible MCP clients")} <AppIcon name="connection" size={18} /></span></div>
      </section>

      <section className={styles.principles} id="product">
        <div className={styles.container}>
          <div className={styles.sectionHeading}>
            <div><span className={styles.eyebrow}>01 / {t("The missing layer")}</span><h2>{t("Better AI. Not another fresh start.")}</h2></div>
            <p>{t("Models evolve. Tools change. Your project should keep learning.")}</p>
          </div>
          <div className={styles.principleGrid}>
            {principles.map((item) => (
              <article key={item.label}>
                <div><AppIcon name={item.icon} size={25} /><span>{t.message(item.label)}</span></div>
                <h3>{t.message(item.title)}</h3><p>{t.message(item.copy)}</p>
              </article>
            ))}
          </div>
        </div>
      </section>

      <section className={`${styles.container} ${styles.system}`} id="system">
        <div className={styles.sectionHeading}>
          <div><span className={styles.eyebrow}>02 / {t("One connected system")}</span><h2>{t("Intelligence shared.")}<br /><span>{t("Boundaries preserved.")}</span></h2></div>
          <p>{t("AI clients, cloud coordination and local execution. Connected through one project-aware system.")}</p>
        </div>
        <ArchitectureModel />
        <div className={styles.boundary}>
          <span><AppIcon name="shield" size={22} /> {t("Scoped by design.")}</span>
          <p>{t("Local tools operate on your machine. Cloud services coordinate identity, routing and sanitized project knowledge.")}</p>
          <Link href="/security">{t("Security model")} <AppIcon name="external" size={16} /></Link>
        </div>
      </section>

      <section className={styles.productBand}>
        <div className={`${styles.container} ${styles.productLayout}`}>
          <div>
            <span className={styles.eyebrow}>03 / {t("Product")}</span>
            <h2>{t("One place to see the whole project.")}</h2>
            <p>{t("Keep your connected workspaces, project knowledge and access controls in view.")}</p>
            <Link className={styles.primaryButton} href="/dashboard">{t("Open dashboard")} <AppIcon name="external" size={17} /></Link>
          </div>
          <nav className={styles.productLinks} aria-label={t("Explore the product")}>
            {productLinks.map((item) => (
              <Link href={item.href} key={item.href}>
                <AppIcon name={item.icon} size={25} />
                <div><h3>{t.message(item.title)}</h3><p>{t.message(item.copy)}</p></div>
                <AppIcon name="chevron-right" size={20} />
              </Link>
            ))}
          </nav>
        </div>
      </section>

      <section className={`${styles.container} ${styles.setup}`} id="how-it-works">
        <div className={styles.sectionHeading}>
          <div><span className={styles.eyebrow}>04 / {t("Get connected")}</span><h2>{t("Your machine. Three steps to connected.")}</h2></div>
          <Link className={styles.secondaryButton} href="/dashboard/connect">{t("Connection guide")} <AppIcon name="external" size={17} /></Link>
        </div>
        <ol className={styles.steps}>
          {steps.map((step, i) => (
            <li key={step.title}><span className={styles.stepNumber}>0{i + 1}</span><h3>{t.message(step.title)}</h3><p>{t.message(step.copy)}</p><code>{i === 2 ? t("One project. Shared context.") : step.command}</code></li>
          ))}
        </ol>
        <section className={styles.desktop} id="desktop" aria-labelledby="desktop-title">
          <AppIcon name="device" size={35} />
          <div><span className={styles.eyebrow}>{t("On the roadmap")}</span><h3 id="desktop-title">CodeLocal Desktop</h3><p>macOS · Windows · Linux</p></div>
          <span className={styles.roadmapStatus}>{t("In development")}</span>
        </section>
      </section>

      <section className={styles.closing}>
        <div className={styles.container}>
          <span className={styles.eyebrow}>{t("Build on what you know")}</span>
          <h2>{t("The next model will change.")}<br /><span>{t("Your project stays yours.")}</span></h2>
          <div className={styles.actions}>
            <Link className={styles.primaryButton} href="/dashboard">{t("Get started")} <AppIcon name="external" size={17} /></Link>
            <Link className={styles.secondaryButton} href="/support">{t("Talk to us")} <AppIcon name="chevron-right" size={17} /></Link>
          </div>
        </div>
      </section>
      <footer className={`${styles.container} ${styles.footer}`}>
        <Link className={styles.brand} href="/"><Image src="/codelocal-icon.png" alt="" width={28} height={28} />CodeLocal.</Link>
        <nav aria-label={t("Footer navigation")}><Link href="/blogs">{t("Journal")}</Link><Link href="/security">{t("Security")}</Link><Link href="/privacy">{t("Privacy")}</Link><Link href="/terms">{t("Terms")}</Link><Link href="/support">{t("Contact")}</Link><Link href="/login">{t("Sign in")}</Link></nav>
      </footer>
    </main>
  );
}
