// Auto-opening the attachment panel on a viewport barely taller than the
// panel itself (PANEL_MAX_HEIGHT in AttachmentStrip.tsx) leaves no room for
// the message. iPhone landscape (~390-430pt tall) falls under this threshold;
// iPad landscape and any desktop window clear it.
export const MIN_HEIGHT_FOR_AUTO_OPEN = 500

/**
 * Whether the scroll-driven auto-open may fire. Suppressed on a viewport too
 * short to show both the panel and the message, and permanently once the user
 * has worked the header themselves.
 *
 * Lives apart from AttachmentStrip.tsx so it stays importable without the
 * file-viewer/PreviewModal tree, which needs the native runtime to load.
 */
export function canAutoOpenStrip(windowHeight: number, userToggled: boolean) {
    return windowHeight >= MIN_HEIGHT_FOR_AUTO_OPEN && !userToggled
}
