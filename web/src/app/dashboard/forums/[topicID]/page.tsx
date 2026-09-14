import dashboard from "../../dashboard.module.css";
import { ForumTopicLive } from "../forum-topic-live";

export default async function ForumTopicPage({ params }: { params: Promise<{ topicID: string }> }) {
  const { topicID } = await params;
  return (
    <section className={dashboard.content}>
      <ForumTopicLive topicID={topicID} />
    </section>
  );
}
