/// <reference path="../../../server/pb_data/types.d.ts" />
migrate(
    app => {
        const c = app.findCollectionByNameOrId('mail_messages')
        if (!c.fields.getByName('delivered_at')) {
            c.fields.add(new DateField({ name: 'delivered_at' }))
        }
        if (!c.fields.getByName('bounce_class')) {
            c.fields.add(new SelectField({ name: 'bounce_class', maxSelect: 1, values: ['soft', 'hard', 'complaint'] }))
        }
        app.save(c)
    },
    app => {
        const c = app.findCollectionByNameOrId('mail_messages')
        c.fields.removeByName('delivered_at')
        c.fields.removeByName('bounce_class')
        app.save(c)
    }
)
