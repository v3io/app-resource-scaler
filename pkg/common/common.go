package common

import (
	"os"
	"strings"

	"github.com/nuclio/logger"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// GetNamespace returns the namespace to use
func GetNamespace(namespaceArgument string) string {

	// if the namespace was passed in the arguments, use that
	if namespaceArgument != "" {
		return namespaceArgument
	}

	// if the namespace exists in env, use that
	if namespaceEnv := os.Getenv("SCALER_NAMESPACE"); namespaceEnv != "" {
		return namespaceEnv
	}

	// get namespace from within the pod. if found, return that
	if namespacePod, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		return string(namespacePod)
	}

	return "default"
}

// GetClientConfig returns a client config based on the kubeconfig path
func GetClientConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}

	return rest.InClusterConfig()
}

type KubernetesClientWarningHandler struct {
	logger logger.Logger
}

func NewKubernetesClientWarningHandler(logger logger.Logger) *KubernetesClientWarningHandler {
	return &KubernetesClientWarningHandler{
		logger: logger,
	}
}

// HandleWarningHeader handles miscellaneous warning messages yielded by Kubernetes api server
// e.g.: "autoscaling/v2beta1 HorizontalPodAutoscaler is deprecated in v1.22+, unavailable in v1.25+; use autoscaling/v2beta2 HorizontalPodAutoscaler"
// Note: code is determined by the Kubernetes server
func (kcl *KubernetesClientWarningHandler) HandleWarningHeader(code int, agent string, message string) {
	if code != 299 || len(message) == 0 {
		return
	}

	// special handling for deprecation warnings
	if strings.Contains(message, "is deprecated") {
		kcl.logger.WarnWith("Kubernetes deprecation alert", "message", message, "agent", agent)
		return
	}
	kcl.logger.WarnWith(message, "agent", agent)
}
