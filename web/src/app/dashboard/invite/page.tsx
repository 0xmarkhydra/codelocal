import dashboard from "../dashboard.module.css";
import { InviteLive } from "./invite-live";

export default function InvitePage() {
  return (
    <section className={dashboard.content}>
      <header className={dashboard.header}><div><h1>Invite</h1></div></header>
      <InviteLive />
    </section>
  );
}
