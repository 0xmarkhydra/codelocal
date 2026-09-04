import { Suspense } from "react";
import { WorkBoard } from "./work-board";

export default function WorkPage() {
  return (
    <section style={{ padding: 20, maxWidth: 960 }}>
      <Suspense fallback={<div>Đang tải Work…</div>}>
        <WorkBoard />
      </Suspense>
    </section>
  );
}
