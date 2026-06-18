# Leases

Reveille uses leases to decide how long a started target should remain running.

A lease belongs to one configured host. When a user starts a timer from the wait
UI, Reveille records an active lease for that host. Finite leases stop the target
automatically when they expire. `never` leases keep the target running until a
manual stop or replacement lease changes that behavior.

Active leases live in Reveille memory. They are not written to disk.

## Overview

The lease flow is:

1. A stopped or unready target sends the browser to the wait UI.
2. The user chooses a run window.
3. The browser posts the selected lease to Reveille.
4. Reveille records or replaces the active lease for that host.
5. The wait UI uses the active lease for countdown and redirect behavior.
6. A finite lease expires later and Reveille asks Dockhand to stop the target.

The backend lease is the source of truth. Browser session storage only helps the
wait UI decide whether this browser session has started a timer.

## Lease Types

### Finite Leases

Finite leases use Go duration strings such as `30m`, `1h`, `2h`, or `4h30m`.

When Reveille records a finite lease:

- it calculates `expiresAt` from the current time plus the selected duration
- it starts an in-memory timer for that host
- it returns `expiresAt` to the wait UI
- the wait UI counts down to that backend timestamp

When the timer expires, Reveille asks Dockhand to stop the target and removes
the active lease from memory.

### Never Leases

`never` disables automatic stop for the active lease.

When Reveille records a `never` lease:

- it marks the lease as `never`
- it does not start an expiry timer
- the wait UI hides the countdown panel
- the target keeps running until manual stop or replacement lease

`never` is parsed case-insensitively and is displayed as `Never`.

## Creating A Lease

The wait UI creates leases through the browser control channel:

```http
POST /_reveille/wait?host=app.example.com
Content-Type: multipart/form-data

action=lease
lease=2h
token=<wait-token>
```

The wait token comes from Reveille's `forwardAuth` redirect and is bound to the
managed host.

The selected `lease` value must match one of the host's configured lease option
labels. Matching is case-insensitive. If the request omits `lease`, Reveille
uses the host's default lease.

The compatibility lease API uses the same handler:

```http
POST /_reveille/api/lease?host=app.example.com&token=<wait-token>
```

It accepts form data and JSON:

```json
{"lease":"2h"}
```

A finite lease response includes `expiresAt`:

```json
{
  "host": "app.example.com",
  "never": false,
  "expiresAt": "2026-06-17T18:30:00Z"
}
```

A `never` lease response includes `never: true` and no expiration:

```json
{
  "host": "app.example.com",
  "never": true
}
```

## Replacement Leases

Setting a lease for a host replaces any active lease for that same host.

Replacement behavior:

- an old finite expiry timer is cancelled
- the new lease becomes the active lease
- a new finite lease gets a fresh `expiresAt`
- replacing `never` with a finite lease re-enables automatic stop
- replacing a finite lease with `never` disables automatic stop

This is useful when a user chooses a short timer, then decides the app should
stay available longer.

## Expiry

Finite lease expiry is handled inside Reveille:

1. The in-memory timer fires.
2. Reveille logs that the lease expired.
3. Reveille calls the configured stop behavior for the host.
4. On success or failure, Reveille removes the active lease and timer from
   memory.

The automatic stop call uses the lease manager's internal stop timeout. Manual
stops use `defaults.stopGrace`.

If the stop call fails, Reveille logs the error and still removes the lease from
memory. The next browser visit can start the normal wait flow again.

## Manual Stop

The wait UI includes a `Stop App` form in countdown mode:

```http
POST /_reveille/wait?host=app.example.com
Content-Type: multipart/form-data

action=stop
token=<wait-token>
```

Manual stop:

- cancels any active finite expiry timer
- removes the active lease from memory
- asks Dockhand to stop the target
- uses `defaults.stopGrace` as the stop timeout

The compatibility route calls the same stop behavior:

```http
POST /_reveille/api/stop?host=app.example.com&token=<wait-token>
```

On success, Reveille returns:

```json
{"status":"stopped"}
```

## Lease Config

Global defaults live in `reveille.yml`:

```yaml
defaults:
  lease: "2h"
  leaseOptions:
    - "30m"
    - "1h"
    - "2h"
    - "4h"
    - "never"
```

Host files can override the default and available options for one host:

```yaml
lease:
  default: "1h"
  options:
    - "15m"
    - "1h"
    - "never"
```

Lease values are parsed with Go duration syntax. `never` is the special value
for a no-stop lease.

The wait page shows the host's lease options and preselects the host's default
lease.

## Runtime Notes

- Active leases are in memory only.
- Restarting Reveille clears lease state.
- Clearing browser storage does not clear backend leases.
- A running Docker target may outlive Reveille's in-memory lease state.
- A replacement lease only affects the matching configured host.
- The status endpoint reports lease state with `leaseActive`, `expiresAt`, and
  `never`.

## Troubleshooting

| Symptom | Likely Cause | Check |
| --- | --- | --- |
| Timer save returns `invalid lease` | Submitted value is not in `lease.options` | Host lease config and browser request payload |
| Timer saves but countdown is wrong | Browser is using stale assets or an old status response | `wait.js` asset and status JSON `expiresAt` |
| Target stops earlier than expected | A replacement finite lease shortened the active timer | Reveille lease accepted logs |
| Target never stops | Active lease is `never` or Dockhand stop failed | Status JSON `never`, Reveille logs, Dockhand logs |
| Stop button fails | Dockhand stop call failed or timed out | Reveille logs and `defaults.stopGrace` |
| New browser visit shows timer selection despite running target | Leases are in memory and browser session state is only a UI guard | Reveille restart history and status JSON |

## Related Docs

- Wait UI: [wait-ui.md](wait-ui.md)
- Runtime flow: [runtime-flow.md](runtime-flow.md)
- Runtime config: [reveille-yml.md](reveille-yml.md)
