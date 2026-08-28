package alert

import (
	"context"
	"log"
	"sync"
	"time"
)

type AlertManager struct {
	store    AlertStore
	notifier *Notifier
	mu       sync.RWMutex
	rules    []AlertRule
	active   map[string]*AlertEvent
}

func NewAlertManager(store AlertStore, notifier *Notifier) *AlertManager {
	am := &AlertManager{
		store:    store,
		notifier: notifier,
		active:   make(map[string]*AlertEvent),
	}
	am.loadRules()
	return am
}

func (am *AlertManager) loadRules() {
	rules, err := am.store.ListRules()
	if err != nil {
		log.Printf("[alert] load rules: %v", err)
		return
	}
	am.mu.Lock()
	am.rules = rules
	am.mu.Unlock()
}

func (am *AlertManager) seedBuiltInRules() {
	existing, _ := am.store.ListRules()
	existingMap := make(map[string]bool)
	for _, r := range existing {
		existingMap[r.ID] = true
	}
	for _, rule := range BuiltInRules {
		if !existingMap[rule.ID] {
			if err := am.store.CreateRule(&rule); err != nil {
				log.Printf("[alert] seed rule %s: %v", rule.ID, err)
			} else {
				log.Printf("[alert] seeded built-in rule: %s", rule.Name)
			}
		}
	}
	am.loadRules()
}

func (am *AlertManager) Evaluate(ctx context.Context) {
	am.mu.RLock()
	rules := make([]AlertRule, len(am.rules))
	copy(rules, am.rules)
	am.mu.RUnlock()

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		am.evaluateRule(ctx, rule)
	}
	if storage, ok := am.store.(StorageRuleStore); ok {
		if err := storage.EvaluateStorageRules(ctx, time.Now().UTC()); err != nil {
			log.Printf("[alert] evaluate storage rules: %v", err)
		}
	}
}

func (am *AlertManager) evaluateRule(ctx context.Context, rule AlertRule) {
	// In a real implementation, this would evaluate PromQL or custom expressions
	// For now, this is a framework that external evaluators can call into
	_ = ctx
}

func (am *AlertManager) FireEvent(event *AlertEvent) {
	event.Status = StatusFiring
	event.StartedAt = time.Now().UTC()

	if err := am.store.CreateEvent(event); err != nil {
		log.Printf("[alert] create event: %v", err)
		return
	}

	am.mu.Lock()
	am.active[event.RuleID] = event
	am.mu.Unlock()

	notifications := am.notifier.Send(event)
	for _, n := range notifications {
		if err := am.store.CreateNotification(n); err != nil {
			log.Printf("[alert] save notification: %v", err)
		}
	}

	log.Printf("[alert] fired: %s (severity=%s)", event.RuleName, event.Severity)
}

func (am *AlertManager) ResolveEvent(ruleID string) {
	am.mu.RLock()
	event, exists := am.active[ruleID]
	am.mu.RUnlock()

	if !exists {
		return
	}

	now := time.Now().UTC()
	event.Status = StatusResolved
	event.ResolvedAt = &now

	if err := am.store.UpdateEvent(event); err != nil {
		log.Printf("[alert] resolve event: %v", err)
	}

	am.mu.Lock()
	delete(am.active, ruleID)
	am.mu.Unlock()

	notifications := am.notifier.Send(event)
	for _, n := range notifications {
		am.store.CreateNotification(n)
	}

	log.Printf("[alert] resolved: %s", event.RuleName)
}

func (am *AlertManager) Acknowledge(eventID, userID string) error {
	event, err := am.store.GetEvent(eventID)
	if err != nil {
		return err
	}
	event.Status = StatusAcknowledged
	event.AcknowledgedBy = userID
	return am.store.UpdateEvent(event)
}

func (am *AlertManager) ListEvents(severity Severity, status Status, limit int) ([]AlertEvent, error) {
	return am.store.ListEvents(severity, status, limit)
}

func (am *AlertManager) ListRules() ([]AlertRule, error) {
	return am.store.ListRules()
}

func (am *AlertManager) CreateRule(rule *AlertRule) error {
	if err := am.store.CreateRule(rule); err != nil {
		return err
	}
	am.loadRules()
	return nil
}

func (am *AlertManager) DeleteRule(id string) error {
	if err := am.store.DeleteRule(id); err != nil {
		return err
	}
	am.loadRules()
	return nil
}

type AlertReconciler struct {
	manager  *AlertManager
	interval time.Duration
}

func NewAlertReconciler(manager *AlertManager, interval time.Duration) *AlertReconciler {
	return &AlertReconciler{
		manager:  manager,
		interval: interval,
	}
}

func (r *AlertReconciler) Start(ctx context.Context) {
	log.Printf("[alert-reconciler] starting (interval=%s)", r.interval)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[alert-reconciler] stopped")
			return
		case <-ticker.C:
			r.manager.Evaluate(ctx)
		}
	}
}
