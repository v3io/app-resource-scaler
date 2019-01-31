package main

import (
	"fmt"
	"os"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	"github.com/v3io/scaler-types"

	"github.com/nuclio/zap"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (s *AppResourceScaler) SetScale(resource scaler_types.Resource, scaling int) error {

	// get deployment by resource name
	deployment, err := s.kubeClientSet.AppsV1beta1().Deployments(s.namespace).Get(string(resource), meta_v1.GetOptions{})
	if err != nil {
		s.logger.WarnWith("Failure during retrieval of deployment", "resource_name", string(resource))
		return errors.Wrap(err, "Failed getting deployment instance")
	}

	// get ingress by resource name
	ingress, err := s.kubeClientSet.ExtensionsV1beta1().Ingresses(s.namespace).Get(string(resource), meta_v1.GetOptions{})
	if err != nil {
		s.logger.WarnWith("Failure during retrieval of ingress", "resource_name", string(resource))
		return errors.Wrap(err, "Failed getting ingress instance")
	}

	// get service by resource name
	service, err := s.kubeClientSet.CoreV1().Services(s.namespace).Get(string(resource), meta_v1.GetOptions{})
	if err != nil {
		s.logger.WarnWith("Failure during retrieval of service", "resource_name", string(resource))
		return errors.Wrap(err, "Failed getting service instance")
	}

	ingress.GetObjectMeta().SetAnnotations(map[string]string{
		"nginx.ingress.kubernetes.io/configuration-snippet": fmt.Sprintf(
			`proxy_set_header X-App-Target "%s";`, string(resource)),
	})
	_, err = s.kubeClientSet.ExtensionsV1beta1().Ingresses(s.namespace).Update(ingress)
	if err != nil {
		s.logger.WarnWith("Failure during update of ingress with annotation",
			"resource_name", string(resource))
		return errors.Wrap(err, "Failed updating ingress instance")
	}

	if scaling == 0 {
		// update service selector to refer to dlx
		s.logger.InfoWith("Changing service's selector to work with dlx", "service_name", string(resource))

		service.Spec.Selector = map[string]string{"app": "scaler-dlx"}
		_, err := s.kubeClientSet.CoreV1().Services(s.namespace).Update(service)
		if err != nil {
			s.logger.WarnWith("Failure during update of service with selector",
				"resource_name", string(resource))
			return errors.Wrap(err, "Failed updating service instance")
		}
	} else {
		// update service selector to refer to resource
		s.logger.InfoWith("Changing service's selector back to work with resource", "service_name", string(resource))

		service.Spec.Selector = service.GetLabels()
		_, err := s.kubeClientSet.CoreV1().Services(s.namespace).Update(service)
		if err != nil {
			s.logger.WarnWith("Failure during update of service with selector",
				"resource_name", string(resource))
			return errors.Wrap(err, "Failed updating service instance")
		}
	}

	// set deployment num of replicas by scaling factor (0/1)
	int32scaling := int32(scaling)
	deployment.Spec.Replicas = &int32scaling
	_, err = s.kubeClientSet.AppsV1beta1().Deployments(s.namespace).Update(deployment)
	if err != nil {
		s.logger.WarnWith("Failure during update of deployment", "resource_name", string(resource))
		return errors.Wrap(err, "Failed updating deployment instance")
	}

	return nil
}

func (s *AppResourceScaler) GetResources() ([]scaler_types.Resource, error) {
	resources := make([]scaler_types.Resource, 0)

	deploymentsList, err := s.kubeClientSet.AppsV1beta1().Deployments(s.namespace).List(meta_v1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get listed releases")
	}

	// return the names of all deployments
	for _, deployment := range deploymentsList.Items {
		resources = append(resources, scaler_types.Resource(deployment.Name))
	}

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
