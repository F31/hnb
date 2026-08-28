package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type Notifier struct {
	channels []*NotificationChannel
	client   *http.Client
}

func NewNotifier(channels []*NotificationChannel) *Notifier {
	return &Notifier{
		channels: channels,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *Notifier) Send(event *AlertEvent) []*Notification {
	var notifications []*Notification

	for _, ch := range n.channels {
		if !ch.Enabled {
			continue
		}
		notif := n.sendToChannel(event, ch)
		notifications = append(notifications, notif)
	}

	return notifications
}

func (n *Notifier) sendToChannel(event *AlertEvent, ch *NotificationChannel) *Notification {
	notif := &Notification{
		ID:          fmt.Sprintf("%s-%s", event.ID, ch.ID),
		EventID:     event.ID,
		ChannelID:   ch.ID,
		ChannelType: ch.Type,
		SentAt:      time.Now().UTC(),
	}

	var err error
	switch ch.Type {
	case ChannelWebhook:
		err = n.sendWebhook(event, ch)
	case ChannelSlack:
		err = n.sendSlack(event, ch)
	case ChannelDingTalk:
		err = n.sendDingTalk(event, ch)
	default:
		err = fmt.Errorf("unsupported channel type: %s", ch.Type)
	}

	if err != nil {
		notif.Status = "failed"
		notif.Error = err.Error()
		log.Printf("[alert] send to %s failed: %v", ch.Type, err)
	} else {
		notif.Status = "sent"
		log.Printf("[alert] sent to %s for event %s", ch.Type, event.ID)
	}

	return notif
}

func (n *Notifier) sendWebhook(event *AlertEvent, ch *NotificationChannel) error {
	payload := map[string]any{
		"event_id":    event.ID,
		"rule_name":   event.RuleName,
		"severity":    event.Severity,
		"status":      event.Status,
		"message":     event.Message,
		"labels":      event.Labels,
		"annotations": event.Annotations,
		"started_at":  event.StartedAt,
		"resolved_at": event.ResolvedAt,
	}

	data, _ := json.Marshal(payload)
	resp, err := n.client.Post(ch.Config.WebhookURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}

func (n *Notifier) sendSlack(event *AlertEvent, ch *NotificationChannel) error {
	color := "#ff0000"
	if event.Severity == SeverityWarning {
		color = "#ffa500"
	} else if event.Severity == SeverityInfo {
		color = "#1e90ff"
	}

	payload := map[string]any{
		"channel": ch.Config.SlackChannel,
		"attachments": []map[string]any{
			{
				"color": color,
				"title": fmt.Sprintf("[%s] %s", event.Severity, event.RuleName),
				"text":  event.Message,
				"fields": []map[string]any{
					{"title": "Status", "value": event.Status, "short": true},
					{"title": "Cluster", "value": event.Labels["cluster_id"], "short": true},
				},
				"ts": event.StartedAt.Unix(),
			},
		},
	}

	data, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+ch.Config.SlackToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		OK bool `json:"ok"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if !result.OK {
		return fmt.Errorf("slack API returned not ok")
	}
	return nil
}

func (n *Notifier) sendDingTalk(event *AlertEvent, ch *NotificationChannel) error {
	payload := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": fmt.Sprintf("[%s] %s", event.Severity, event.RuleName),
			"text":  fmt.Sprintf("### %s\n\n%s\n\n- Severity: %s\n- Status: %s\n- Cluster: %s\n- Time: %s",
				event.RuleName, event.Message, event.Severity, event.Status,
				event.Labels["cluster_id"], event.StartedAt.Format(time.RFC3339)),
		},
	}

	data, _ := json.Marshal(payload)
	url := fmt.Sprintf("https://oapi.dingtalk.com/robot/send?access_token=%s", ch.Config.DingTalkToken)
	resp, err := n.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}