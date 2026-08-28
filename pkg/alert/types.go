package alert

import "time"

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

type Status string

const (
	StatusFiring   Status = "firing"
	StatusResolved Status = "resolved"
	StatusAcknowledged Status = "acknowledged"
)

type ChannelType string

const (
	ChannelWebhook ChannelType = "webhook"
	ChannelEmail   ChannelType = "email"
	ChannelSlack   ChannelType = "slack"
	ChannelDingTalk ChannelType = "dingtalk"
	ChannelWeChat  ChannelType = "wechat"
)

type AlertRule struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Severity    Severity          `json:"severity"`
	Enabled     bool              `json:"enabled"`
	Expr        string            `json:"expr"`
	Duration    string            `json:"duration"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Channels    []string          `json:"channels,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

type AlertEvent struct {
	ID          string            `json:"id"`
	RuleID      string            `json:"rule_id"`
	RuleName    string            `json:"rule_name"`
	Severity    Severity          `json:"severity"`
	Status      Status            `json:"status"`
	Message     string            `json:"message"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Value       float64           `json:"value,omitempty"`
	StartedAt   time.Time         `json:"started_at"`
	ResolvedAt  *time.Time        `json:"resolved_at,omitempty"`
	AcknowledgedBy string        `json:"acknowledged_by,omitempty"`
}

type NotificationChannel struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Type     ChannelType `json:"type"`
	Enabled  bool        `json:"enabled"`
	Config   ChannelConfig `json:"config"`
}

type ChannelConfig struct {
	WebhookURL  string `json:"webhook_url,omitempty"`
	EmailTo     string `json:"email_to,omitempty"`
	SlackToken  string `json:"slack_token,omitempty"`
	SlackChannel string `json:"slack_channel,omitempty"`
	DingTalkToken string `json:"dingtalk_token,omitempty"`
	WeChatKey   string `json:"wechat_key,omitempty"`
}

type Notification struct {
	ID        string         `json:"id"`
	EventID   string         `json:"event_id"`
	ChannelID string         `json:"channel_id"`
	ChannelType ChannelType  `json:"channel_type"`
	Status    string         `json:"status"`
	Error     string         `json:"error,omitempty"`
	SentAt    time.Time      `json:"sent_at"`
}

type AlertStore interface {
	CreateRule(rule *AlertRule) error
	UpdateRule(rule *AlertRule) error
	DeleteRule(id string) error
	GetRule(id string) (*AlertRule, error)
	ListRules() ([]AlertRule, error)

	CreateEvent(event *AlertEvent) error
	UpdateEvent(event *AlertEvent) error
	GetEvent(id string) (*AlertEvent, error)
	ListEvents(severity Severity, status Status, limit int) ([]AlertEvent, error)

	CreateNotification(n *Notification) error
	ListNotifications(eventID string) ([]Notification, error)
}

var BuiltInRules = []AlertRule{
	{
		ID: "cluster-offline", Name: "Cluster Offline",
		Description: "Cluster has been unreachable for more than 5 minutes",
		Severity: SeverityCritical, Enabled: true, Duration: "5m",
		Labels: map[string]string{"category": "cluster"},
		Annotations: map[string]string{"summary": "Cluster {{.labels.cluster_id}} is offline"},
	},
	{
		ID: "agent-disconnected", Name: "Agent Disconnected",
		Description: "Cluster agent has been disconnected for more than 2 minutes",
		Severity: SeverityWarning, Enabled: true, Duration: "2m",
		Labels: map[string]string{"category": "agent"},
		Annotations: map[string]string{"summary": "Agent for {{.labels.cluster_id}} disconnected"},
	},
	{
		ID: "extension-degraded", Name: "Extension Degraded",
		Description: "Extension has been in degraded state for more than 5 minutes",
		Severity: SeverityWarning, Enabled: true, Duration: "5m",
		Labels: map[string]string{"category": "extension"},
		Annotations: map[string]string{"summary": "Extension {{.labels.extension_name}} is degraded"},
	},
	{
		ID: "provider-health", Name: "Provider Health Check Failed",
		Description: "Provider health check has failed 3 times consecutively",
		Severity: SeverityWarning, Enabled: true, Duration: "1m",
		Labels: map[string]string{"category": "provider"},
		Annotations: map[string]string{"summary": "Provider {{.labels.provider}} health check failed"},
	},
}