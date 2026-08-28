# Notification Bell and SSE/WebSocket Design Evidence

## Architecture

```
Server: Platform API SSE endpoint (/alert-events)
    |
    ├── Subscribe to JetStream for delivery-changed events
    ├── Filter by tenant context
    ├── Maintain per-connection event cursor
    ├── Push events as SSE messages
    └── Track connected clients for unread count
```

## SSE Event Format

```
event: alert.firing
data: {"alertId":"uuid","severity":"critical","summary":"..."}

event: alert.resolved
data: {"alertId":"uuid","fingerprint":"..."}

event: notification.delivery-changed
data: {"deliveryId":"uuid","state":"accepted","channelType":"email"}

event: unread.count
data: {"count":5}
```

## Connection Recovery

1. Client connects to `/alert-events?since={lastEventId}`
2. Server replays events since the cursor
3. If cursor is too old (>5 min), server returns full state
4. Client re-syncs by fetching current alert list
5. Unread count is recalculated on reconnection

## Multi-tab Support

- Each tab maintains its own SSE connection
- Unread count is stored server-side, not in localStorage
- When one tab marks an alert as read, all tabs receive the update
- Server broadcasts `unread.count` events to all tenant connections

## Offline Handling

- Client detects connection loss (SSE `error` event)
- Exponential backoff reconnection: 1s, 2s, 4s, 8s, max 30s
- On reconnect, send `lastEventId` to resume
- If reconnection fails after 5 min, show "Connection lost" banner

## Test Plan
- SSE: events are received in real-time
- Reconnection: client reconnects after disconnect, events resume from cursor
- Multi-tab: action in tab A updates tab B
- Unread count: count updates correctly on new alert and on read
- Permission: tenant A cannot receive tenant B's events
- Old cursor: cursor >5 min old triggers full re-sync