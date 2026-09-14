import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { notFound } from "next/navigation";
import { AppIcon } from "@/app/dashboard/app-icon";
import { forumExcerpt, getPublicForumTopic, publicForumImageURL } from "@/lib/forum-server";
import styles from "../forums.module.css";

type TopicPageProps = { params: Promise<{ topicID: string }> };

const statusLabels: Record<string, string> = {
  open: "Open",
  under_review: "Under review",
  planned: "Planned",
  in_progress: "In progress",
  resolved: "Resolved",
  closed: "Closed",
};

const kindLabels: Record<string, string> = {
  question: "Question / Problem",
  bug: "Bug Report",
  idea: "Idea / Feedback",
};

function dateTime(value: number) {
  return new Intl.DateTimeFormat("en", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

export async function generateMetadata({ params }: TopicPageProps): Promise<Metadata> {
  const { topicID } = await params;
  const resource = await getPublicForumTopic(topicID);
  if (!resource) return { title: "Forum topic not found", robots: { index: false, follow: false } };
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
          <Link className={styles.backLink} href="/forums"><AppIcon name="chevron-left" size={15} /> All discussions</Link>
          <div className={styles.badges}>
            <span data-kind={topic.kind}>{kindLabels[topic.kind]}</span>
            <span data-status={topic.status}>{statusLabels[topic.status]}</span>
            {topic.kind === "bug" && topic.severity && <span>{topic.severity}</span>}
          </div>
          <h1>{topic.title}</h1>
          <div className={styles.detailMeta}>
            <span>{topic.author}</span>
            <time dateTime={new Date(topic.createdAt).toISOString()}>{dateTime(topic.createdAt)}</time>
            <span>{topic.voteCount} votes</span>
            <span>{topic.commentCount} replies</span>
          </div>
        </div>
      </section>

      <section className={styles.articleBand}>
        <div className={`${styles.container} ${styles.articleLayout}`}>
          <article className={styles.articleBody}>
            <span className={styles.articleLabel}>Discussion</span>
            <p className={styles.bodyCopy}>{topic.body}</p>
            {topic.assetIds.length > 0 && (
              <div className={styles.gallery}>
                {topic.assetIds.map((assetID, index) => (
                  <a href={publicForumImageURL(assetID)} target="_blank" rel="noreferrer" key={assetID}>
                    <Image src={publicForumImageURL(assetID)} alt={`Attachment ${index + 1} for ${topic.title}`} width={1200} height={800} unoptimized />
                  </a>
                ))}
              </div>
            )}
            <div className={styles.tags}>{topic.tags.map((tag) => <span key={tag}>#{tag}</span>)}</div>
          </article>

          <aside className={styles.topicAside}>
            <span className={styles.articleLabel}>Thread</span>
            <dl>
              <div><dt>Status</dt><dd>{statusLabels[topic.status]}</dd></div>
              <div><dt>Type</dt><dd>{kindLabels[topic.kind]}</dd></div>
              <div><dt>Replies</dt><dd>{topic.commentCount}</dd></div>
              <div><dt>Votes</dt><dd>{topic.voteCount}</dd></div>
            </dl>
            <Link className={styles.darkButton} href={`/dashboard/forums/${topic.id}`}>Join the discussion <AppIcon name="external" size={15} /></Link>
          </aside>
        </div>
      </section>

      {topic.kind === "bug" && (
        <section className={styles.engineering}>
          <div className={styles.container}>
            <div className={styles.sectionHeading}>
              <div><span className={styles.eyebrow}>Engineering context</span><h2>From report to resolution.</h2></div>
              <p>Technical details stay secondary to the report itself, but remain available when they help reproduce and fix the issue.</p>
            </div>
            <div className={styles.bugGrid}>
              <div><strong>Version</strong><p>{topic.version || "Not provided"}</p></div>
              <div><strong>Environment</strong><p>{topic.environment || "Captured automatically or not provided"}</p></div>
              <div><strong>Steps to reproduce</strong><p>{topic.reproductionSteps || "Not provided"}</p></div>
              <div><strong>Expected behavior</strong><p>{topic.expectedBehavior || "Not provided"}</p></div>
              <div><strong>Actual behavior</strong><p>{topic.actualBehavior || "Not provided"}</p></div>
            </div>

            {(topic.githubIssueUrl || topic.githubPrUrl || topic.resolutionNote) && (
              <div className={styles.deliveryStrip}>
                <div><span>ENGINEERING HANDOFF</span><strong>{topic.resolutionNote ? "Resolution recorded" : topic.githubPrUrl ? "Fix in progress" : "Issue linked"}</strong></div>
                <div className={styles.githubLinks}>
                  {topic.githubIssueUrl && <a href={topic.githubIssueUrl} target="_blank" rel="noreferrer">GitHub Issue{topic.githubIssueNumber ? ` #${topic.githubIssueNumber}` : ""} <AppIcon name="external" size={14} /></a>}
                  {topic.githubPrUrl && <a href={topic.githubPrUrl} target="_blank" rel="noreferrer">Pull Request / Fix <AppIcon name="external" size={14} /></a>}
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
            <div><span className={styles.eyebrow}>Replies / {String(comments.length).padStart(2, "0")}</span><h2>Community context.</h2></div>
            <Link className={styles.secondaryButton} href={`/dashboard/forums/${topic.id}`}>Sign in to reply <AppIcon name="external" size={16} /></Link>
          </div>

          {comments.length === 0 ? (
            <div className={styles.empty}><span className={styles.eyebrow}>No replies yet</span><h2>Be the first to add context.</h2></div>
          ) : (
            <div className={styles.replyList}>
              {comments.map((comment, index) => (
                <article className={styles.reply} key={comment.id}>
                  <span className={styles.replyIndex}>{String(index + 1).padStart(2, "0")}</span>
                  <div>
                    <header><strong>{comment.author}</strong><time dateTime={new Date(comment.createdAt).toISOString()}>{dateTime(comment.createdAt)}</time></header>
                    <p>{comment.body}</p>
                    {comment.assetIds.length > 0 && (
                      <div className={styles.replyGallery}>
                        {comment.assetIds.map((assetID, imageIndex) => (
                          <a href={publicForumImageURL(assetID)} target="_blank" rel="noreferrer" key={assetID}>
                            <Image src={publicForumImageURL(assetID, "medium")} alt={`Reply attachment ${imageIndex + 1}`} width={640} height={420} unoptimized />
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
          <span className={styles.eyebrow}>Keep the loop moving</span>
          <h2>Know the answer?<br /><span>Add what you learned.</span></h2>
          <div className={styles.actions}>
            <Link className={styles.primaryButton} href={`/dashboard/forums/${topic.id}`}>Reply to this thread <AppIcon name="external" size={17} /></Link>
            <Link className={styles.secondaryButton} href="/forums">Back to Forums <AppIcon name="chevron-right" size={17} /></Link>
          </div>
        </div>
      </section>
    </main>
  );
}
