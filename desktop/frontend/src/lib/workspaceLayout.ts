export const COMPACT_WORKSPACE_MAX_WIDTH = 820;

export function resolveWorkspacePanelPresentation({
  viewportWidth,
  open,
  maximized,
  dockedWidth,
  minRenderWidth,
}: {
  viewportWidth: number;
  open: boolean;
  maximized: boolean;
  dockedWidth: number;
  minRenderWidth: number;
}): {
  compact: boolean;
  renderWidth: number;
  renderable: boolean;
  gridOpen: boolean;
} {
  const compact = viewportWidth <= COMPACT_WORKSPACE_MAX_WIDTH;
  const renderWidth = compact ? Math.max(0, viewportWidth) : dockedWidth;
  const renderable = open && (compact || maximized || renderWidth >= minRenderWidth);
  return {
    compact,
    renderWidth,
    renderable,
    gridOpen: renderable && !compact && !maximized,
  };
}

export function availableWorkspacePanelWidth({
  viewportWidth,
  sidebarCollapsed,
  sidebarWidth,
  chatMinWidth,
  resizerWidth,
}: {
  viewportWidth: number;
  sidebarCollapsed: boolean;
  sidebarWidth: number;
  chatMinWidth: number;
  resizerWidth: number;
}): number {
  return Math.max(0, viewportWidth - (sidebarCollapsed ? 0 : sidebarWidth) - chatMinWidth - resizerWidth);
}

export function resolveWorkspacePanelWidth({
  open,
  maximized,
  preferredWidth,
  minWidth,
  availableWidth,
}: {
  open: boolean;
  maximized: boolean;
  preferredWidth: number;
  minWidth: number;
  availableWidth: number;
}): number {
  if (!open || maximized) return preferredWidth;
  return Math.min(Math.max(minWidth, preferredWidth), Math.max(0, availableWidth));
}

export function resolveLiveWorkspacePanelWidth({
  viewportWidth,
  sidebarCollapsed,
  sidebarWidth,
  chatMinWidth,
  resizerWidth,
  open,
  maximized,
  preferredWidth,
  minWidth,
}: {
  viewportWidth: number;
  sidebarCollapsed: boolean;
  sidebarWidth: number;
  chatMinWidth: number;
  resizerWidth: number;
  open: boolean;
  maximized: boolean;
  preferredWidth: number;
  minWidth: number;
}): number {
  return resolveWorkspacePanelWidth({
    open,
    maximized,
    preferredWidth,
    minWidth,
    availableWidth: availableWorkspacePanelWidth({
      viewportWidth,
      sidebarCollapsed,
      sidebarWidth,
      chatMinWidth,
      resizerWidth,
    }),
  });
}

export function workspacePanelAriaMinWidth(minWidth: number, renderedWidth: number): number {
  return Math.min(minWidth, renderedWidth);
}
