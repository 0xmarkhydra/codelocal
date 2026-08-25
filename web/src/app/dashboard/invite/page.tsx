import dashboard from "../dashboard.module.css";
import { InviteLive } from "./invite-live";

export default function InvitePage() {
  return (
    <section className={dashboard.content}>
      <header className={dashboard.header}>
        <div>
          <span className={dashboard.eyebrow}>Team</span>
          <h1>Invite</h1>
          <p>Mời thành viên và theo dõi trạng thái tham gia.</p>
        </div>
      </header>
      <InviteLive />
    </section>
  );
}
