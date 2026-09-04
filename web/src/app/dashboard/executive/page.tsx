import { Suspense } from "react";
import { ExecutiveHome } from "./executive-home";

export default function ExecutivePage() {
  return (
    <section style={{ padding: 20, maxWidth: 960 }}>
      <Suspense fallback={<div>Đang mở Executive Home…</div>}>
        <ExecutiveHome />
      </Suspense>
    </section>
  );
}
