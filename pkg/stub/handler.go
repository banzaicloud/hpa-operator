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
	"strconv"
	"strings"
)

const hpaAnnotationPrefix = "autoscale"

func NewHandler() sdk.Handler {
	return &Handler{}
}

type Handler struct {
	// Fill me
}

func (h *Handler) Handle(ctx context.Context, event sdk.Event) error {
	switch o := event.Object.(type) {
	case *appsv1.Deployment:
		return handleReplicaController(event.Deleted, o, o.GroupVersionKind(), o.Spec.Template.Annotations)
	case *appsv1.StatefulSet:
		return handleReplicaController(event.Deleted, o, o.GroupVersionKind(), o.Spec.Template.Annotations)
	}
	return nil
}

func handleReplicaController(deleted bool, o metav1.Object, gvk schema.GroupVersionKind, podAnnotations map[string]string) error {
	logrus.Infof("handle  : %v", o.GetName())
	annotations := o.GetAnnotations()
	if !checkAutoscaleAnnotationIsPresent(annotations) {
		annotations = podAnnotations
		if !checkAutoscaleAnnotationIsPresent(annotations) {
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
		if deleted {
			logrus.Infof("HorizontalPodAutoscaler found, will be deleted!")
			err := sdk.Delete(hpa)
			if err != nil && !errors.IsAlreadyExists(err) {
				logrus.Errorf("Failed to delete HPA: %v", err)
				return err
			}
		} else {
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

func checkAutoscaleAnnotationIsPresent(annotations map[string]string) bool {
	for key, _ := range annotations {
		if strings.HasPrefix(key, hpaAnnotationPrefix) {
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

func addCpuMetric(annotations map[string]string, annotationName string, deploymentName string, metrics []v2beta1.MetricSpec) []v2beta1.MetricSpec {
	strValue := annotations[annotationName]
	if len(strValue) == 0 {
		return metrics
	}
	int64Value, err := strconv.ParseInt(strValue, 10, 32)
	if err != nil {
		logrus.Errorf("Invalid annotation: %v value for deployment %v is invalid: %v", annotationName, deploymentName, err.Error())
		return metrics
	}
	cpuTargetValue := int32(int64Value)
	if cpuTargetValue <= 0 || cpuTargetValue > 100 {
		logrus.Errorf("Invalid annotation: %v value for deployment %v should be a percentage value between [1,99]", annotationName, deploymentName)
		return metrics
	}

	if cpuTargetValue > 0 {
		logrus.Info("add cpu")
		return append(metrics, v2beta1.MetricSpec{
			Type: v2beta1.ResourceMetricSourceType,
			Resource: &v2beta1.ResourceMetricSource{
				Name: v1.ResourceCPU,
				TargetAverageUtilization: &cpuTargetValue,
			},
		})
	}
	return metrics
}

func addMemoryMetric(annotations map[string]string, annotationName string, deploymentName string, metrics []v2beta1.MetricSpec) []v2beta1.MetricSpec {
	memoryTargetValueStr := annotations[annotationName]
	if len(memoryTargetValueStr) == 0 {
		return metrics
	}

	memoryTargetValue, err := resource.ParseQuantity(memoryTargetValueStr)
	if err != nil {
		logrus.Errorf("Invalid annotation: %v value for deployment %v is invalid: %v", annotationName, deploymentName, err.Error())
		return metrics
	} else {
		return append(metrics, v2beta1.MetricSpec{
			Type: v2beta1.ResourceMetricSourceType,
			Resource: &v2beta1.ResourceMetricSource{
				Name:               v1.ResourceMemory,
				TargetAverageValue: &memoryTargetValue,
			},
		})
	}
	return metrics
}

func addPodMetrics(annotations map[string]string, annotationPrefix string, deploymentName string, metrics []v2beta1.MetricSpec) []v2beta1.MetricSpec {

	for metricKey, metricValue := range annotations {
		if strings.HasPrefix(metricKey, annotationPrefix) {
			metricName := metricKey[len(annotationPrefix)+1:]
			logrus.Infof("pod metric %v: %v", metricName, metricValue)

			if len(metricValue) == 0 {
				logrus.Errorf("Invalid annotation: %v value for deployment %v is missing", metricKey, deploymentName)
				continue
			}

			targetAverageValue, err := resource.ParseQuantity(metricValue)
			if err != nil {
				logrus.Errorf("Invalid annotation: %v value for deployment %v is invalid: %v", metricKey, deploymentName, err.Error())
				continue
			} else {
				metrics = append(metrics, v2beta1.MetricSpec{
					Type: v2beta1.PodsMetricSourceType,
					Pods: &v2beta1.PodsMetricSource{
						MetricName:         metricName,
						TargetAverageValue: targetAverageValue,
					},
				})
			}

		}
	}
	return metrics
}

func createHorizontalPodAutoscaler(o metav1.Object, gvk schema.GroupVersionKind, annotations map[string]string) *v2beta1.HorizontalPodAutoscaler {

	minReplicas, err := extractAnnotationIntValue(annotations, hpaAnnotationPrefix+"/minReplicas", o.GetName())
	if err != nil {
		logrus.Errorf("Invalid annotation: %v", err.Error())
		return nil
	}

	maxReplicas, err := extractAnnotationIntValue(annotations, hpaAnnotationPrefix+"/maxReplicas", o.GetName())
	if err != nil {
		logrus.Errorf("Invalid annotation: %v", err.Error())
		return nil
	}

	metrics := make([]v2beta1.MetricSpec, 0, 4)
	metrics = addCpuMetric(annotations, hpaAnnotationPrefix+"/cpu", o.GetName(), metrics)
	metrics = addMemoryMetric(annotations, hpaAnnotationPrefix+"/memory", o.GetName(), metrics)
	metrics = addPodMetrics(annotations, hpaAnnotationPrefix+".pod", o.GetName(), metrics)

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
