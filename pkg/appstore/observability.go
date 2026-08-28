package appstore

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ArtifactVerificationEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_artifact_verification_events_total",
		Help: "Artifact verification outcomes by result.",
	}, []string{"result"})
	ArtifactRobotCleanupEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_artifact_robot_cleanup_events_total",
		Help: "Temporary upload robot cleanup outcomes by result.",
	}, []string{"result"})
	ArtifactProfileHealthEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_artifact_profile_health_events_total",
		Help: "Storage profile health transitions by backend and health.",
	}, []string{"backend", "health"})
	ArtifactDistributionRebuildEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_artifact_distribution_rebuild_events_total",
		Help: "Distribution rebuild outcomes by role and result.",
	}, []string{"role", "result"})
	ArtifactGCEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_artifact_gc_events_total",
		Help: "Artifact GC outcomes by stage and result.",
	}, []string{"stage", "result"})
)

type ArtifactEvent struct {
	Event       string    `json:"event"`
	TenantID    string    `json:"tenant_id,omitempty"`
	ArtifactID  string    `json:"artifact_id,omitempty"`
	Digest      string    `json:"digest,omitempty"`
	OperationID string    `json:"operation_id,omitempty"`
	Result      string    `json:"result,omitempty"`
	OccurredAt  time.Time `json:"occurred_at"`
}

func LogArtifactEvent(event ArtifactEvent) {
	event.OccurredAt = time.Now().UTC()
	data, err := json.Marshal(redactArtifactEvent(event))
	if err != nil {
		log.Printf("artifact event marshal failed: %v", err)
		return
	}
	log.Printf("%s", data)
}

func redactArtifactEvent(event ArtifactEvent) ArtifactEvent {
	forbidden := []string{"token", "secret", "password", "credential"}
	for _, word := range forbidden {
		if strings.Contains(strings.ToLower(event.Event), word) {
			event.Event = "redacted"
		}
	}
	return event
}
