# Notifications

IPChronicle records address, complete-probe, history-gap, and upstream-format
events before it evaluates notification rules. A rule selects one event type,
one sender, and optional known-field, node, and network-egress filters. Enabled
rules are evaluated when the durable event is processed, not when the event was
first observed.

When several rules select the same event and sender, IPChronicle creates one
delivery and records every matching rule on it. Delivery and event identifiers
are stable, so restarting the Center does not intentionally duplicate work.
Notification events and delivery history are stored in `history.db` and follow
the configured history-retention policy. Active deliveries are protected from
retention cleanup.

## Senders

Open **Notifications** in the administrator interface to create, edit, test,
disable, or delete a sender. Test delivery uses the same persistent queue,
worker, timeout, and retry path as an event delivery and appears in **Delivery
history**.

### Telegram

Provide the Bot API token and target chat ID. IPChronicle calls the official
Telegram `sendMessage` API with the localized event title and body. The bot
must already have permission to send to the target chat.

The token is encrypted in `config.db`. It is never returned by the API or web
interface. Leaving the token blank while editing preserves the configured
value.

### Webhook

Provide an HTTP or HTTPS URL and, when needed, up to 32 request headers in
`Name: value` form. IPChronicle sends an HTTP `POST` containing the versioned
event envelope as JSON. It sets these base headers:

```text
Content-Type: application/json
User-Agent: IPChronicle-Notification/1
```

Configured headers may replace those values, but cannot set `Host`,
`Content-Length`, `Connection`, or `Transfer-Encoding`. URLs containing user
information or fragments are rejected. Redirects are returned as failures and
are not followed.

Header values are encrypted in `config.db`. Only configured header names are
returned. Editing a Webhook preserves all header values unless **Replace
configured headers** is enabled.

### JavaScript

A JavaScript sender runs the configured source in a new isolated goja worker
process for every delivery. The script receives one global object:

```javascript
ipchronicle.apiVersion; // 1
ipchronicle.event; // parsed versioned event envelope
ipchronicle.title; // localized plain-text title
ipchronicle.body; // localized plain-text body
ipchronicle.http.request({
  method: "POST", // optional; defaults to GET
  url: "https://example.invalid/notify",
  headers: { "Content-Type": "application/json" }, // optional
  body: JSON.stringify(ipchronicle.event), // optional
});
```

`ipchronicle.http.request()` is synchronous and returns:

```javascript
{
  status: 204,
  headers: { "Header-Name": ["value"] },
  body: "",
}
```

An uncaught exception fails the attempt. A non-2xx response does not
automatically fail the script; inspect `status` and throw when the destination
requires a particular result.

The worker has no Node.js or Deno runtime, module loading, `require`, `fetch`,
DOM, filesystem, process, environment-variable, timer, or raw-socket API. Its
only network primitive is the synchronous HTTP/HTTPS call above. It does not
use the Center process's configured HTTP proxy and does not follow redirects.

Each invocation is bounded by:

- 30 seconds total wall-clock time;
- 10 HTTP requests, each with at most 10 seconds remaining/request time;
- 1 MiB for each request body and response body;
- 32 headers, with at most 8 KiB per header name/value pair;
- 256 KiB of source, 1 MiB of event input, and 16 KiB of worker output; and
- a 128 MiB worker data-segment limit on Linux.

These boundaries limit accidental resource exhaustion; they are not a policy
sandbox for mutually untrusted administrators. The sole server operator owns
the script and may intentionally send event data or script-embedded secrets to
any reachable HTTP or HTTPS endpoint.

## Queue, Retry, And Failure Behavior

IPChronicle runs four workers shared by Telegram and Webhook deliveries and one
global JavaScript worker. Each sender may have at most 1,024 active deliveries.
An additional matched event is retained as a terminal `queue-full` failure
instead of creating unbounded work.

A delivery is attempted at most four times. Retryable failures wait 10 seconds
after the first attempt, one minute after the second, and five minutes after
the third. Timeouts, connection failures, response-read failures, HTTP `429`,
HTTP `5xx`, Center cancellation, and JavaScript worker timeout are retryable.
Other HTTP `4xx`, invalid configuration or requests, oversized responses, and
script errors are terminal.

The delivery page shows `pending`, `running`, `retrying`, `succeeded`, and
`failed` states, attempt count, and a bounded error code. Destination response
bodies, exception messages, Telegram tokens, and Webhook header values are not
stored as delivery errors or returned to the browser.

Disabling a sender prevents queued work from being sent. A sender cannot be
deleted while a rule references it or while it has active deliveries. Existing
terminal delivery history keeps the sender name and type after deletion.

## Event Types

Rules can select these events:

- complete-probe known-field change;
- public-address change, check failure, and recovery;
- complete-probe failure and recovery;
- address-history and probe-history gaps; and
- upstream-format mismatch, changed mismatch, and recovery.

A known-field filter applies only to a complete-probe field-change rule. A node
filter may be combined with one of that node's durable network egresses.
