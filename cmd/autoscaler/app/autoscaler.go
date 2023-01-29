/*
Copyright 2017 The Nuclio Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package app

import (
	"os"
	"time"

	"github.com/v3io/app-resource-scaler/pkg/common"
	"github.com/v3io/app-resource-scaler/pkg/resourcescaler"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	nucliozap "github.com/nuclio/zap"
	"github.com/v3io/scaler/pkg/autoscaler"
	"github.com/v3io/scaler/pkg/scalertypes"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/metrics/pkg/client/custom_metrics"
)

func Run(kubeconfigPath string,
	namespace string,
	scaleInterval time.Duration,
	metricsResourceKind string,
	metricsResourceGroup string) error {

	// create root logger
	rootLogger, err := nucliozap.NewNuclioZap("autoscaler",
		"console",
		nil,
		os.Stdout,
		os.Stderr,
		nucliozap.DebugLevel)
	if err != nil {
		return errors.Wrap(err, "Failed creating a new logger")
	}

	// create autoscaler
	autoScaler, err := createAutoScaler(
		rootLogger,
		namespace,
		kubeconfigPath,
		scaleInterval,
		metricsResourceKind,
		metricsResourceGroup)
	if err != nil {
		return errors.Wrap(err, "Failed to create autoscaler")
	}

	// start autoscaler and run forever
	if err := autoScaler.Start(); err != nil {
		return errors.Wrap(err, "Failed to start autoscaler")
	}
	select {}
}

func createAutoScaler(logger logger.Logger,
	namespace string,
	kubeconfigPath string,
	scaleInterval time.Duration,
	metricsResourceKind string,
	metricsResourceGroup string) (*autoscaler.Autoscaler, error) {

	// create k8s rest config
	customMetricsClient, err := newMetricsCustomClient(kubeconfigPath)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create new metric custom client")
	}

	// create k8s client
	kubeconfig, err := common.GetClientConfig(kubeconfigPath)
	if err != nil {
		return nil, errors.Wrap(err, "Failed parsing cluster's kubeconfig from path")
	}

	// create k8s clientset
	kubeClientSet, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return nil, errors.Wrap(err, "Failed creating kubeclient from kubeconfig")
	}

	// create resource scaler
	resourceScaler, err := resourcescaler.New(logger,
		kubeClientSet,
		namespace,
		scalertypes.DLXOptions{},
		scalertypes.AutoScalerOptions{
			Namespace:     namespace,
			ScaleInterval: scalertypes.Duration{Duration: scaleInterval},
			GroupKind: schema.GroupKind{
				Kind:  metricsResourceKind,
				Group: metricsResourceGroup,
			},
		})
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create resource scaler")
	}

	// get resource scaler configuration
	resourceScalerConfig, err := resourceScaler.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get resource scaler config")
	}

	// create autoscaler
	autoScaler, err := autoscaler.NewAutoScaler(logger,
		resourceScaler,
		customMetricsClient,
		resourceScalerConfig.AutoScalerOptions)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create autoscaler")
	}

	rest.SetDefaultWarningHandler(common.NewKubernetesClientWarningHandler(logger.GetChild("kube_warnings")))

	return autoScaler, nil
}

func newMetricsCustomClient(kubeconfigPath string) (custom_metrics.CustomMetricsClient, error) {
	restConfig, err := common.GetClientConfig(kubeconfigPath)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get rest config")
	}

	// create metric client and
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create discovery client")
	}
	availableAPIsGetter := custom_metrics.NewAvailableAPIsGetter(discoveryClient)
	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient))
	return custom_metrics.NewForConfig(restConfig, restMapper, availableAPIsGetter), nil
}
