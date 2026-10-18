import { describe, expect, it } from 'vitest'
import { visibleAddressViews } from '~/tinycld/mail/hooks/useAddressViews'

const main = { id: 'a1', email: 'nathan@tinycld.com', rank: 1 }
const second = { id: 'a2', email: 'nathan@q.example', rank: 2 }
const third = { id: 'a3', email: 'sales@tinycld.com', rank: 1 }

describe('visibleAddressViews', () => {
    it('with the switch off there are no views', () => {
        expect(
            visibleAddressViews({ enabled: false, hidden: [], addresses: [main, second] })
        ).toEqual([])
    })

    it('one visible address is the whole inbox, so there are no views', () => {
        expect(visibleAddressViews({ enabled: true, hidden: [], addresses: [main] })).toEqual([])
        expect(
            visibleAddressViews({ enabled: true, hidden: ['a2'], addresses: [main, second] })
        ).toEqual([])
    })

    it('two addresses give two views, ordered by domain rank', () => {
        expect(
            visibleAddressViews({ enabled: true, hidden: [], addresses: [second, main] })
        ).toEqual([
            { id: 'a1', email: 'nathan@tinycld.com' },
            { id: 'a2', email: 'nathan@q.example' },
        ])
    })

    it('addresses of one rank are ordered by local part', () => {
        expect(
            visibleAddressViews({ enabled: true, hidden: [], addresses: [third, second, main] })
        ).toEqual([
            { id: 'a1', email: 'nathan@tinycld.com' },
            { id: 'a3', email: 'sales@tinycld.com' },
            { id: 'a2', email: 'nathan@q.example' },
        ])
    })

    it('a hidden address is left out', () => {
        expect(
            visibleAddressViews({ enabled: true, hidden: ['a1'], addresses: [main, second, third] })
        ).toEqual([
            { id: 'a3', email: 'sales@tinycld.com' },
            { id: 'a2', email: 'nathan@q.example' },
        ])
    })
})
