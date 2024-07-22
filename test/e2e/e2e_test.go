package e2e

import (
	_ "embed"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var testenv env.Environment

//go:embed tester/tester.go
var testerContents string

type ErrorLine struct {
	Error       string      `json:"error"`
	ErrorDetail ErrorDetail `json:"errorDetail"`
}

type ErrorDetail struct {
	Message string `json:"message"`
}

func TestKindCluster(t *testing.T) {
	featuresToTest := make([]features.Feature, 0)
	createCommandFeatureConfigs := []CreateCommandFeatureConfig{
		{
			language:   "gomodule",
			deployType: "manifests",
			port:       "1323",
			repo:       "davidgamero/go_echo",
			version:    "1.22",
		},
	}

	for _, c := range createCommandFeatureConfigs {
		featuresToTest = append(featuresToTest, NewDraftCreateFeature(c))
	}

	testenv.Test(t, featuresToTest...)
}

func newTesterDeployment(namespace string, language string, deployType string, testURL string, contents string) *appsv1.Deployment {
	appName := fmt.Sprintf("%s-%s-tester", language, deployType)
	command := []string{
		"/bin/sh",
		"-c",
		"mkdir source && cd source && go mod init source && echo '" + contents + "' > main.go && go mod tidy && go run main.go",
	}

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: to.Ptr(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": appName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{"app": appName},
					Annotations: map[string]string{},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    "container",
						Image:   "mcr.microsoft.com/oss/go/microsoft/golang:1.22",
						Command: command,
						Env: []corev1.EnvVar{
							{
								Name:  "URL",
								Value: testURL,
							},
						},
						ReadinessProbe: &corev1.Probe{
							FailureThreshold:    1,
							InitialDelaySeconds: 5,
							PeriodSeconds:       2,
							SuccessThreshold:    1,
							TimeoutSeconds:      30,
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path:   "/",
									Port:   intstr.FromInt(8080),
									Scheme: corev1.URISchemeHTTP,
								},
							},
						},
					}},
				},
			},
		},
	}
}
