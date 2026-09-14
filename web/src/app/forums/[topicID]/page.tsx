import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { notFound } from "next/navigation";
import { forumExcerpt, getPublicForumTopic, publicForumImageURL } from "@/lib/forum-server";
import styles from "../forums.module.css";

type TopicPageProps = { params: Promise<{ topicID: string }> };

const statusLabels: Record<string, string> = { open: "Open", under_review: "Under review", planned: "Planned", in_progress: "In progress", resolved: "Resolved", closed: "Closed" };
const kindLabels: Record<string, string> = { question: "Question / Problem", bug: "Bug Report", idea: "Idea / Feedback" };

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
    comment: comments.slice(0, 50).map((comment) => ({ "@type": "Comment", text: comment.body, dateCreated: new Date(comment.createdAt).toISOString(), author: { "@type": "Person", name: comment.author } })),
  };

  return (
    <main className={`${styles.page} ${styles.detailPage}`}>
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd).replace(/</g, "\\u003c") }} />
      <Link className={styles.back} href="/forums">← All discussions</Link>
      <article className={styles.detailCard}>
        <div className={styles.badges}><span data-kind={topic.kind}>{kindLabels[topic.kind]}</span><span data-status={topic.status}>{statusLabels[topic.status]}</span>{topic.kind === "bug" && topic.severity && <span>{topic.severity}</span>}</div>
        <h1>{topic.title}</h1>
        <div className={styles.detailMeta}><span>{topic.author}</span><time dateTime={new Date(topic.createdAt).toISOString()}>{dateTime(topic.createdAt)}</time><span>{topic.voteCount} votes</span><span>{topic.commentCount} replies</span></div>
        <p className={styles.bodyCopy}>{topic.body}</p>
        {topic.assetIds.length > 0 && <div className={styles.gallery}>{topic.assetIds.map((assetID, index) => <a href={publicForumImageURL(assetID)} target="_blank" rel="noreferrer" key={assetID}><Image src={publicForumImageURL(assetID)} alt={`Attachment ${index + 1} for ${topic.title}`} width={1200} height={800} unoptimized /></a>)}</div>}
        <div className={styles.tags}>{topic.tags.map((tag) => <span key={tag}>#{tag}</span>)}</div>

        {topic.kind === "bug" && <section className={styles.bugGrid} aria-label="Bug details">
          <div><strong>Version</strong><p>{topic.version || "Not provided"}</p></div>
          <div><strong>Environment</strong><p>{topic.environment || "Not provided"}</p></div>
          <div><strong>Steps to reproduce</strong><p>{topic.reproductionSteps || "Not provided"}</p></div>
          <div><strong>Expected behavior</strong><p>{topic.expectedBehavior || "Not provided"}</p></div>
          <div><strong>Actual behavior</strong><p>{topic.actualBehavior || "Not provided"}</p></div>
        </section>}

        {(topic.githubIssueUrl || topic.githubPrUrl) && <div className={styles.githubLinks}>{topic.githubIssueUrl && <a href={topic.githubIssueUrl} target="_blank" rel="noreferrer">GitHub Issue{topic.githubIssueNumber ? ` #${topic.githubIssueNumber}` : ""} ↗</a>}{topic.githubPrUrl && <a href={topic.githubPrUrl} target="_blank" rel="noreferrer">Pull Request / Fix ↗</a>}</div>}
        {topic.resolutionNote && <aside className={styles.resolution}><strong>Resolution</strong><p>{topic.resolutionNote}</p></aside>}
      </article>

      <section className={styles.replies}>
        <div className={styles.repliesHead}><h2>Replies <span>{comments.length}</span></h2><Link href={`/dashboard/forums/${topic.id}`}>Sign in to reply</Link></div>
        {comments.length === 0 ? <div className={styles.empty}><p>No replies yet.</p></div> : comments.map((comment) => <article className={styles.reply} key={comment.id}><header><strong>{comment.author}</strong><time dateTime={new Date(comment.createdAt).toISOString()}>{dateTime(comment.createdAt)}</time></header><p>{comment.body}</p>{comment.assetIds.length > 0 && <div className={styles.replyGallery}>{comment.assetIds.map((assetID, index) => <a href={publicForumImageURL(assetID)} target="_blank" rel="noreferrer" key={assetID}><Image src={publicForumImageURL(assetID, "medium")} alt={`Reply attachment ${index + 1}`} width={640} height={420} unoptimized /></a>)}</div>}</article>)}
      </section>
    </main>
  );
}
