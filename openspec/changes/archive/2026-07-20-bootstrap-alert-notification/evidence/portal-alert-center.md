# Portal Alert Center Design Evidence

## Vue Component Architecture

```
AlertCenter (page component)
  ├── AlertFilterBar (severity, state, source, time range)
  ├── AlertList (virtualized list with infinite scroll)
  │   └── AlertListItem (summary, severity badge, state, time)
  │       └── AlertActions (acknowledge, assign, silence, unsilence)
  └── AlertDetail (slide-out panel)
      ├── AlertInfo (summary, timeline, labels, source)
      ├── RelatedResources (resource, Operation, logs, metrics, traces, Runbook)
      └── DeliveryHistory (channel, state, attempts, timestamps)
```

## API Endpoints (Platform API)

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/alerts` | List alerts with filters |
| GET | `/alerts/{id}` | Alert detail |
| POST | `/alerts/{id}:acknowledge` | Acknowledge alert |
| POST | `/alerts/{id}:assign` | Assign alert to user |
| POST | `/alerts/{id}:silence` | Silence alert |
| POST | `/alerts/{id}:unsilence` | Remove silence |
| GET | `/alert-events` | SSE/WebSocket endpoint |

## Related Resource Links

Each alert displays links to:
- **Resource**: RuntimeTarget dashboard
- **Operation**: Operation detail page (if operationId present)
- **Logs**: Log query pre-filtered by resource and time range
- **Metrics**: Metrics dashboard for the resource
- **Traces**: Trace query for the correlation ID
- **Runbook**: Link to the alert's runbook reference

## Test Plan
- Component: AlertList renders correctly with mock data
- Filter: severity, state, source filters work independently and combined
- Detail: slide-out panel shows all info sections
- Related links: all resource links navigate correctly
- E2E: full alert lifecycle from creation to resolution in Portal
- Permissions: read-only user sees alerts but no action buttons