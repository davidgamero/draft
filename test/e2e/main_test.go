package e2e

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kind"
	"sigs.k8s.io/kind/pkg/cluster"
)

const ENV_DRAFT_BIN_KEY = "DRAFT_E2E_BIN"
const KIND_CLUSTER_PREFIX = "draft-e2e"
const REG_CONTAINER_NAME = "kind-registry"

var CONTEXT_KEY_DOCKER_CLIENT = struct{}{}

// AddLocalRegistryConfigMap creates a configmap entry in kind cluster n
// See https://kind.sigs.k8s.io/docs/user/local-registry/
func AddLocalRegistryConfigMap(n string) func(context.Context, *envconf.Config) (context.Context, error) {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		dockerCli, err := client.NewClientWithOpts(
			client.FromEnv,
			client.WithAPIVersionNegotiation(),
		)
		if err != nil {
			return ctx, fmt.Errorf("creating docker client: %w", err)
		}
		ctx = context.WithValue(ctx, CONTEXT_KEY_DOCKER_CLIENT, dockerCli)
		containerPort := "5000" // Port to be exposed inside the container
		hostPort := "5000"      // Port to be exposed on the host
		hostIP := "127.0.0.1"   // Host IP address to bind the port

		// Create host configuration

		log.Println("validating local registry container is running")
		regContainer, err := dockerCli.ContainerInspect(ctx, REG_CONTAINER_NAME)
		if err != nil {
			containerConfig := &container.Config{
				ExposedPorts: map[nat.Port]struct{}{
					nat.Port(containerPort + "/tcp"): {},
				},
				Image: "registry:2",
			}
			hostConfig := &container.HostConfig{
				PortBindings: nat.PortMap{
					nat.Port(containerPort + "/tcp"): []nat.PortBinding{
						{
							HostIP:   hostIP,
							HostPort: hostPort,
						},
					},
				},
			}
			netConfig := &network.NetworkingConfig{
				EndpointsConfig: map[string]*network.EndpointSettings{
					"bridge": {
						NetworkID: "bridge",
					},
				},
			}

			resp, err := dockerCli.ContainerCreate(ctx,
				containerConfig,
				hostConfig,
				netConfig,
				nil,
				REG_CONTAINER_NAME,
			)
			if err != nil {
				return ctx, fmt.Errorf("creating new registry container: %w", err)
			}

			regContainer, err = dockerCli.ContainerInspect(ctx, REG_CONTAINER_NAME)
			if err != nil {
				return ctx, fmt.Errorf("inspecting created registry container with ID=%s : %w", resp.ID, err)
			}
		}
		log.Printf("using registry container with ID=%s", regContainer.ID)
		if !regContainer.State.Running {
			err := dockerCli.ContainerStart(ctx, regContainer.ID, container.StartOptions{})
			if err != nil {
				return ctx, fmt.Errorf("starting registry container with ID=%s: %w", regContainer.ID, err)
			}
		}

		configMap := corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "local-registry-hosting",
				Namespace: "kube-public",
			},
			Data: map[string]string{
				"localRegistryHosting.v1.host": fmt.Sprintf("localhost:%s", hostPort),
				"localRegistryHosting.v1.help": "https://kind.sigs.k8s.io/docs/user/local-registry/",
			},
		}
		log.Println("applying local registry configmap")
		err = cfg.Client().Resources().Create(ctx, &configMap)
		if err != nil {
			return ctx, fmt.Errorf("creating local registry configmap: %w", err)
		}
		return ctx, nil
	}
}

func TestMain(m *testing.M) {
	log.Println("testing draft e2e...")
	draftBinaryPath := os.Getenv(ENV_DRAFT_BIN_KEY)
	if draftBinaryPath == "" {
		draftBinaryPath = "/workspaces/draft/draft"
		log.Printf("no DRAFT_BIN_KEY environment variable set, using default value of '%s'", draftBinaryPath)
		// panic(fmt.Sprintf("missing env var %s for draft binary path ", ENV_DRAFT_BIN_KEY))
	}
	testenv, _ = env.NewFromFlags()
	kindClusterName := envconf.RandomName("draft-e2e", 16)
	namespace := envconf.RandomName("draft-e2e-ns", 16)
	log.Println("creating kind cluster test env")

	kindProvider := cluster.NewProvider(cluster.ProviderWithDocker())
	clusters, err := kindProvider.List()
	if err != nil {
		panic(err)
	}

	for _, c := range clusters {
		if strings.HasPrefix(c, KIND_CLUSTER_PREFIX) {
			log.Printf("cleaning up old e2e cluster: %s\n", c)
			f, err := os.CreateTemp("", "e2e-kubeconfig")
			if err != nil {
				panic(err)
			}
			kindProvider.ExportKubeConfig(c, f.Name(), false)
			kindProvider.Delete(c, f.Name())
		}
	}

	testenv.Setup(
		envfuncs.CreateClusterWithConfig(kind.NewProvider(), kindClusterName, "kind-config.yaml", kind.WithImage("kindest/node:v1.28.7@sha256:9bc6c451a289cf96ad0bbaf33d416901de6fd632415b076ab05f5fa7e4f65c58")),
		envfuncs.CreateNamespace(namespace),
		AddLocalRegistryConfigMap(kindClusterName),
	)

	testenv.Finish(
	// envfuncs.DeleteNamespace(namespace),
	// This line is commented because the whole test hangs indefinitely every time we try to export cluster logs
	// envfuncs.ExportClusterLogs(kindClusterName, "./logs"),
	// envfuncs.DestroyCluster(kindClusterName),
	)
	os.Exit(testenv.Run(m))
}
