package minikube

import (
	"os/exec"
)

// ValidateMinikube checks if minikube is installed
func ValidateMinikube() error {
	_, err := exec.LookPath("minikube")
	return err
}
