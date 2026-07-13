import { useEffect, useRef } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import type { VirtualMenuProps } from "./VirtualMenu";

// VirtualMenu is the shared scroll container for the composer's "/" and "@"
// dropdowns. Rows are virtualized so a directory with thousands of entries (or a
// long command list) only ever mounts the visible rows plus a small overscan —
// no truncation, no jank. The caller owns the item data, the active index, and
// per-row markup; this owns layout and keeping the active row in view.
export function VirtualMenuImpl<T>({
  items,
  activeIndex,
  itemKey,
  renderItem,
}: VirtualMenuProps<T>) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => 34,
    overscan: 10,
  });

  useEffect(() => {
    if (activeIndex >= 0 && activeIndex < items.length) {
      virtualizer.scrollToIndex(activeIndex, { align: "auto" });
    }
  }, [activeIndex, items.length, virtualizer]);

  return (
    <div ref={scrollRef} className="slashmenu" role="listbox">
      <div className="slashmenu__sizer" style={{ height: virtualizer.getTotalSize() }}>
        {virtualizer.getVirtualItems().map((row) => (
          <div
            key={itemKey(items[row.index], row.index)}
            data-index={row.index}
            ref={virtualizer.measureElement}
            className="slashmenu__row"
            style={{ transform: `translateY(${row.start}px)` }}
          >
            {renderItem(items[row.index], row.index)}
          </div>
        ))}
      </div>
    </div>
  );
}
