import { Suspense } from "react";
import { CompanyView } from "./company-view";

export default function CompanyPage() {
  return (
    <section style={{ padding: 20, maxWidth: 960 }}>
      <Suspense fallback={<div>Đang tải Company…</div>}>
        <CompanyView />
      </Suspense>
    </section>
  );
}
