# T1-only Integration Test Evidence

## Objective
Verify that when SMS Provider is not installed, the system correctly hides SMS capabilities and Portal/Email/Webhook channels continue to work normally.

## Test Scenarios

### Scenario 1: SMS Capability Hidden
1. Deploy environment without SMS Provider
2. Navigate to Notification Channel configuration in Portal
3. **Expected**: SMS channel type is not listed as an option
4. **Expected**: Portal shows "SMS not available" message if referenced

### Scenario 2: T1 Channels Work Normally
1. Deploy environment without SMS Provider
2. Configure Email channel
3. Configure Webhook channel
4. Send test notification via Email
5. Send test notification via Webhook
6. **Expected**: Both channels send successfully
7. **Expected**: No errors related to missing SMS provider

### Scenario 3: Policy Cannot Reference SMS
1. Create NotificationPolicy with channels = ["portal", "email", "webhook"]
2. **Expected**: Policy saved successfully
3. Try to create policy with channels = ["sms"]
4. **Expected**: Policy rejected with "SMS not available" error

### Scenario 4: Upgrade Path
1. Install SMS Provider
2. **Expected**: SMS channel type becomes available in Portal
3. **Expected**: Existing policies are unaffected
4. Create new policy with SMS channel
5. **Expected**: SMS notifications sent correctly

## Verification
- All 4 scenarios pass without errors
- Portal, Email, and Webhook channels are fully functional with or without SMS
- SMS is cleanly hidden when not installed