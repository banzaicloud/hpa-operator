package stub

import (
	"context"
	stderrors "errors"
	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"regexp"
	"strconv"
	"strings"
)

const hpaAnnotationPrefix = "hpa.autoscaling.banzaicloud.io"

const cpuAnnotationPrefix = "cpu"
const memoryAnnotationPrefix = "memory"
const podAnnotationPrefix = "pod"

const targetAverageUtilization = "targetAverageUtilization"
const targetAverageValue = "targetAverageValue"
const annotationDomainSeparator = "/"
const annotationSubDomainSeparator = "."

const annotationRegExpString = "(" + cpuAnnotationPrefix + "|" + memoryAnnotationPrefix + "|" + podAnnotationPrefix + ")?(\\.)?hpa\\.autoscaling\\.banzaicloud\\.io\\/[a-zA-Z]+"

func NewHandler() sdk.Handler {
	r, _ := regexp.Compile(annotationRegExpString)
	return &Handler{
		annotationRegExp: r,
	}
}

type Handler struct {
	annotationRegExp *regexp.Regexp
}

func (h *Handler) Handle(ctx context.Context, event sdk.Event) error {
	if event.Deleted {
		return nil
	}
	switch o := event.Object.(type) {
	case *appsv1.Deployment:
		return h.handleReplicaController(o, o.GroupVersionKind(), o.Spec.Template.Annotations)
	case *appsv1.StatefulSet:
		return h.handleReplicaController(o, o.GroupVersionKind(), o.Spec.Template.Annotations)
	}
	return nil
}

func (h *Handler) handleReplicaController(o metav1.Object, gvk schema.GroupVersionKind, podAnnotations map[string]string) error {
	logrus.Infof("handle  : %v", o.GetName())
	annotations := o.GetAnnotations()
	if !h.checkAutoscaleAnnotationIsPresent(annotations) {
		annotations = podAnnotations
		if !h.checkAutoscaleAnnotationIsPresent(annotations) {
			logrus.Infof("Autoscale annotations not found")
			return nil
		} else {
			logrus.Infof("Autoscale annotations found on Pod")
		}
	} else {
		logrus.Infof("Autoscale annotations found on %v", gvk.Kind)
	}

	hpa := &v2beta1.HorizontalPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HorizontalPodAutoscaler",
			APIVersion: "autoscaling/v2beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.GetName(),
			Namespace: o.GetNamespace(),
		},
	}
	exists := true
	if err := sdk.Get(hpa); err != nil {
		logrus.Infof("HorizontalPodAutoscaler doesn't exist %s", err.Error())
		exists = false
	}

	if exists {
		logrus.Infof("HorizontalPodAutoscaler found, will be updated")
		hpa := createHorizontalPodAutoscaler(o, gvk, annotations)
		if hpa == nil {
			return nil
		}
		err := sdk.Update(hpa)
		if err != nil && !errors.IsAlreadyExists(err) {
			logrus.Errorf("Failed to update HPA: %v", err)
			return err
		}

	} else {
		logrus.Infof("HorizontalPodAutoscaler doesn't exist will be created")
		hpa := createHorizontalPodAutoscaler(o, gvk, annotations)
		if hpa == nil {
			return nil
		}
		err := sdk.Create(hpa)
		if err != nil && !errors.IsAlreadyExists(err) {
			logrus.Errorf("Failed to create HPA : %v", err)
			return err
		}
	}
	return nil
}

func (h *Handler) checkAutoscaleAnnotationIsPresent(annotations map[string]string) bool {
	for key, _ := range annotations {
		if h.annotationRegExp.MatchString(key) {
			return true
		}
	}
	return false
}

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

func createResourceMetric(resourceName v1.ResourceName, annotationName string, valueFormat string, annotationValue string, deploymentName string) *v2beta1.MetricSpec {
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
			return &v2beta1.MetricSpec{
				Type: v2beta1.ResourceMetricSourceType,
				Resource: &v2beta1.ResourceMetricSource{
					Name: resourceName,
					TargetAverageUtilization: &targetValue,
				},
			}
		}

	case targetAverageValue:
		targetValue, err := resource.ParseQuantity(annotationValue)
		if err != nil {
			logrus.Errorf("Invalid resource metric annotation: %v value for deployment %v is invalid: %v", annotationName, deploymentName, err.Error())
			return nil
		} else {
			return &v2beta1.MetricSpec{
				Type: v2beta1.ResourceMetricSourceType,
				Resource: &v2beta1.ResourceMetricSource{
					Name:               resourceName,
					TargetAverageValue: &targetValue,
				},
			}
		}
	default:
		logrus.Warnf("Invalid resource metric valueFormat: %v for deployment %v", valueFormat, deploymentName)
	}

	return nil
}

func createPodMetrics(annotationName string, metricName string, annotationValue string, deploymentName string) *v2beta1.MetricSpec {

	if len(metricName) == 0 {
		logrus.Errorf("Invalid pod metric annotation: %v for deployment %v", annotationName, deploymentName)
		return nil
	}
	logrus.Infof("pod metric %v: %v", metricName, annotationValue)

	if len(annotationValue) == 0 {
		logrus.Errorf("Invalid pod metric annotation: %v value for deployment %v is missing", annotationName, deploymentName)
		return nil
	}

	targetAverageValue, err := resource.ParseQuantity(annotationValue)
	if err != nil {
		logrus.Errorf("Invalid pod metric annotation: %v value for deployment %v is invalid: %v", annotationName, deploymentName, err.Error())
		return nil
	} else {
		return &v2beta1.MetricSpec{
			Type: v2beta1.PodsMetricSourceType,
			Pods: &v2beta1.PodsMetricSource{
				MetricName:         metricName,
				TargetAverageValue: targetAverageValue,
			},
		}
	}
}

func parseMetrics(annotations map[string]string, deploymentName string) []v2beta1.MetricSpec {

	metrics := make([]v2beta1.MetricSpec, 0, 4)

	for metricKey, metricValue := range annotations {
		keys := strings.Split(metricKey, annotationDomainSeparator)
		if len(keys) != 2 {
			logrus.Errorf("Metric annotation for deployment %v is invalid: metricKey", deploymentName, metricKey)
			return metrics
		}
		metricSubDomains := strings.Split(keys[0], annotationSubDomainSeparator)
		if len(metricSubDomains) < 2 {
			logrus.Errorf("Metric annotation for deployment %v is invalid: metricKey", deploymentName, metricKey)
			return metrics
		}
		var metric *v2beta1.MetricSpec
		switch metricSubDomains[0] {
		case cpuAnnotationPrefix:
			metric = createResourceMetric(v1.ResourceCPU, metricKey, keys[1], metricValue, deploymentName)
		case memoryAnnotationPrefix:
			metric = createResourceMetric(v1.ResourceMemory, metricKey, keys[1], metricValue, deploymentName)
		case podAnnotationPrefix:
			metric = createPodMetrics(metricKey, keys[1], metricValue, deploymentName)
		}
		if metric != nil {
			metrics = append(metrics, *metric)
		}

	}

	return metrics
}

func createHorizontalPodAutoscaler(o metav1.Object, gvk schema.GroupVersionKind, annotations map[string]string) *v2beta1.HorizontalPodAutoscaler {

	minReplicas, err := extractAnnotationIntValue(annotations, hpaAnnotationPrefix+annotationDomainSeparator+"minReplicas", o.GetName())
	if err != nil {
		logrus.Errorf("Invalid annotation: %v", err.Error())
		return nil
	}

	maxReplicas, err := extractAnnotationIntValue(annotations, hpaAnnotationPrefix+annotationDomainSeparator+"maxReplicas", o.GetName())
	if err != nil {
		logrus.Errorf("Invalid annotation: %v", err.Error())
		return nil
	}

	metrics := parseMetrics(annotations, o.GetName())
	logrus.Info("number of metrics: ", len(metrics))
	if len(metrics) == 0 {
		logrus.Error("No metrics configured")
		return nil
	}

	return &v2beta1.HorizontalPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HorizontalPodAutoscaler",
			APIVersion: "autoscaling/v2beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.GetName(),
			Namespace: o.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(o, gvk),
			},
		},
		Spec: v2beta1.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: v2beta1.CrossVersionObjectReference{
				APIVersion: gvk.GroupVersion().String(),
				Kind:       gvk.Kind,
				Name:       o.GetName(),
			},
			MinReplicas: &minReplicas,
			MaxReplicas: maxReplicas,
			Metrics:     metrics,
		},
	}
}
