// @vitest-environment happy-dom
import { describe, expect, it, vi } from 'vitest'
import { measureContentHeight } from '~/tinycld/mail/lib/email-frame-height'

// The iframe used to be sized from documentElement.scrollHeight plus the body
// margins. The root's scrollHeight never drops below the frame's own height,
// so each re-measure grew the frame by the margins: a 91px email sat in a
// 332px frame, and a re-wrap that kept the ResizeObserver firing grew it every
// frame, flooding the dev terminal with "ResizeObserver loop" errors.

function frameDocument({
    contentHeight,
    frameHeight,
}: {
    contentHeight: number
    frameHeight: number
}) {
    const doc = document.implementation.createHTMLDocument('email')
    vi.spyOn(doc.documentElement, 'scrollHeight', 'get').mockReturnValue(
        Math.max(contentHeight, frameHeight)
    )
    vi.spyOn(doc.documentElement, 'getBoundingClientRect').mockReturnValue(
        DOMRect.fromRect({ width: 729, height: contentHeight })
    )
    return doc
}

describe('measureContentHeight', () => {
    it('measures the content, not the taller frame around it', () => {
        expect(measureContentHeight(frameDocument({ contentHeight: 119, frameHeight: 332 }))).toBe(
            119
        )
    })

    it('reaches a fixed point once the frame is sized to it', () => {
        let frameHeight = 300
        for (let i = 0; i < 5; i++) {
            frameHeight = measureContentHeight(frameDocument({ contentHeight: 119, frameHeight }))
        }
        expect(frameHeight).toBe(119)
    })

    it('rounds a fractional box up so the last line is not clipped', () => {
        expect(
            measureContentHeight(frameDocument({ contentHeight: 118.4, frameHeight: 300 }))
        ).toBe(119)
    })
})
