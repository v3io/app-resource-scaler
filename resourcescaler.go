package main

import (
	"encoding/json"
	"fmt"
	"k8s.io/apimachinery/pkg/types"
	"os"
	"time"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	"github.com/nuclio/zap"
	"github.com/v3io/scaler-types"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type AppResourceScaler struct {
	logger        logger.Logger
	namespace     string
	kubeClientSet kubernetes.Interface
}

func New(kubeconfigPath string, namespace string) (scaler_types.ResourceScaler, error) {
	rLogger, err := nucliozap.NewNuclioZap("resourcescaler", "console", os.Stdout, os.Stderr, nucliozap.DebugLevel)
	if err != nil {
		return nil, errors.Wrap(err, "Failed creating a new logger")
	}

	kubeconfig, err := getClientConfig(kubeconfigPath)
	if err != nil {
		rLogger.WarnWith("Could not parse kubeconfig from path", "kubeconfigPath", kubeconfigPath)
		return nil, errors.Wrap(err, "Failed parsing cluster's kubeconfig from path")
	}

	kubeClientSet, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return nil, errors.Wrap(err, "Failed creating kubeclient from kubeconfig")
	}

	return &AppResourceScaler{
		logger:        rLogger,
		namespace:     namespace,
		kubeClientSet: kubeClientSet,
	}, nil
}

func (s *AppResourceScaler) SetScale(resource scaler_types.Resource, scale int) error {
	if scale == 0 {
		return s.scaleServiceToZero(s.namespace, string(resource))
	}
	return s.scaleServiceFromZero(s.namespace, string(resource))
}

func (s *AppResourceScaler) scaleServiceFromZero(namespace string, serviceName string) error {
	var jsonPatchMapper []map[string]string
	s.logger.DebugWith("Scaling from zero", "namespace", namespace, "serviceName", serviceName)
	path := fmt.Sprintf("/spec/spec/tenants/0/spec/services/%s/state", string(serviceName))
	jsonPatchMapper = append(jsonPatchMapper, map[string]string{
		"op":    "add",
		"path":  path,
		"value": "scaledFromZero",
	})

	jsonPatchMapper = append(jsonPatchMapper, map[string]string{
		"op":    "add",
		"path":  "/status/state",
		"value": "waitingForProvisioning",
	})

	err := s.patchIguazioTenantAppServiceSets(namespace, jsonPatchMapper)

	if err != nil {
		return errors.Wrap(err, "Failed to patch iguazio tenant app service sets")
	}

	return s.waitForServiceReadiness(namespace, serviceName)
}

func (s *AppResourceScaler) scaleServiceToZero(namespace string, serviceName string) error {
	var jsonPatchMapper []map[string]string
	s.logger.DebugWith("Scaling to zero", "namespace", namespace, "serviceName", serviceName)
	path := fmt.Sprintf("/spec/spec/tenants/0/spec/services/%s/state", string(serviceName))
	jsonPatchMapper = append(jsonPatchMapper, map[string]string{
		"op":    "add",
		"path":  path,
		"value": "scaledToZero",
	})

	jsonPatchMapper = append(jsonPatchMapper, map[string]string{
		"op":    "add",
		"path":  "/status/state",
		"value": "waitingForProvisioning",
	})

	return s.patchIguazioTenantAppServiceSets(namespace, jsonPatchMapper)
}

func (s *AppResourceScaler) patchIguazioTenantAppServiceSets(namespace string, jsonPatchMapper []map[string]string) error {
	body, err := json.Marshal(jsonPatchMapper)
	s.logger.DebugWith("Patching iguazio tenant app service sets", "body", string(body))
	if err != nil {
		return errors.Wrap(err, "Could not marshal json patch mapper")
	}

	absPath := []string{"apis", "iguazio.com", "v1beta1", "namespaces", namespace, "iguaziotenantappservicesets", namespace}
	_, err = s.kubeClientSet.Discovery().RESTClient().Patch(types.JSONPatchType).Body(body).AbsPath(absPath...).Do().Raw()
	if err != nil {
		return errors.Wrap(err, "Failed to patch iguazio tenant app service sets")
	}
	return nil
}

func (s *AppResourceScaler) waitForServiceReadiness(namespace string, serviceName string) error {
	s.logger.DebugWith("Waiting for service readiness", "serviceName", serviceName)
	for {
		resourcesList, err := s.GetResources()
		if err != nil {
			return errors.Wrap(err, "Failed to get ready services")
		}
		for _, resource := range resourcesList {
			if string(resource) == serviceName {
				s.logger.DebugWith("Service ready", "serviceName", serviceName)
				return nil
			}
		}
		time.Sleep(5 * time.Second)
	}
	return nil
}

func (s *AppResourceScaler) GetResources() ([]scaler_types.Resource, error) {
	var iguazioTenantAppServicesSetMap map[string]interface{}
	resources := make([]scaler_types.Resource, 0)

	absPath := []string{"apis", "iguazio.com", "v1beta1", "namespaces", s.namespace, "iguaziotenantappservicesets", s.namespace}
	iguazioTenantAppServicesSet, err := s.kubeClientSet.Discovery().RESTClient().Get().AbsPath(absPath...).Do().Raw()

	if err != nil {
		return nil, errors.Wrap(err, "Failed to get iguazio tenant app service sets")
	}

	if err := json.Unmarshal(iguazioTenantAppServicesSet, &iguazioTenantAppServicesSetMap); err != nil {
		return nil, errors.Wrap(err, "Failed to unmarshal response")
	}

	status, ok := iguazioTenantAppServicesSetMap["status"].(map[string]interface{})
	if !ok {
		s.logger.WarnWith("Service set does not have status", "serviceSet", iguazioTenantAppServicesSetMap)
		return resources, nil
	}

	servicesMap, ok := status["services"].(map[string]interface{})
	if !ok {
		s.logger.WarnWith("Status does not have services", "status", status)
		return resources, nil
	}

	for serviceName, serviceStatus := range servicesMap {
		serviceStatusMap, ok := serviceStatus.(map[string]interface{})
		if !ok {
			s.logger.WarnWith("Service status type assertion failed, continuing", "serviceStatus", serviceStatus)
			continue
		}

		stateString, ok := serviceStatusMap["state"].(string)
		if !ok {
			s.logger.WarnWith("Service status does not have state, continuing", "serviceStatusMap", serviceStatusMap)
			continue
		}

		if stateString == "ready" {
			resources = append(resources, scaler_types.Resource(serviceName))
		}
	}

	s.logger.DebugWith("Found services", "services", resources)

	return resources, nil
}

func (s *AppResourceScaler) GetConfig() (*scaler_types.ResourceScalerConfig, error) {
	return nil, nil
}

func getClientConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}

	return rest.InClusterConfig()
}
