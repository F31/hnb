package worker

import (
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDeploymentReady(t *testing.T) {
	replicas := int32(2)
	deploy := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Generation: 3},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{ObservedGeneration: 3, UpdatedReplicas: 2, AvailableReplicas: 2},
	}
	if err := deploymentReady(deploy); err != nil {
		t.Fatalf("deployment should be ready: %v", err)
	}
	deploy.Status.AvailableReplicas = 1
	if err := deploymentReady(deploy); err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("expected not ready error, got %v", err)
	}
}

func TestDaemonSetReady(t *testing.T) {
	ds := appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Generation: 2},
		Status: appsv1.DaemonSetStatus{
			ObservedGeneration:     2,
			DesiredNumberScheduled: 3,
			UpdatedNumberScheduled: 3,
			NumberAvailable:        3,
		},
	}
	if err := daemonSetReady(ds); err != nil {
		t.Fatalf("daemonset should be ready: %v", err)
	}
	ds.Status.NumberAvailable = 2
	if err := daemonSetReady(ds); err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("expected not ready error, got %v", err)
	}
}

func TestStatefulSetReady(t *testing.T) {
	replicas := int32(1)
	sts := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Generation: 1},
		Spec:       appsv1.StatefulSetSpec{Replicas: &replicas},
		Status:     appsv1.StatefulSetStatus{ObservedGeneration: 1, UpdatedReplicas: 1, ReadyReplicas: 1},
	}
	if err := statefulSetReady(sts); err != nil {
		t.Fatalf("statefulset should be ready: %v", err)
	}
	sts.Status.ReadyReplicas = 0
	if err := statefulSetReady(sts); err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("expected not ready error, got %v", err)
	}
}

func TestPodReady(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "demo"},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{
			{Type: corev1.PodReady, Status: corev1.ConditionTrue},
		}},
	}
	if err := podReady(pod); err != nil {
		t.Fatalf("pod should be ready: %v", err)
	}
	pod.Status.Conditions = nil
	if err := podReady(pod); err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("expected not ready error, got %v", err)
	}
	pod.Status.Phase = corev1.PodSucceeded
	if err := podReady(pod); err != nil {
		t.Fatalf("succeeded pod should be accepted: %v", err)
	}
}
