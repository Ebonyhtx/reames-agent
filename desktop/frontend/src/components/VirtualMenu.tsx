import { lazy, Suspense, type ReactNode } from "react";
import { useT } from "../lib/i18n";

export interface VirtualMenuProps<T> {
  items: T[];
  activeIndex: number;
  itemKey: (item: T, index: number) => string;
  renderItem: (item: T, index: number) => ReactNode;
}

const VirtualMenuImpl = lazy(() => import("./VirtualMenuImpl").then((module) => ({ default: module.VirtualMenuImpl }))) as <T>(props: VirtualMenuProps<T>) => ReactNode;

// Keep the small menu boundary in the app shell while loading TanStack's
// virtualizer only when a slash/file-reference menu actually opens.
export function VirtualMenu<T>(props: VirtualMenuProps<T>) {
  const t = useT();
  return (
    <Suspense
      fallback={(
        <div className="slashmenu" role="listbox" aria-busy="true">
          <div className="slashmenu__item slashmenu__item--empty">
            <span className="slashmenu__name">{t("common.loading")}</span>
          </div>
        </div>
      )}
    >
      <VirtualMenuImpl {...props} />
    </Suspense>
  );
}
