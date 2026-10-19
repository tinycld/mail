/// <reference path="../../../server/pb_data/types.d.ts" />

// The "A message arrives" trigger offered its receiving address as `alias`,
// but inbound mail never sets alias (only mail sent from a legacy alias does).
// So a saved condition on it never tested the real address: `is` never
// matched, and `is_not` matched every message. The trigger now reads
// delivered_to, and the engine refuses a condition on a field the trigger
// does not expose, so a condition left on alias would silently never match.
//
// This moves each such condition to delivered_to. An `is` / `is_not` value is
// a mail_mailbox_aliases id; it becomes the id of the mail_addresses row the
// alias was copied to (1830000014, and the server's legacy-alias mirror). A
// value with no such row (the alias was deleted) stays as it is: it names no
// address, so `is` still never matches and `is_not` still always matches.
//
// `is_empty` on alias was meant as "not received at an alias", which a
// condition on delivered_to cannot say. It moves to delivered_to unchanged so
// the rule builder shows it to its owner; it no longer matches, because every
// arriving message has a receiving address.
migrate(
    app => {
        const addressOfAlias = aliasId => {
            const row = new DynamicModel({ id: '' })
            try {
                app.db()
                    .newQuery(
                        `SELECT x.id FROM mail_mailbox_aliases a
                         JOIN mail_mailboxes mb ON mb.id = a.mailbox
                         JOIN mail_addresses x ON x.mailbox = a.mailbox
                            AND x.local_part = lower(trim(a.address))
                            AND x.source = 'manual' AND x.is_primary = FALSE
                         WHERE a.id = {:alias}
                         ORDER BY (x.domain = mb.domain) DESC
                         LIMIT 1`
                    )
                    .bind({ alias: aliasId })
                    .one(row)
            } catch (_) {
                return ''
            }
            return row.id
        }

        const moveCondition = condition => {
            if (!condition || condition.field !== 'alias') return false
            condition.field = 'delivered_to'
            if (typeof condition.value === 'string' && condition.value !== '') {
                const addressId = addressOfAlias(condition.value)
                if (addressId) condition.value = addressId
            }
            return true
        }

        // Mail's migrations can run without core's (the package's own RLS
        // tests apply them alone), and with no rules there is nothing to move.
        try {
            app.findCollectionByNameOrId('rules')
        } catch (_) {
            return
        }
        const rules = app.findRecordsByFilter('rules', 'trigger = {:trigger}', '', 0, 0, {
            trigger: 'mail:message-received',
        })
        for (const rule of rules) {
            const raw = rule.getString('conditions')
            if (!raw || raw === 'null') continue
            const conditions = JSON.parse(raw)
            let changed = false
            for (const group of conditions.groups || []) {
                for (const condition of group.conditions || []) {
                    if (moveCondition(condition)) changed = true
                }
            }
            if (!changed) continue
            rule.set('conditions', conditions)
            app.save(rule)
        }
    },
    () => {
        // Not reversed: a condition on alias never tested the real receiving
        // address, so moving it back would only break it again.
    }
)
