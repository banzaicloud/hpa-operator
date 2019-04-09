package stub

import (
	"context"
	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/autoscaling/v2beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"regexp"
)

const hpaAnnotationPrefix = "hpa.autoscaling.banzaicloud.io"

const cpuAnnotationPrefix = "cpu"
const memoryAnnotationPrefix = "memory"
const prometheusAnnotationPrefix = "prometheus"

const targetAverageUtilization = "targetAverageUtilization"
const targetAverageValue = "targetAverageValue"
const annotationDomainSeparator = "/"
const annotationSubDomainSeparator = "."

const annotationRegExpString = "[a-zA-Z\\.]+hpa\\.autoscaling\\.banzaicloud\\.io\\/[a-zA-Z\\.]+"

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
		return h.handleReplicaSet(o, o.GroupVersionKind(), o.Spec.Template.Annotations)
	case *appsv1.StatefulSet:
		return h.handleReplicaSet(o, o.GroupVersionKind(), o.Spec.Template.Annotations)
	}
	return nil
}

func (h *Handler) handleReplicaSet(o metav1.Object, gvk schema.GroupVersionKind, podAnnotations map[string]string) error {
	logrus.Infof("handle  : %v", o.GetName())
	annotations := o.GetAnnotations()
	hpaAnnotationsFound := false
	if !h.checkAutoscaleAnnotationIsPresent(annotations) {
		annotations = podAnnotations
		if !h.checkAutoscaleAnnotationIsPresent(annotations) {
			logrus.Infof("Autoscale annotations not found")
		} else {
			hpaAnnotationsFound = true
			logrus.Infof("Autoscale annotations found on Pod")
		}
	} else {
		hpaAnnotationsFound = true
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

		if hpaAnnotationsFound {
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
			logrus.Infof("HorizontalPodAutoscaler found, will be deleted")
			err := sdk.Delete(hpa)
			if err != nil {
				logrus.Errorf("Failed to delete HPA : %v", err)
				return err
			}
		}

	} else if hpaAnnotationsFound {
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

	hpa := &v2beta1.HorizontalPodAutoscaler{
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
		},
	}

	metrics := parseMetrics(hpa, annotations, o.GetName())
	logrus.Info("number of metrics: ", len(metrics))
	if len(metrics) == 0 {
		logrus.Error("No metrics configured")
		return nil
	}

	hpa.Spec.Metrics = metrics

	return hpa
}
