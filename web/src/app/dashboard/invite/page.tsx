import dashboard from "../dashboard.module.css";
import { InviteLive } from "./invite-live";

export default function InvitePage() {
  return <section className={dashboard.content}>
    <header className={dashboard.header}><div><span className={dashboard.eyebrow}>Referral network</span><h1>Invite</h1><p>Manage your invite code and see the people who joined through you.</p></div><span className={dashboard.productionLink}>Emails stay masked</span></header>
    <InviteLive />
  </section>;
}
