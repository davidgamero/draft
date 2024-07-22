package e2e

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/docker/docker/client"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	bo "github.com/cenkalti/backoff/v4"
)

var testenv env.Environment

//go:embed tester/tester.go
var testerContents string

type CreateCommandFeatureConfig struct {
	language   string
	port       string
	appName    string
	namespace  string
	deployType string
	repo       string
	imageName  string
	version    string
}
type ErrorLine struct {
	Error       string      `json:"error"`
	ErrorDetail ErrorDetail `json:"errorDetail"`
}

type ErrorDetail struct {
	Message string `json:"message"`
}

func TestKindCluster(t *testing.T) {
	featuresToTest := make([]features.Feature, 0)
	f1 := features.New("appsv1/deployment").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			language := "gomodule"
			deployType := "manifests"
			c := CreateCommandFeatureConfig{
				language:   language,
				port:       "1323",
				appName:    fmt.Sprintf("%s-%s", language, deployType),
				namespace:  cfg.Namespace(),
				deployType: deployType,
				repo:       "davidgamero/go_echo",
				version:    "1.22",
			}
			imageName := fmt.Sprintf("localhost:5000/%s-%s-%s", c.deployType, c.language, c.port)
			c.imageName = imageName

			draftBinaryPath := os.Getenv(ENV_DRAFT_BIN_KEY)

			repoDir, err := os.MkdirTemp("", "create-command")
			t.Logf("creating tmp dir: %s", repoDir)
			if err != nil {
				t.Fatal(err)
			}

			repoURL := fmt.Sprintf("https://github.com/%s", c.repo)
			t.Logf("cloning %s into %s", repoURL, repoDir)
			cloneCmd := exec.Command("git", "clone", repoURL, ".")
			cloneCmd.Dir = repoDir
			err = cloneCmd.Run()
			if err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command(draftBinaryPath, "-v", "create",
				"-l", c.language,
				"--deploy-type", c.deployType,
				"--skip-file-detection", // overwrite existing files like Dockerfile and manifests
				"--variable", fmt.Sprintf("PORT=%s", c.port),
				"--variable", fmt.Sprintf("SERVICEPORT=%s", c.port),
				"--variable", "VERSION=1.22",
				"--variable", fmt.Sprintf("NAMESPACE=%s", c.namespace),
				"--variable", fmt.Sprintf("APPNAME=%s", c.appName),
				"--variable", fmt.Sprintf("IMAGENAME=%s", c.imageName),
				"--variable", fmt.Sprintf("IMAGETAG=%s", "latest"),
			)
			cmd.Dir = repoDir
			var outb, errb bytes.Buffer
			cmd.Stdout = &outb
			cmd.Stderr = &errb
			err = cmd.Run()
			t.Log("out:", outb.String(), "err:", errb.String())
			if err != nil {
				t.Fatal(err)
			}

			dockerCli := ctx.Value(CONTEXT_KEY_DOCKER_CLIENT).(*client.Client)

			err = DockerBuildAndPush(ctx, dockerCli, imageName, repoDir)
			if err != nil {
				t.Fatalf("building and pushing dockerfile: %s", err.Error())
			}

			// apply the generated yamls
			manifestPath := filepath.Join(repoDir, "manifests")
			err = filepath.WalkDir(manifestPath, func(path string, d fs.DirEntry, err error) error {
				isYaml := strings.HasSuffix(d.Name(), ".yaml") || strings.HasSuffix(d.Name(), ".yml")
				if !d.IsDir() && isYaml {

					t.Logf("reading generated yaml file: %s", path)
					b, err := os.ReadFile(path)
					if err != nil {
						return fmt.Errorf("reading yaml file %s: %w", path, err)
					}
					var u unstructured.Unstructured
					err = yaml.Unmarshal(b, &u)
					if err != nil {
						return fmt.Errorf("marshaling yaml file %s into unstructured: %w", path, err)
					}

					if err := cfg.Client().Resources().Create(ctx, &u); err != nil {
						return fmt.Errorf("creating resource for yaml file %s: %w", path, err)
					}
				}
				return nil
			})
			if err != nil {
				t.Errorf("applying manifest yaml: %s", err.Error())
			}

			testerDeployment := newTesterDeployment(cfg.Namespace(), language, deployType, fmt.Sprintf("http://%s.%s.svc.cluster.local:%s", c.appName, cfg.Namespace(), c.port), testerContents)
			if err := cfg.Client().Resources().Create(ctx, testerDeployment); err != nil {
				t.Fatal(err)
			}

			return context.WithValue(ctx, "tester-deployment-name", testerDeployment)
		}).
		Assess("deployment creation", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

			deployName := ctx.Value("tester-deployment-name").(*appsv1.Deployment).Name

			backoff := bo.NewExponentialBackOff()
			backoff.MaxElapsedTime = 120 * time.Second

			var dep appsv1.Deployment
			err := bo.Retry(func() error {
				if err := cfg.Client().Resources().Get(ctx, deployName, cfg.Namespace(), &dep); err != nil {
					return fmt.Errorf("getting tester deployment: %w", err)
				}

				if dep.Status.ReadyReplicas == 0 {
					t.Logf("deployment %s has 0 replicas, waiting", deployName)
					return fmt.Errorf("deployment %s has 0 replicas", deployName)
				}

				return nil
			}, backoff)

			if err != nil {
				t.Fatal(err)
			}

			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			return ctx
		}).Feature()
	featuresToTest = append(featuresToTest, f1)

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
