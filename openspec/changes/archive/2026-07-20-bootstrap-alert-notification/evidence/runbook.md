# Alert/Notification Runbook Evidence

## Runbook Sections

### 1. Alert Rule Management
- Creating alert rules (source type, severity, expression)
- Enabling/disabling rules
- Viewing rule evaluation history

### 2. Notification Routing
- Creating notification policies
- Configuring matchers (label-based routing)
- Setting up escalation steps
- Testing policy matching

### 3. Silence and Maintenance
- Creating time-bound silences
- Scheduling maintenance windows
- Viewing active silences
- Removing silences

### 4. On-Call Schedule
- Creating schedules with shifts
- Assigning schedules to contact groups
- Managing exceptions and overrides
- Viewing who is on-call

### 5. Notification Channels
- Configuring Email (SMTP, TLS, templates)
- Configuring Webhook (URL, HMAC, SSRF)
- Managing channel credentials (SecretReference)
- Testing channels

### 6. Template Management
- Creating versioned templates
- Language and timezone configuration
- Template validation (forbidden fields)
- Testing templates

### 7. Failure Recovery
- Manual redrive of failed deliveries
- Circuit breaker reset
- Backlog management
- Reconnecting to external services

### 8. Privacy and Compliance
- Contact data encryption and masking
- Data retention configuration
- Audit log review
- PII verification

### 9. Troubleshooting
- Common error codes and solutions
- Debugging delivery failures
- Diagnosing rate limiting
- Investigating duplicate notifications

### 10. Rollback
- Disabling channels
- Portal read-only mode
- Reverting to old monitoring pipeline
- Data preservation during rollback