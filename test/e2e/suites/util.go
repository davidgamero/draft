package suites

import (
	"context"
	"os/exec"

	"k8s.io/client-go/kubernetes"
)

type test struct {
	name string
	run  func(context.Context, testConfig) error
}

type testConfig interface {
	RunDraftCommand(ctx context.Context, args ...string) ([]byte, error)
	GetK8sClient() *kubernetes.Clientset
}

var _ testConfig = &draftTestConfig{}

type draftTestConfig struct {
	draftBinPath string                // path to draft binary for use in this run
	k8sClient    *kubernetes.Clientset // k8s client for use in this run
}

func (c *draftTestConfig) RunDraftCommand(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.Command(c.draftBinPath, args...)
	return cmd.CombinedOutput()
}

func (c *draftTestConfig) GetK8sClient() *kubernetes.Clientset {
	return c.k8sClient
}
