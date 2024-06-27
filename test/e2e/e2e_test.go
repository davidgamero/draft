package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/client"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
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
			c := CreateCommandFeatureConfig{
				language:   "gomodule",
				port:       "8080",
				appName:    "go-app",
				namespace:  cfg.Namespace(),
				deployType: "manifests",
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

			deployment := newDeployment(cfg.Namespace(), "test-deployment", 1)
			if err := cfg.Client().Resources().Create(ctx, deployment); err != nil {
				t.Fatal(err)
			}
			// decode := scheme.Codecs.UniversalDeserializer().Decode

			// please let me just kubectl apply some yaml from a directory what have i done to deserve reading these KEPs
			// https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/2155-clientgo-apply
			// https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/555-server-side-apply
			// https://github.com/kubernetes/enhancements/issues/555
			//
			// just apply all the yamls in this dir and set me free
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
					gvk := u.GroupVersionKind()

					t.Logf("applying yaml %s", path)
					var o k8s.Object
					gvkString := fmt.Sprintf("%s,%s,%s", gvk.Group, gvk.Version, gvk.Kind)
					t.Logf("processing gvk: %s", gvkString)
					switch gvkString {
					case "apps,v1,Deployment":
						d := &appsv1.Deployment{}
						err := yaml.Unmarshal(b, d)
						if err != nil {
							return fmt.Errorf("marshaling yaml file %s into deployment: %w", path, err)
						}
						o = d
					case "networking.k8s.io,v1,Ingress":
						i := &networkingv1.Ingress{}
						err := yaml.Unmarshal(b, i)
						if err != nil {
							return fmt.Errorf("marshaling yaml file %s into ingress: %w", path, err)
						}
						o = i
					case ",v1,Service":
						s := &corev1.Service{}
						err := yaml.Unmarshal(b, s)
						if err != nil {
							return fmt.Errorf("marshaling yaml file %s into ingress: %w", path, err)
						}
						o = s
					default:
						return fmt.Errorf("marshaling yaml file %s into ingress: %w", path, err)
					}
					if err := cfg.Client().Resources().Create(ctx, o); err != nil {
						return fmt.Errorf("creating resource for yaml file %s: %w", path, err)
					}
				}
				return nil
			})
			if err != nil {
				t.Errorf("applying manifest yaml: %s", err.Error())
			}

			time.Sleep(2 * time.Second)
			return ctx
		}).
		Assess("deployment creation", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			var dep appsv1.Deployment
			if err := cfg.Client().Resources().Get(ctx, "test-deployment", cfg.Namespace(), &dep); err != nil {
				t.Fatal(err)
			}
			testJob := batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cfg.Namespace(),
					Name:      "test-job",
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-container",
									Image: "alpine",
									Command: []string{
										"wget",
										"go-app.svc.cluster.local",
									},
								},
							},
							RestartPolicy: corev1.RestartPolicyNever,
						},
					},
				},
			}

			if err := cfg.Client().Resources().Create(ctx, &testJob); err != nil {
				t.Fatal(err)
			}

			err := wait.For(conditions.New(cfg.Client().Resources()).JobCompleted(&testJob), wait.WithTimeout(1*time.Minute))
			if err != nil {
				t.Fatalf("waiting for job to complete: %s", err.Error())
			}

			t.Fail()

			return context.WithValue(ctx, "test-deployment", &dep)
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			return ctx
		}).Feature()
	featuresToTest = append(featuresToTest, f1)

	testenv.Test(t, featuresToTest...)
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
