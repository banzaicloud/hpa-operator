package stub

import (
	"github.com/google/uuid"
	"k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/autoscaling/v2beta2"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
)

func TestCreateHPAWithResourceMetrics(t *testing.T) {

	annotations := map[string]string{
		"hpa.autoscaling.banzaicloud.io/minReplicas":                  "1",
		"hpa.autoscaling.banzaicloud.io/maxReplicas":                  "3",
		"cpu.hpa.autoscaling.banzaicloud.io/targetAverageUtilization": "80%",
		"memory.hpa.autoscaling.banzaicloud.io/targetAverageValue":    "1024Mi",
	}

	uuid, err := uuid.NewUUID()
	if err != nil {
		t.Error("Error can not generate UUID!")
		return
	}
	hpa := createHorizontalPodAutoscaler(types.UID(uuid.String()), "test", "default",
		"Deployment", "apps/v1", annotations)

	if hpa == nil {
		t.Error("Error hpa is not created!")
		return
	}

	if len(hpa.Spec.Metrics) == 0 {
		t.Error("Error no metrics found!")
		return
	}

	for _, metric := range hpa.Spec.Metrics {
		if metric.Type != v2beta2.ResourceMetricSourceType {
			t.Errorf("Metric type expected: %v actual: %v", v2beta1.ResourceMetricSourceType, metric.Type)
		}
		if metric.Resource.Name != v1.ResourceMemory {
			t.Errorf("Metric name expected: %v actual: %v", v1.ResourceMemory, metric.Resource.Name)
		}
	}

}

func TestCreateHPAWithExternalPrometheusMetrics(t *testing.T) {

	annotations := map[string]string{
		"hpa.autoscaling.banzaicloud.io/minReplicas":                                "1",
		"hpa.autoscaling.banzaicloud.io/maxReplicas":                                "3",
		"prometheus.customMetric.hpa.autoscaling.banzaicloud.io/query":              "{prometheusQuery}",
		"prometheus.customMetric.hpa.autoscaling.banzaicloud.io/targetAverageValue": "1024Mi",
	}

	uuid, err := uuid.NewUUID()
	if err != nil {
		t.Error("Error can not generate UUID!")
		return
	}
	hpa := createHorizontalPodAutoscaler(types.UID(uuid.String()), "test", "default",
		"Deployment", "apps/v1", annotations)

	if hpa == nil {
		t.Error("Error hpa is not created!")
		return
	}

	if len(hpa.Spec.Metrics) == 0 {
		t.Error("Error no metrics found!")
		return
	}

	if _, ok := hpa.Annotations["metric-config.external.prometheus-query.prometheus/customMetric"]; !ok {
		t.Errorf("Hpa query annotation is missing for custom metric!")
	}

	for _, metric := range hpa.Spec.Metrics {
		if metric.Type != v2beta2.ExternalMetricSourceType {
			t.Errorf("Metric type expected: %v actual: %v", v2beta1.PodsMetricSourceType, metric.Type)
		}
		if metric.External == nil {
			t.Errorf("External metric missing")
		}
		if metric.External.Metric.Name != "prometheus-query" {
			t.Errorf("Metric name expected: %v actual: %v", "prometheus-query", metric.External.Metric.Name)
		}
	}

}
