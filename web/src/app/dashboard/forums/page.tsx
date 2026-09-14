import dashboard from "../dashboard.module.css";
import { ForumsHub } from "./forums-hub";

export default function ForumsPage() {
  return (
    <section className={dashboard.content}>
      <ForumsHub />
    </section>
  );
}
