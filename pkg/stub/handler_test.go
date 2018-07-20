package stub

import (
	"k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/core/v1"
	"testing"
)

func TestCreateHorizontalPodAutoscaler(t *testing.T) {

	annotations := map[string]string{
		"hpaAnnotationPrefix/minReplicas":      "1",
		"hpaAnnotationPrefix/maxReplicas":      "3",
		"hpaAnnotationPrefix/cpu":              "80%",
		"hpaAnnotationPrefix/memory":           "1024Mi",
		"hpaAnnotationPrefix.pod/customMetric": "1024xMi",
	}
	hpa := createHorizontalPodAutoscaler("test", "default", "apps/v1", "Deployment", annotations)

	if hpa == nil {
		t.Error("Error hpa is not created!")
	}

	if len(hpa.Spec.Metrics) != 1 {
		t.Error("Error no metrics found!")
	}

	for _, metric := range hpa.Spec.Metrics {
		if metric.Type != v2beta1.ResourceMetricSourceType {
			t.Errorf("Metric type expected: %v actual: %v", v2beta1.ResourceMetricSourceType, metric.Type)
		}
		if metric.Resource.Name != v1.ResourceMemory {
			t.Errorf("Metric name expected: %v actual: %v", v1.ResourceMemory, metric.Resource.Name)
		}
	}

}
