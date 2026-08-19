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
    <>
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
              Clear
            </button>
          )}
        </div>
        <span className={controls.listMeta}>{totalResults} result{totalResults === 1 ? "" : "s"}</span>
      </div>

      {totalPages > 1 && (
        <div className={controls.listPager}>
          <button className={controls.listButton} type="button" disabled={page <= 1} onClick={() => onPageChange(page - 1)}>Previous</button>
          <span>Page {page} of {totalPages}</span>
          <button className={controls.listButton} type="button" disabled={page >= totalPages} onClick={() => onPageChange(page + 1)}>Next</button>
        </div>
      )}
    </>
  );
}
