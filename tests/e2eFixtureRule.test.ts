import { readdirSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

// CLAUDE.md's "e2e never writes data directly" rule now carries one narrow
// exception: a fixture whose creation path cannot run in CI may be seeded by
// a superuser write, PROVIDED the spec's header says why the path is
// unreachable and what it therefore does not cover.
//
// The seeding half of that bargain enforces itself — the spec does not run
// without it. The documenting half is the part that rots: a later author
// copies a seeding spec, keeps the write, drops the explanation, and the
// exception quietly widens into the rule. This test keeps it honest by
// failing any spec that takes the shortcut without paying for it.

const TESTS_DIR = import.meta.dirname
const HEADER_LINES = 40

// A superuser write, not merely a superuser read: mail-attachments.spec.ts and
// helpers.ts authenticate as the superuser only to look up a webhook secret,
// which the rule permits. The exception is about creating or changing rows.
const WRITE_CALL = /\.(create|update|delete)\s*\(/

// Deliberately loose about wording — a future author will phrase the
// justification their own way — but not so loose that incidental prose
// satisfies it. Each alternative names an ABSENCE, so a passing header has
// to actually assert that something is missing or impossible; a bare noun
// like "provider" appearing in unrelated commentary is not enough.
const WHY_UNREACHABLE = new RegExp(
    [
        'not available',
        '(?:is|are|was|were)n.t available',
        'unavailable',
        'unreachable',
        'cannot (?:run|succeed|be|create|reach)',
        'can.t (?:run|succeed|be|create|reach)',
        'always fails',
        'no (?:real |provider |valid )?(?:credential|account|inbox|mailbox|key|token)',
        'without (?:provider |real |valid )?credentials',
        '(?:no|not) .{0,40}\\bin CI\\b',
        '\\bCI\\b .{0,40}(?:has no|lacks|does not have|cannot)',
    ].join('|'),
    'i'
)
const WHAT_IS_UNCOVERED = new RegExp(
    [
        'do(?:es)? ?n.t cover',
        'does not cover',
        'not cover(?:ed|age)?',
        'do(?:es)? ?n.t exercise',
        'does not exercise',
        '(?:is |are )?not exercised',
        'not end-to-end',
        'leaves? .{0,30}uncovered',
        'uncovered',
    ].join('|'),
    'i'
)

function specFiles() {
    return readdirSync(TESTS_DIR)
        .filter(name => name.endsWith('.spec.ts'))
        .sort()
}

function readSpec(name: string) {
    return readFileSync(path.join(TESTS_DIR, name), 'utf8')
}

function seedsWithSuperuserWrite(source: string) {
    return source.includes('_superusers') && WRITE_CALL.test(source)
}

function headerComment(source: string) {
    return source
        .split('\n')
        .slice(0, HEADER_LINES)
        .filter(line => line.trimStart().startsWith('//'))
        .join('\n')
}

function ruleReminder(file: string, missing: string) {
    return [
        `${file} performs a superuser PocketBase write but its first ${HEADER_LINES} lines`,
        `do not explain ${missing}.`,
        '',
        'CLAUDE.md: "The one exception: a fixture whose creation path cannot run in CI',
        'may be seeded by a superuser write. [...] A spec taking this exception must say',
        'in its header comment *why* the path is unreachable and *what it therefore does',
        'not cover*, so nobody reads it as end-to-end coverage of the write it skipped."',
        '',
        `Add a header comment to ${file} covering both halves, or seed the fixture by`,
        'driving the UI instead.',
    ].join('\n')
}

describe('e2e superuser-seeding exception', () => {
    const seedingSpecs = specFiles().filter(name => seedsWithSuperuserWrite(readSpec(name)))

    it('finds the specs that take the exception', () => {
        // A guard on the detector itself: if this drops to zero the assertions
        // below start vacuously passing and pin nothing.
        expect(seedingSpecs.length).toBeGreaterThan(0)
    })

    it.each(seedingSpecs)('%s documents why the real path is unreachable', name => {
        const header = headerComment(readSpec(name))
        expect(
            WHY_UNREACHABLE.test(header),
            ruleReminder(name, 'WHY the real creation path is unreachable in CI')
        ).toBe(true)
    })

    it.each(seedingSpecs)('%s documents what it therefore does not cover', name => {
        const header = headerComment(readSpec(name))
        expect(
            WHAT_IS_UNCOVERED.test(header),
            ruleReminder(name, 'WHAT the seeded write therefore leaves uncovered')
        ).toBe(true)
    })
})
