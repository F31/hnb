package storagemetrics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

type Kind string

const (
	Capacity   Kind = "capacity"
	Usage      Kind = "usage"
	IOPS       Kind = "iops"
	Throughput Kind = "throughput"
	Latency    Kind = "latency"
	Health     Kind = "health"
)

type Applicability string

const (
	Applicable  Applicability = "Applicable"
	Unsupported Applicability = "Unsupported"
	Unknown     Applicability = "Unknown"
)

type Status string

const (
	Known         Status = "Known"
	Elastic       Status = "Elastic"
	NotReported   Status = "NotReported"
	StatusUnknown Status = "Unknown"
)

type Freshness string

const (
	Fresh            Freshness = "Fresh"
	Stale            Freshness = "Stale"
	FreshnessUnknown Freshness = "Unknown"
)

var canonicalUnits = map[Kind]string{
	Capacity: "By", Usage: "By", IOPS: "1/s", Throughput: "By/s", Latency: "s", Health: "1",
}

var allKinds = []Kind{Capacity, Usage, IOPS, Throughput, Latency, Health}

// Descriptor is provider package metadata. Applicability must be declared for
// every canonical metric so consumers never infer support from a missing value.
type Descriptor struct {
	ProviderID   string
	Source       string
	Capabilities map[Kind]Applicability
	FreshFor     time.Duration
}

type Scope struct {
	TargetID, ResourceKind, ResourceUID string
}

type RawMeasurement struct {
	Kind   Kind
	Value  *float64
	Status Status
}

type Snapshot struct {
	ObservedAt time.Time
	Values     []RawMeasurement
}

// Adapter is the provider boundary. Source is a stable adapter/exporter name,
// not a tenant, PVC, PV, volume handle, or other resource instance name.
type Adapter interface {
	Descriptor() Descriptor
	Read(context.Context, Scope) (Snapshot, error)
}

type Measurement struct {
	Kind          Kind          `json:"kind"`
	Unit          string        `json:"unit"`
	Value         *float64      `json:"value,omitempty"`
	Status        Status        `json:"status"`
	Applicability Applicability `json:"applicability"`
	Source        string        `json:"source"`
	ObservedAt    time.Time     `json:"observedAt"`
	Freshness     Freshness     `json:"freshness"`
}

type NormalizedSnapshot struct {
	ProviderID   string        `json:"providerId"`
	TargetID     string        `json:"targetId"`
	ResourceKind string        `json:"resourceKind"`
	ResourceUID  string        `json:"resourceUid"`
	Metrics      []Measurement `json:"metrics"`
}

func Normalize(descriptor Descriptor, scope Scope, snapshot Snapshot, now time.Time) (NormalizedSnapshot, error) {
	if descriptor.ProviderID == "" || descriptor.Source == "" || snapshot.ObservedAt.IsZero() || descriptor.FreshFor <= 0 {
		return NormalizedSnapshot{}, errors.New("provider, source, observedAt, and positive freshness window are required")
	}
	if scope.TargetID == "" || scope.ResourceKind == "" || scope.ResourceUID == "" {
		return NormalizedSnapshot{}, errors.New("stable target and resource references are required")
	}
	values := make(map[Kind]RawMeasurement, len(snapshot.Values))
	for _, raw := range snapshot.Values {
		if _, ok := canonicalUnits[raw.Kind]; !ok {
			return NormalizedSnapshot{}, fmt.Errorf("unknown metric kind %q", raw.Kind)
		}
		if _, duplicate := values[raw.Kind]; duplicate {
			return NormalizedSnapshot{}, fmt.Errorf("duplicate metric kind %q", raw.Kind)
		}
		values[raw.Kind] = raw
	}
	freshness := Fresh
	if now.After(snapshot.ObservedAt.Add(descriptor.FreshFor)) {
		freshness = Stale
	}
	result := NormalizedSnapshot{ProviderID: descriptor.ProviderID, TargetID: scope.TargetID, ResourceKind: scope.ResourceKind, ResourceUID: scope.ResourceUID}
	for _, kind := range allKinds {
		applicability, ok := descriptor.Capabilities[kind]
		if !ok || (applicability != Applicable && applicability != Unsupported && applicability != Unknown) {
			return NormalizedSnapshot{}, fmt.Errorf("metric %q requires valid capability applicability", kind)
		}
		raw, reported := values[kind]
		measurement := Measurement{Kind: kind, Unit: canonicalUnits[kind], Status: NotReported, Applicability: applicability,
			Source: descriptor.Source, ObservedAt: snapshot.ObservedAt.UTC(), Freshness: freshness}
		if reported {
			measurement.Status, measurement.Value = raw.Status, raw.Value
		}
		if err := validateMeasurement(measurement); err != nil {
			return NormalizedSnapshot{}, fmt.Errorf("metric %q: %w", kind, err)
		}
		result.Metrics = append(result.Metrics, measurement)
	}
	return result, nil
}

func validateMeasurement(metric Measurement) error {
	if metric.Applicability != Applicable && metric.Value != nil {
		return errors.New("non-applicable metric cannot carry a value")
	}
	if metric.Status == Known {
		if metric.Applicability != Applicable || metric.Value == nil || math.IsNaN(*metric.Value) || math.IsInf(*metric.Value, 0) || *metric.Value < 0 {
			return errors.New("Known requires an applicable, finite, non-negative value")
		}
		return nil
	}
	if metric.Status != Elastic && metric.Status != NotReported && metric.Status != StatusUnknown {
		return fmt.Errorf("invalid status %q", metric.Status)
	}
	if metric.Value != nil {
		return errors.New("unavailable metric cannot carry a value")
	}
	return nil
}

func Kinds() []Kind {
	result := append([]Kind(nil), allKinds...)
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
