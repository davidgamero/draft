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

	deployTypes := []string{"manifests", "helm", "kustomize"}
	createCommandFeatureConfigs := []CreateCommandFeatureConfig{}

	for _, deployType := range deployTypes {
		createCommandFeatureConfigs = append(createCommandFeatureConfigs, []CreateCommandFeatureConfig{
			{
				deployType: deployType,
				language:   "gomodule",
				version:    "1.22.0",
				port:       "1323",
				repo:       "gambtho/go_echo",
			},
			{
				deployType: deployType,
				language:   "go",
				version:    "1.22.0",
				port:       "8080",
				repo:       "davidgamero/go-echo-no-mod",
			},
			{
				deployType: deployType,
				language:   "python",
				version:    "3",
				port:       "5000",
				repo:       "OliverMKing/flask-hello-world",
			},
			{
				deployType: deployType,
				language:   "rust",
				version:    "1.77.0",
				port:       "8000",
				repo:       "OliverMKing/tiny-http-hello-world",
			},
			{
				deployType: deployType,
				language:   "javascript",
				version:    "14",
				port:       "1313",
				repo:       "davidgamero/express-hello-world",
			},
			{
				deployType: deployType,
				language:   "ruby",
				version:    "3.1.2",
				port:       "8000",
				repo:       "OliverMKing/ruby-hello-world",
			},
			{
				deployType: deployType,
				language:   "csharp",
				version:    "5.0",
				port:       "80",
				repo:       "imiller31/csharp-simple-web-app",
			},
			{
				deployType:     deployType,
				language:       "java",
				version:        "11-jre",
				builderVersion: "3-jdk-11",
				port:           "8080",
				repo:           "imiller31/simple-java-server",
			},
			{
				deployType:     deployType,
				language:       "gradle",
				version:        "11-jre",
				builderVersion: "7-jdk11",
				port:           "8080",
				repo:           "imiller31/simple-gradle-server",
			},
			{
				deployType: deployType,
				language:   "swift",
				version:    "5.5",
				port:       "8080",
				repo:       "OliverMKing/swift-hello-world",
			},
			{
				deployType:     deployType,
				language:       "erlang",
				version:        "3.15",
				builderVersion: "24.2-alpine",
				port:           "8080",
				repo:           "bfoley13/ErlangExample",
			},
			{
				deployType: deployType,
				language:   "clojure",
				version:    "8-jdk-alpine",
				port:       "8080",
				repo:       "imiller31/clojure-simple-http",
			},
		}...)
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
