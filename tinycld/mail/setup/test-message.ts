// The first message a new address sends. It lands in the person's own inbox
// at the end of setup, so it is written as a finish line, not a probe.
// Email clients ignore stylesheets, so every style is inline and the layout
// is tables; the colors match core's transactional emails (mailer.BrandColor).

const BRAND = '#0d9488'
const BRAND_SOFT = '#ccfbf1'
const INK = '#1c1917'
const INK_SOFT = '#44403c'
const MUTED = '#78716c'
const PAGE = '#f5f5f4'

const DONE_STEPS = [
    'Your domain is verified for sending and receiving',
    'Your address is set up',
    'Your first message is delivered',
]

function escapeHtml(value: string): string {
    return value
        .replaceAll('&', '&amp;')
        .replaceAll('<', '&lt;')
        .replaceAll('>', '&gt;')
        .replaceAll('"', '&quot;')
}

function doneStepRow(step: string): string {
    return `<tr>
  <td width="28" valign="top" style="padding:6px 0;color:${BRAND};font-size:16px;font-weight:700;">&#10003;</td>
  <td style="padding:6px 0;font-size:15px;line-height:1.5;color:${INK_SOFT};">${escapeHtml(step)}</td>
</tr>`
}

export function testMessageSubject(workspace: string): string {
    return `It works! Email is ready for ${workspace}`
}

export function testMessageText(workspace: string, fromAddress: string): string {
    const steps = DONE_STEPS.map(step => `  ✓ ${step}`).join('\n')
    return `It works!

${fromAddress} just sent its first message, and you are reading it. Email for ${workspace} is ready.

${steps}

Your team can now send and receive email at your own domain.`
}

export function testMessageHtml(workspace: string, fromAddress: string): string {
    const name = escapeHtml(workspace)
    const address = escapeHtml(fromAddress)
    const steps = DONE_STEPS.map(doneStepRow).join('\n')
    return `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>It works!</title>
</head>
<body style="margin:0;padding:0;background:${PAGE};font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;color:${INK};">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:${PAGE};padding:40px 16px;">
    <tr>
      <td align="center">
        <table role="presentation" width="560" cellpadding="0" cellspacing="0" border="0" style="max-width:560px;width:100%;background:#ffffff;border-radius:12px;box-shadow:0 1px 3px rgba(0,0,0,0.08);overflow:hidden;">
          <tr>
            <td align="center" style="padding:40px 40px 8px 40px;border-top:4px solid ${BRAND};">
              <table role="presentation" cellpadding="0" cellspacing="0" border="0">
                <tr>
                  <td align="center" width="64" height="64" style="width:64px;height:64px;border-radius:32px;background:${BRAND_SOFT};color:${BRAND};font-size:32px;font-weight:700;line-height:64px;">&#10003;</td>
                </tr>
              </table>
              <p style="margin:20px 0 4px 0;font-size:13px;color:${MUTED};letter-spacing:0.5px;text-transform:uppercase;font-weight:600;">Email is set up &middot; ${name}</p>
              <h1 style="margin:0;font-size:30px;line-height:1.25;font-weight:700;color:${INK};">It works!</h1>
            </td>
          </tr>
          <tr>
            <td align="center" style="padding:16px 40px 8px 40px;">
              <p style="margin:0 0 16px 0;font-size:16px;line-height:1.6;color:${INK_SOFT};">Your new address just sent its first message, and you are reading it.</p>
              <span style="display:inline-block;padding:10px 18px;border-radius:999px;background:${BRAND_SOFT};color:${BRAND};font-size:16px;font-weight:600;">${address}</span>
            </td>
          </tr>
          <tr>
            <td style="padding:24px 40px 32px 40px;">
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
                ${steps}
              </table>
            </td>
          </tr>
          <tr>
            <td style="padding:20px 40px;border-top:1px solid #e7e5e4;background:#fafaf9;">
              <p style="margin:0;font-size:13px;line-height:1.6;color:${MUTED};">Your team can now send and receive email at your own domain. Go back to setup to finish the last steps.</p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`
}
