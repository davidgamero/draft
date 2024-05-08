package e2e

import (
	"log"
	"os"
	"strings"
	"testing"

	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kind"

	"sigs.k8s.io/kind/pkg/cluster"
)

const ENV_DRAFT_BIN_KEY = "DRAFT_E2E_BIN"
const KIND_CLUSTER_PREFIX = "draft-e2e"

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
	)

	testenv.Finish(
	// envfuncs.DeleteNamespace(namespace),
	// This line is commented because the whole test hangs indefinitely every time we try to export cluster logs
	// envfuncs.ExportClusterLogs(kindClusterName, "./logs"),
	// envfuncs.DestroyCluster(kindClusterName),
	)
	os.Exit(testenv.Run(m))
}
