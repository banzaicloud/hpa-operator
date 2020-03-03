package stub

import (
	stderrors "errors"
	"fmt"
	"k8s.io/api/autoscaling/v2beta2"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func extractAnnotationIntValue(annotations map[string]string, annotationName string, deploymentName string) (int32, error) {
	strValue := annotations[annotationName]
	if len(strValue) == 0 {
		return 0, stderrors.New(annotationName + " annotation is missing for deployment " + deploymentName)
	}
	int64Value, err := strconv.ParseInt(strValue, 10, 32)
	if err != nil {
		return 0, stderrors.New(annotationName + " value for deployment " + deploymentName + " is invalid: " + err.Error())
	}
	value := int32(int64Value)
	if value <= 0 {
		return 0, stderrors.New(annotationName + " value for deployment " + deploymentName + " should be positive number")
	}
	return value, nil
}

func createResourceMetric(resourceName v1.ResourceName, annotationName string, valueFormat string, annotationValue string, deploymentName string) *v2beta2.MetricSpec {
	if len(annotationValue) == 0 {
		logrus.Errorf("Invalid resource metric annotation: %v value for deployment %v is missing", annotationName, deploymentName)
		return nil
	}
	if len(valueFormat) == 0 {
		logrus.Errorf("Invalid resource metric annotation: %v value format for deployment %v is missing", annotationName, deploymentName)
		return nil
	}

	switch valueFormat {
	case targetAverageUtilization:
		int64Value, err := strconv.ParseInt(annotationValue, 10, 32)
		if err != nil {
			logrus.Errorf("Invalid resource metric annotation: %v value for deployment %v is invalid: %v", annotationName, deploymentName, err.Error())
			return nil
		}
		targetValue := int32(int64Value)
		if targetValue <= 0 || targetValue > 100 {
			logrus.Errorf("Invalid resource metric annotation: %v value for deployment %v should be a percentage value between [1,99]", annotationName, deploymentName)
			return nil
		}

		if targetValue > 0 {
			return &v2beta2.MetricSpec{
				Type: v2beta2.ResourceMetricSourceType,
				Resource: &v2beta2.ResourceMetricSource{
					Name: resourceName,
					Target: v2beta2.MetricTarget{
						Type:               v2beta2.UtilizationMetricType,
						AverageUtilization: &targetValue,
					},
				},
			}
		}

	case targetAverageValue:
		targetValue, err := resource.ParseQuantity(annotationValue)
		if err != nil {
			logrus.Errorf("Invalid resource metric annotation: %v value for deployment %v is invalid: %v", annotationName, deploymentName, err.Error())
			return nil
		} else {
			return &v2beta2.MetricSpec{
				Type: v2beta2.ResourceMetricSourceType,
				Resource: &v2beta2.ResourceMetricSource{
					Name: resourceName,
					Target: v2beta2.MetricTarget{
						Type:         v2beta2.AverageValueMetricType,
						AverageValue: &targetValue,
					},
				},
			}
		}
	default:
		logrus.Warnf("Invalid resource metric valueFormat: %v for deployment %v", valueFormat, deploymentName)
	}

	return nil
}

func createExternalPrometheusMetrics(hpa *v2beta2.HorizontalPodAutoscaler, metricName string, annotations map[string]string, deploymentName string) *v2beta2.MetricSpec {

	logrus.Infof("setup custom prometheus metric: %v", metricName)

	queryKey := fmt.Sprintf("prometheus.%v.%v/query", metricName, hpaAnnotationPrefix)
	query, ok := annotations[queryKey]
	if !ok {
		logrus.Errorf("query is missing for custom metric: %s, deployment %v", metricName, deploymentName)
		return nil
	}
	if len(hpa.Annotations) == 0 {
		hpa.Annotations = make(map[string]string)
	}
	hpa.Annotations[fmt.Sprintf("metric-config.external.prometheus-query.prometheus/%s", metricName)] = query

	metricSpec := &v2beta2.MetricSpec{
		Type: v2beta2.ExternalMetricSourceType,
		External: &v2beta2.ExternalMetricSource{
			Metric: v2beta2.MetricIdentifier{
				Name: "prometheus-query",
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"query-name": metricName,
					},
				},
			},
		},
	}

	targetValueKey := fmt.Sprintf("prometheus.%v.%v/targetValue", metricName, hpaAnnotationPrefix)
	targetAverageValueKey := fmt.Sprintf("prometheus.%v.%v/targetAverageValue", metricName, hpaAnnotationPrefix)

	if valueStr, ok := annotations[targetValueKey]; ok {
		targetValue, err := resource.ParseQuantity(valueStr)
		if err != nil {
			logrus.Errorf("targetValue is invalid in custom metric: %s, deployment: %s (%s)", metricName, deploymentName, err.Error())
			return nil
		}
		metricSpec.External.Target = v2beta2.MetricTarget{
			Type:  v2beta2.ValueMetricType,
			Value: &targetValue,
		}
	} else if valueStr, ok = annotations[targetAverageValueKey]; ok {
		targetValue, err := resource.ParseQuantity(valueStr)
		if err != nil {
			logrus.Errorf("targetAverageValue is invalid in custom metric: %s, deployment: %s (%s)", metricName, deploymentName, err.Error())
			return nil
		}
		metricSpec.External.Target = v2beta2.MetricTarget{
			Type:         v2beta2.AverageValueMetricType,
			AverageValue: &targetValue,
		}
	} else {
		logrus.Errorf("either targetValue or targetAverageValue is required for custom metric: %s, deployment: %v", metricName, deploymentName)
		return nil
	}

	return metricSpec
}

func parseMetrics(hpa *v2beta2.HorizontalPodAutoscaler, annotations map[string]string, deploymentName string) []v2beta2.MetricSpec {

	metrics := make([]v2beta2.MetricSpec, 0, 4)
	customMetricsMap := make(map[string]*v2beta2.MetricSpec)

	for metricKey, metricValue := range annotations {
		keys := strings.Split(metricKey, annotationDomainSeparator)
		if len(keys) != 2 {
			logrus.Errorf("Metric annotation for deployment %v is invalid: %v", deploymentName, metricKey)
			return metrics
		}
		metricSubDomains := strings.Split(keys[0], annotationSubDomainSeparator)
		if len(metricSubDomains) < 2 {
			logrus.Errorf("Metric annotation for deployment %v is invalid: %v", deploymentName, metricKey)
			return metrics
		}
		var metric *v2beta2.MetricSpec
		switch metricSubDomains[0] {
		case cpuAnnotationPrefix:
			metric = createResourceMetric(v1.ResourceCPU, metricKey, keys[1], metricValue, deploymentName)
		case memoryAnnotationPrefix:
			metric = createResourceMetric(v1.ResourceMemory, metricKey, keys[1], metricValue, deploymentName)
		case prometheusAnnotationPrefix:
			metricName := metricSubDomains[1]
			if _, ok := customMetricsMap[metricName]; !ok {
				metric = createExternalPrometheusMetrics(hpa, metricName, annotations, deploymentName)
				customMetricsMap[metricName] = metric
			}
		}
		if metric != nil {
			metrics = append(metrics, *metric)
		}

	}

	return metrics
}
