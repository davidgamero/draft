package e2e

import (
	"os"
	"testing"

	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kind"
)

var testenv env.Environment

func TestMain(m *testing.M) {
	testenv, _ = env.NewFromFlags()
	kindClusterName := envconf.RandomName("kind-with-config", 16)
	namespace := envconf.RandomName("kind-ns", 16)

	testenv.Setup(
		envfuncs.CreateClusterWithConfig(kind.NewProvider(), kindClusterName, "kind-config.yaml", kind.WithImage("kindest/node:v1.22.2")),
		envfuncs.CreateNamespace(namespace),
	)

	testenv.Finish(
		envfuncs.DeleteNamespace(namespace),
		envfuncs.ExportClusterLogs(kindClusterName, "./logs"),
		envfuncs.DestroyCluster(kindClusterName),
	)
	os.Exit(testenv.Run(m))
}
