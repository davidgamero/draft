package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/docker/docker/client"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var testenv env.Environment

type CreateCommandFeatureConfig struct {
	language   string
	port       string
	appName    string
	namespace  string
	deployType string
	repo       string
}
type ErrorLine struct {
	Error       string      `json:"error"`
	ErrorDetail ErrorDetail `json:"errorDetail"`
}

type ErrorDetail struct {
	Message string `json:"message"`
}

func TestKindCluster(t *testing.T) {
	c := CreateCommandFeatureConfig{
		language:   "gomodule",
		port:       "8080",
		appName:    "go-app",
		namespace:  "go-ns",
		deployType: "manifests",
		repo:       "gambtho/go_echo",
	}
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
		"--variable", fmt.Sprintf("IMAGENAME=%s", c.appName),
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
	// - run: ./draft -v create -c ./test/integration/$lang/helm.yaml -d ./langtest/

	fs := make([]features.Feature, 0)
	f1 := features.New("appsv1/deployment").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			dockerCli := ctx.Value(CONTEXT_KEY_DOCKER_CLIENT).(*client.Client)

			imageName := fmt.Sprintf("localhost:5000/%s-%s-%s", c.deployType, c.language, c.port)
			err := DockerBuildAndPush(ctx, dockerCli, imageName, repoDir)
			if err != nil {
				t.Fatalf("building and pushing dockerfile: %s", err.Error())
			}

			deployment := newDeployment(cfg.Namespace(), "test-deployment", 1)
			if err := cfg.Client().Resources().Create(ctx, deployment); err != nil {
				t.Fatal(err)
			}
			time.Sleep(2 * time.Second)
			return ctx
		}).
		Assess("deployment creation", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			var dep appsv1.Deployment
			if err := cfg.Client().Resources().Get(ctx, "test-deployment", cfg.Namespace(), &dep); err != nil {
				t.Fatal(err)
			}
			if &dep != nil {
				t.Logf("deployment found: %s", dep.Name)
			}
			return context.WithValue(ctx, "test-deployment", &dep)
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			return ctx
		}).Feature()
	fs = append(fs, f1)

	testenv.Test(t, fs...)
}

func newDeployment(namespace string, name string, replicaCount int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{"app": "test-app"}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicaCount,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test-app"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test-app"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
			},
		},
	}
}
