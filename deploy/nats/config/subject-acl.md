# NATS Subject ACL Configuration
# Service account isolation, credential rotation, and permission separation

# Service Accounts and Permissions

# 1. Outbox Relay (publishes all messages)
#    - Can publish to all hnb.* subjects
#    - Cannot subscribe to any subjects
#    - Credentials: nats-outbox-relay-creds (rotated every 90 days)

# 2. Operation Worker (combined worker and outbox relay process)
#    - Can subscribe to hnb.command.> and hnb.failed.> subjects
#    - Can publish committed outbox, completion, and failure subjects under hnb.>
#    - Can use JetStream API and response inbox subjects
#    - Credentials: nats-operation-worker-creds (rotated every 90 days)

# 3. Projector (consumes domain events)
#    - Can subscribe to hnb.event.> subjects
#    - Cannot publish to any subjects
#    - Credentials: nats-projector-creds (rotated every 90 days)

# 4. Audit (consumes domain events)
#    - Can subscribe to hnb.event.> subjects
#    - Cannot publish to any subjects
#    - Credentials: nats-audit-creds (rotated every 90 days)

# 5. Notification Dispatcher (consumes notifications)
#    - Can subscribe to hnb.notification.> subjects
#    - Cannot publish to any subjects
#    - Credentials: nats-notification-dispatcher-creds (rotated every 90 days)

# 6. Admin (full access)
#    - Can publish and subscribe to all subjects
#    - Used for management, stream/consumer configuration
#    - Credentials: nats-admin-creds (rotated every 30 days)

# 7. Remaining production clients (mTLS certificate CN equals user name)
#    - alert-manager: subscribe hnb.alert.fire and hnb.alert.resolve only
#    - extension-controller: subscribe hnb.extension.{install,upgrade,uninstall,health};
#      request hnb.extension.provider.> and use response inboxes
#    - calico-provider, cilium-provider, kube-ovn-provider: consume and publish
#      results only below their own hnb.network.<provider>.> hierarchy
#    - network-provider and network-registry: consume/route hnb.network.> as
#      required by their dynamic provider code
#    - Network workers may use JetStream API, ACK, and response inbox subjects
#    - Secrets: one nats-<service>-client-tls Secret per Helm release; sharing a
#      client Secret between services is prohibited

# NATS Configuration (nats.conf authorization section)

authorization {
  timeout: 5
  users = [
    {
      user: "outbox-relay"
      permissions: {
        publish: {
          allow: ["hnb.>"]
          deny: ["$SYS.>", "$G.>"]
        }
        subscribe: ""
      }
    }
    {
      user: "operation-worker"
      permissions: {
        publish: ["hnb.>", "$JS.API.>"]
        subscribe: {
          allow: ["hnb.command.>", "hnb.failed.>", "_INBOX.>"]
          deny: ["$SYS.>", "$G.>"]
        }
      }
    }
    {
      user: "app-market"
      permissions: { publish: ["hnb.market.>", "$JS.API.>"], subscribe: ["hnb.market.>", "_INBOX.>"] }
    }
    {
      user: "gateway-provider"
      permissions: { publish: ["hnb.gateway.>", "$JS.API.>"], subscribe: ["hnb.gateway.>", "_INBOX.>"] }
    }
    {
      user: "apiserver"
      permissions: { publish: ["$JS.API.>", "$KV.HNB_LEADER.>"], subscribe: ["_INBOX.>", "$KV.HNB_LEADER.>"] }
    }
    {
      user: "alert-manager"
      permissions: { publish: "", subscribe: ["hnb.alert.fire", "hnb.alert.resolve"] }
    }
    {
      user: "extension-controller"
      permissions: {
        publish: ["hnb.extension.provider.>", "_INBOX.>"]
        subscribe: ["hnb.extension.install", "hnb.extension.upgrade", "hnb.extension.uninstall", "hnb.extension.health", "_INBOX.>"]
      }
    }
    {
      user: "calico-provider"
      permissions: { publish: ["hnb.network.calico.*.result", "$JS.API.>", "$JS.ACK.>"], subscribe: ["hnb.network.calico.>", "_INBOX.>"] }
    }
    {
      user: "cilium-provider"
      permissions: { publish: ["hnb.network.cilium.*.result", "$JS.API.>", "$JS.ACK.>"], subscribe: ["hnb.network.cilium.>", "_INBOX.>"] }
    }
    {
      user: "kube-ovn-provider"
      permissions: { publish: ["hnb.network.kube-ovn.*.result", "$JS.API.>", "$JS.ACK.>"], subscribe: ["hnb.network.kube-ovn.>", "_INBOX.>"] }
    }
    {
      user: "network-provider"
      permissions: { publish: ["hnb.network.*.*.result", "$JS.API.>", "$JS.ACK.>"], subscribe: ["hnb.network.>", "_INBOX.>"] }
    }
    {
      user: "network-registry"
      permissions: { publish: ["hnb.network.*.*", "$JS.API.>", "$JS.ACK.>"], subscribe: ["hnb.network.>", "_INBOX.>"] }
    }
    {
      user: "projector"
      permissions: {
        publish: ""
        subscribe: {
          allow: ["hnb.event.>"]
          deny: ["$SYS.>", "$G.>"]
        }
      }
    }
    {
      user: "audit"
      permissions: {
        publish: ""
        subscribe: {
          allow: ["hnb.event.>"]
          deny: ["$SYS.>", "$G.>"]
        }
      }
    }
    {
      user: "notification-dispatcher"
      permissions: {
        publish: ""
        subscribe: {
          allow: ["hnb.notification.>"]
          deny: ["$SYS.>", "$G.>"]
        }
      }
    }
    {
      user: "admin"
      permissions: {
        publish: ">"
        subscribe: ">"
      }
    }
  ]
}

# Credential Rotation Procedure
# 1. Generate new NKey pair: nk -gen user > new.creds
# 2. Add new user to nats.conf with updated permissions
# 3. Reload NATS config: nats-server --signal reload
# 4. Verify new credentials work: nats --creds new.creds pub test.rotate "ok"
# 5. Update service secrets with new credentials
# 6. Remove old credentials after all services have rotated

# Permission Separation
# - Admin: full access, used for Stream/Consumer CRUD
# - Publish: only Outbox Relay
# - Subscribe: per-service scoped to relevant subject hierarchy
# - Default deny: no user has implicit access to any subject
