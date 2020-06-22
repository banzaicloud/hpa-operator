package stub

import (
	"context"
	"github.com/sirupsen/logrus"
	"k8s.io/api/autoscaling/v2beta2"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"regexp"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func NewHandler(client client.Client) *HPAHandler {
	r, _ := regexp.Compile(annotationRegExpString)
	return &HPAHandler{
		annotationRegExp: r,
		client:           client,
	}
}

type HPAHandler struct {
	annotationRegExp *regexp.Regexp
	client           client.Client
}

func (h *HPAHandler) HandleReplicaSet(
	ctx context.Context,
	UID types.UID,
	name string, namespace string,
	kind string, apiVersion string,
	annotations map[string]string, podAnnotations map[string]string) error {

	logrus.Infof("handle  : %v", name)
	hpaAnnotationsFound := false
	hpaAnnotations := h.filterAutoscaleAnnotations(annotations)
	if len(hpaAnnotations) > 0 {
		hpaAnnotationsFound = true
		logrus.Infof("Autoscale annotations found on %v", kind)
	} else {
		hpaAnnotations = h.filterAutoscaleAnnotations(podAnnotations)
		if len(hpaAnnotations) > 0 {
			hpaAnnotationsFound = true
			logrus.Infof("Autoscale annotations found on Pod")
		} else {
			hpaAnnotationsFound = false
			logrus.Infof("Autoscale annotations not found")
		}
	}

	hpa := v2beta2.HorizontalPodAutoscaler{}
	exists := true
	namespacedName := client.ObjectKey{
		Name:      name,
		Namespace: namespace,
	}
	if err := h.client.Get(ctx, namespacedName, &hpa); err != nil {
		logrus.Infof("HorizontalPodAutoscaler doesn't exist %s", err.Error())
		exists = false
	}

	if exists {
		if !isCreatedByHpaController(&hpa, name, kind) {
			logrus.Infof("HorizontalPodAutoscaler is not created by us")
			return nil
		}

		if hpaAnnotationsFound {
			logrus.Infof("HorizontalPodAutoscaler found, will be updated")
			hpa := createHorizontalPodAutoscaler(UID, name, namespace, kind, apiVersion, hpaAnnotations)
			if hpa == nil {
				return nil
			}
			err := h.client.Update(ctx, hpa)
			if err != nil && !errors.IsAlreadyExists(err) {
				logrus.Errorf("Failed to update HPA: %v", err)
				return err
			}
		} else {
			logrus.Infof("HorizontalPodAutoscaler found, will be deleted")

			err := h.client.Delete(ctx, &hpa)
			if err != nil {
				logrus.Errorf("Failed to delete HPA : %v", err)
				return err
			}
		}

	} else if hpaAnnotationsFound {
		logrus.Infof("HorizontalPodAutoscaler doesn't exist will be created")
		hpa := createHorizontalPodAutoscaler(UID, name, namespace, kind, apiVersion, hpaAnnotations)
		if hpa == nil {
			return nil
		}
		err := h.client.Create(ctx, hpa)
		if err != nil && !errors.IsAlreadyExists(err) {
			logrus.Errorf("Failed to create HPA : %v", err)
			return err
		}
	}
	return nil
}

func isCreatedByHpaController(hpa *v2beta2.HorizontalPodAutoscaler, name string, kind string) bool {
	for _, ref := range hpa.OwnerReferences {
		if ref.Name == name && ref.Kind == kind {
			return true
		}
	}
	return false
}

func (h *HPAHandler) filterAutoscaleAnnotations(annotations map[string]string) map[string]string {
	autoscaleAnnotations := make(map[string]string)
	for key, value := range annotations {
		if h.annotationRegExp.MatchString(key) {
			autoscaleAnnotations[key] = value
		}
	}
	return autoscaleAnnotations
}

func createHorizontalPodAutoscaler(UID types.UID, name string, namespace string, kind string, apiVersion string, annotations map[string]string) *v2beta2.HorizontalPodAutoscaler {

	minReplicas, err := extractAnnotationIntValue(annotations, hpaAnnotationPrefix+annotationDomainSeparator+"minReplicas", name)
	if err != nil {
		logrus.Errorf("Invalid annotation: %v", err.Error())
		return nil
	}

	maxReplicas, err := extractAnnotationIntValue(annotations, hpaAnnotationPrefix+annotationDomainSeparator+"maxReplicas", name)
	if err != nil {
		logrus.Errorf("Invalid annotation: %v", err.Error())
		return nil
	}

	blockOwnerDeletion := true
	isController := true

	ref := metav1.OwnerReference{
		APIVersion:         apiVersion,
		Kind:               kind,
		Name:               name,
		UID:                UID,
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}

	hpa := &v2beta2.HorizontalPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HorizontalPodAutoscaler",
			APIVersion: "autoscaling/v2beta2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				ref,
			},
		},
		Spec: v2beta2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: v2beta2.CrossVersionObjectReference{
				APIVersion: apiVersion,
				Kind:       kind,
				Name:       name,
			},
			MinReplicas: &minReplicas,
			MaxReplicas: maxReplicas,
		},
	}

	metrics := parseMetrics(hpa, annotations, name)
	logrus.Info("number of metrics: ", len(metrics))
	if len(metrics) == 0 {
		logrus.Error("No metrics configured")
		return nil
	}

	hpa.Spec.Metrics = metrics

	return hpa
}
