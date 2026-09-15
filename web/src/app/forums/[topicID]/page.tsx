import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { notFound } from "next/navigation";
import { AppIcon } from "@/app/dashboard/app-icon";
import { forumExcerpt, getPublicForumTopic, publicForumImageURL } from "@/lib/forum-server";
import { getLocale, getTranslations } from "@/lib/i18n/server";
import type { MessageKey } from "@/lib/i18n/messages";
import styles from "../forums.module.css";

type TopicPageProps = { params: Promise<{ topicID: string }> };

const statusLabels: Record<string, MessageKey> = {
  open: "Open",
  under_review: "Under review",
  planned: "Planned",
  in_progress: "In progress",
  resolved: "Resolved",
  closed: "Closed",
};

const kindLabels: Record<string, MessageKey> = {
  question: "Question / Problem",
  bug: "Bug Report",
  idea: "Idea / Feedback",
};

function dateTime(value: number, locale: string) {
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

export async function generateMetadata({ params }: TopicPageProps): Promise<Metadata> {
  const t = await getTranslations();
  const { topicID } = await params;
  const resource = await getPublicForumTopic(topicID);
  if (!resource) return { title: t("Forum topic not found"), robots: { index: false, follow: false } };
  const { topic } = resource;
  const description = forumExcerpt(topic.body, 180);
  return {
    title: topic.title,
    description,
    alternates: { canonical: `/forums/${topic.id}` },
    openGraph: {
      type: "article",
      title: topic.title,
      description,
      url: `/forums/${topic.id}`,
      publishedTime: new Date(topic.createdAt).toISOString(),
      modifiedTime: new Date(topic.updatedAt).toISOString(),
      tags: topic.tags,
      images: topic.assetIds[0] ? [{ url: publicForumImageURL(topic.assetIds[0]) }] : undefined,
    },
  };
}

export default async function ForumTopicPage({ params }: TopicPageProps) {
  const [t, locale] = await Promise.all([getTranslations(), getLocale()]);
  const { topicID } = await params;
  const resource = await getPublicForumTopic(topicID);
  if (!resource) notFound();
  const { topic, comments } = resource;
  const canonical = `https://codelocal.cloud/forums/${topic.id}`;
  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "DiscussionForumPosting",
    headline: topic.title,
    text: topic.body,
    url: canonical,
    mainEntityOfPage: canonical,
    datePublished: new Date(topic.createdAt).toISOString(),
    dateModified: new Date(topic.updatedAt).toISOString(),
    author: { "@type": "Person", name: topic.author },
    publisher: { "@type": "Organization", name: "CodeLocal", url: "https://codelocal.cloud" },
    image: topic.assetIds.map((assetID) => `https://codelocal.cloud${publicForumImageURL(assetID)}`),
    interactionStatistic: [
      { "@type": "InteractionCounter", interactionType: "https://schema.org/LikeAction", userInteractionCount: topic.voteCount },
      { "@type": "InteractionCounter", interactionType: "https://schema.org/CommentAction", userInteractionCount: topic.commentCount },
    ],
    comment: comments.slice(0, 50).map((comment) => ({
      "@type": "Comment",
      text: comment.body,
      dateCreated: new Date(comment.createdAt).toISOString(),
      author: { "@type": "Person", name: comment.author },
    })),
  };

  return (
    <main className={styles.page}>
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd).replace(/</g, "\\u003c") }} />

      <section className={styles.topicHero}>
        <div className={styles.container}>
          <Link className={styles.backLink} href="/forums"><AppIcon name="chevron-left" size={15} /> {t("All discussions")}</Link>
          <div className={styles.badges}>
            <span data-kind={topic.kind}>{t(kindLabels[topic.kind])}</span>
            <span data-status={topic.status}>{t(statusLabels[topic.status])}</span>
            {topic.kind === "bug" && topic.severity && <span>{topic.severity}</span>}
          </div>
          <h1>{topic.title}</h1>
          <div className={styles.detailMeta}>
            <span>{topic.author}</span>
            <time dateTime={new Date(topic.createdAt).toISOString()}>{dateTime(topic.createdAt, locale)}</time>
            <span>{t("{count} votes", { count: topic.voteCount })}</span>
            <span>{t("{count} replies", { count: topic.commentCount })}</span>
          </div>
        </div>
      </section>

      <section className={styles.articleBand}>
        <div className={`${styles.container} ${styles.articleLayout}`}>
          <article className={styles.articleBody}>
            <span className={styles.articleLabel}>{t("Discussion")}</span>
            <p className={styles.bodyCopy}>{topic.body}</p>
            {topic.assetIds.length > 0 && (
              <div className={styles.gallery}>
                {topic.assetIds.map((assetID, index) => (
                  <a href={publicForumImageURL(assetID)} target="_blank" rel="noreferrer" key={assetID}>
                    <Image src={publicForumImageURL(assetID)} alt={t("Attachment {count} for {name}", { count: index + 1, name: topic.title })} width={1200} height={800} unoptimized />
                  </a>
                ))}
              </div>
            )}
            <div className={styles.tags}>{topic.tags.map((tag) => <span key={tag}>#{tag}</span>)}</div>
          </article>

          <aside className={styles.topicAside}>
            <span className={styles.articleLabel}>{t("Thread")}</span>
            <dl>
              <div><dt>{t("Status")}</dt><dd>{t(statusLabels[topic.status])}</dd></div>
              <div><dt>{t("Type")}</dt><dd>{t(kindLabels[topic.kind])}</dd></div>
              <div><dt>{t("Replies")}</dt><dd>{topic.commentCount}</dd></div>
              <div><dt>{t("Votes")}</dt><dd>{topic.voteCount}</dd></div>
            </dl>
            <Link className={styles.darkButton} href={`/dashboard/forums/${topic.id}`}>{t("Join the discussion")} <AppIcon name="external" size={15} /></Link>
          </aside>
        </div>
      </section>

      {topic.kind === "bug" && (
        <section className={styles.engineering}>
          <div className={styles.container}>
            <div className={styles.sectionHeading}>
              <div><span className={styles.eyebrow}>{t("Engineering context")}</span><h2>{t("From report to resolution.")}</h2></div>
              <p>{t("Technical details stay secondary to the report itself, but remain available when they help reproduce and fix the issue.")}</p>
            </div>
            <div className={styles.bugGrid}>
              <div><strong>{t("Version")}</strong><p>{topic.version || t("Not provided")}</p></div>
              <div><strong>{t("Environment")}</strong><p>{topic.environment || t("Captured automatically or not provided")}</p></div>
              <div><strong>{t("Steps to reproduce")}</strong><p>{topic.reproductionSteps || t("Not provided")}</p></div>
              <div><strong>{t("Expected behavior")}</strong><p>{topic.expectedBehavior || t("Not provided")}</p></div>
              <div><strong>{t("Actual behavior")}</strong><p>{topic.actualBehavior || t("Not provided")}</p></div>
            </div>

            {(topic.githubIssueUrl || topic.githubPrUrl || topic.resolutionNote) && (
              <div className={styles.deliveryStrip}>
                <div><span>{t("ENGINEERING HANDOFF")}</span><strong>{t(topic.resolutionNote ? "Resolution recorded" : topic.githubPrUrl ? "Fix in progress" : "Issue linked")}</strong></div>
                <div className={styles.githubLinks}>
                  {topic.githubIssueUrl && <a href={topic.githubIssueUrl} target="_blank" rel="noreferrer">{t("GitHub Issue")}{topic.githubIssueNumber ? ` #${topic.githubIssueNumber}` : ""} <AppIcon name="external" size={14} /></a>}
                  {topic.githubPrUrl && <a href={topic.githubPrUrl} target="_blank" rel="noreferrer">{t("Pull Request / Fix")} <AppIcon name="external" size={14} /></a>}
                </div>
                {topic.resolutionNote && <p>{topic.resolutionNote}</p>}
              </div>
            )}
          </div>
        </section>
      )}

      <section className={styles.repliesSection}>
        <div className={styles.container}>
          <div className={styles.sectionHeading}>
            <div><span className={styles.eyebrow}>{t("Replies")} / {String(comments.length).padStart(2, "0")}</span><h2>{t("Community context.")}</h2></div>
            <Link className={styles.secondaryButton} href={`/dashboard/forums/${topic.id}`}>{t("Sign in to reply")} <AppIcon name="external" size={16} /></Link>
          </div>

          {comments.length === 0 ? (
            <div className={styles.empty}><span className={styles.eyebrow}>{t("No replies yet")}</span><h2>{t("Be the first to add context.")}</h2></div>
          ) : (
            <div className={styles.replyList}>
              {comments.map((comment, index) => (
                <article className={styles.reply} key={comment.id}>
                  <span className={styles.replyIndex}>{String(index + 1).padStart(2, "0")}</span>
                  <div>
                    <header><strong>{comment.author}</strong><time dateTime={new Date(comment.createdAt).toISOString()}>{dateTime(comment.createdAt, locale)}</time></header>
                    <p>{comment.body}</p>
                    {comment.assetIds.length > 0 && (
                      <div className={styles.replyGallery}>
                        {comment.assetIds.map((assetID, imageIndex) => (
                          <a href={publicForumImageURL(assetID)} target="_blank" rel="noreferrer" key={assetID}>
                            <Image src={publicForumImageURL(assetID, "medium")} alt={t("Reply attachment {count}", { count: imageIndex + 1 })} width={640} height={420} unoptimized />
                          </a>
                        ))}
                      </div>
                    )}
                  </div>
                </article>
              ))}
            </div>
          )}
        </div>
      </section>

      <section className={styles.detailClosing}>
        <div className={styles.container}>
          <span className={styles.eyebrow}>{t("Keep the loop moving")}</span>
          <h2>{t("Know the answer?")}<br /><span>{t("Add what you learned.")}</span></h2>
          <div className={styles.actions}>
            <Link className={styles.primaryButton} href={`/dashboard/forums/${topic.id}`}>{t("Reply to this thread")} <AppIcon name="external" size={17} /></Link>
            <Link className={styles.secondaryButton} href="/forums">{t("Back to Forums")} <AppIcon name="chevron-right" size={17} /></Link>
          </div>
        </div>
      </section>
    </main>
  );
}
