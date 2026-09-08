"use client";

import { useEffect } from "react";
import { useTranslations } from "@/lib/i18n/provider";

export function useUnsavedWarning(pending: boolean) {
  const { t } = useTranslations();
  const warning = t("Unsaved changes will be lost. Leave this page?");
  useEffect(() => {
    if (!pending) return;
    const unload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    const navigate = (event: MouseEvent) => {
      if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      const anchor = event.target instanceof Element ? event.target.closest("a[href]") : null;
      if (!(anchor instanceof HTMLAnchorElement) || anchor.hasAttribute("download") || (anchor.target && anchor.target !== "_self")) return;
      const next = new URL(anchor.href);
      const current = new URL(window.location.href);
      if (next.origin === current.origin && next.pathname === current.pathname && next.search === current.search) return;
      if (!window.confirm(warning)) {
        event.preventDefault();
        event.stopPropagation();
      }
    };
    // ponytail: Guard page exits and links; same-document browser history needs a router blocker.
    window.addEventListener("beforeunload", unload);
    document.addEventListener("click", navigate, true);
    return () => {
      window.removeEventListener("beforeunload", unload);
      document.removeEventListener("click", navigate, true);
    };
  }, [pending, warning]);
}
