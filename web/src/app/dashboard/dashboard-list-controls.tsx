"use client";

import controls from "./dashboard-controls.module.css";

export const dashboardPageSize = 12;

type DashboardListControlsProps = {
  query: string;
  onQueryChange: (value: string) => void;
  page: number;
  totalPages: number;
  totalResults: number;
  onPageChange: (page: number) => void;
  placeholder: string;
};

export function DashboardListControls({
  query,
  onQueryChange,
  page,
  totalPages,
  totalResults,
  onPageChange,
  placeholder,
}: DashboardListControlsProps) {
  return (
    <div className={controls.listControls}>
      <div className={controls.listToolbar}>
        <div className={controls.listSearch}>
          <input
            type="search"
            value={query}
            placeholder={placeholder}
            aria-label={placeholder}
            onChange={(event) => {
              onQueryChange(event.target.value);
              onPageChange(1);
            }}
          />
          {query && (
            <button className={controls.listButton} type="button" onClick={() => { onQueryChange(""); onPageChange(1); }}>
              Xóa
            </button>
          )}
        </div>
        <span className={controls.listMeta}>{totalResults} kết quả</span>
      </div>

      {totalPages > 1 && (
        <div className={controls.listPager}>
          <button className={controls.listButton} type="button" disabled={page <= 1} onClick={() => onPageChange(page - 1)}>Trước</button>
          <span>Trang {page}/{totalPages}</span>
          <button className={controls.listButton} type="button" disabled={page >= totalPages} onClick={() => onPageChange(page + 1)}>Sau</button>
        </div>
      )}
    </div>
  );
}
