/// <reference path="../../../server/pb_data/types.d.ts" />
migrate(
    app => {
        const domains = app.findCollectionByNameOrId('mail_domains')

        // Per-provider state for this domain: the provider's own id, and the
        // verification answer it last gave, stamped with when.
        //
        //   {"postmark":{"domain_id":12345,"checked_at":"2026-09-16T14:02:11Z",
        //    "enrolled":true,"spf_verified":false,"dkim_verified":true,
        //    "return_path_verified":true}}
        //
        // The id matters because Postmark has no lookup-by-name: without it a
        // status check must page the whole account's domain list and reports a
        // domain past the first page as unenrolled.
        //
        // The status is kept for reporting, NOT to render this domain's UI —
        // the row's own *_verified columns do that. This is the provider's
        // answer with a timestamp, which those columns cannot express: they
        // carry no "when", and no "enrolled" at all.
        //
        // JSON rather than scalar columns so a second provider, or more
        // per-provider state, needs no further migration.
        domains.fields.add(
            new Field({
                id: 'mail_domains_provider_metadata',
                name: 'provider_domain_metadata',
                type: 'json',
                maxSize: 2000,
            })
        )

        app.save(domains)
    },
    app => {
        const domains = app.findCollectionByNameOrId('mail_domains')
        domains.fields.removeById('mail_domains_provider_metadata')
        app.save(domains)
    }
)
