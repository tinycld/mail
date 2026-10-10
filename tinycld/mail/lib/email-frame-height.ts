// The <html> box (height:auto) is the content alone, body margins and
// collapsed child margins included. The root's scrollHeight is not: it never
// drops below the frame's own height, so sizing the frame from it can only
// grow it, and each observer callback that added the body margins on top grew
// it again — once per frame when a re-wrap kept the observer firing.
export function measureContentHeight(doc: Document) {
    return Math.ceil(doc.documentElement.getBoundingClientRect().height)
}
