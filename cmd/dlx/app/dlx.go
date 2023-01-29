/*
Copyright 2019 Iguazio Systems Ltd.

Licensed under the Apache License, Version 2.0 (the "License") with
an addition restriction as set forth herein. You may not use this
file except in compliance with the License. You may obtain a copy of
the License at http://www.apache.org/licenses/LICENSE-2.0.

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
implied. See the License for the specific language governing
permissions and limitations under the License.

In addition, you may not use the software for any purposes that are
illegal under applicable law, and the grant of the foregoing license
under the Apache 2.0 license is conditioned upon your compliance with
such restriction.
*/

package app

import (
	"os"
	"time"

	"github.com/v3io/app-resource-scaler/pkg/common"
	"github.com/v3io/app-resource-scaler/pkg/resourcescaler"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	"github.com/nuclio/zap"
	"github.com/v3io/scaler/pkg/dlx"
	"github.com/v3io/scaler/pkg/scalertypes"
	"k8s.io/client-go/kubernetes"
)

func Run(kubeconfigPath string,
	namespace string,
	targetNameHeader string,
	targetPathHeader string,
	targetPort int,
	listenAddress string,
	resourceReadinessTimeout string,
	multiTargetStrategy string) error {

	// create root logger
	rootLogger, err := nucliozap.NewNuclioZap("dlx",
		"console",
		nil,
		os.Stdout,
		os.Stderr,
		nucliozap.DebugLevel)
	if err != nil {
		return errors.Wrap(err, "Failed creating a new logger")
	}

	resourceReadinessTimeoutDuration, err := time.ParseDuration(resourceReadinessTimeout)
	if err != nil {
		return errors.Wrap(err, "Failed to parse resource readiness timeout")
	}

	dlxOptions := scalertypes.DLXOptions{
		TargetNameHeader:         targetNameHeader,
		TargetPathHeader:         targetPathHeader,
		TargetPort:               targetPort,
		ListenAddress:            listenAddress,
		Namespace:                namespace,
		ResourceReadinessTimeout: scalertypes.Duration{Duration: resourceReadinessTimeoutDuration},
		MultiTargetStrategy:      scalertypes.MultiTargetStrategy(multiTargetStrategy),
	}

	// create k8s client
	kubeconfig, err := common.GetClientConfig(kubeconfigPath)
	if err != nil {
		return errors.Wrap(err, "Failed parsing cluster's kubeconfig from path")
	}

	// create k8s clientset
	kubeClientSet, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return errors.Wrap(err, "Failed creating kubeclient from kubeconfig")
	}

	// create resource scaler
	resourceScaler, err := resourcescaler.New(rootLogger,
		kubeClientSet,
		namespace,
		dlxOptions,
		scalertypes.AutoScalerOptions{})
	if err != nil {
		return errors.Wrap(err, "Failed to create resource scaler")
	}

	// see if resource scaler wants to override the arguments
	resourceScalerConfig, err := resourceScaler.GetConfig()
	if err != nil {
		return errors.Wrap(err, "Failed to get resource scaler config")
	}

	if resourceScalerConfig != nil {
		dlxOptions = resourceScalerConfig.DLXOptions
	}

	newDLX, err := createDLX(rootLogger, resourceScaler, dlxOptions)
	if err != nil {
		return errors.Wrap(err, "Failed to create dlx")
	}

	// start the scaler
	if err := newDLX.Start(); err != nil {
		return errors.Wrap(err, "Failed to start dlx")
	}

	select {}
}

func createDLX(loggerInstance logger.Logger,
	resourceScaler scalertypes.ResourceScaler,
	options scalertypes.DLXOptions) (*dlx.DLX, error) {

	newScaler, err := dlx.NewDLX(loggerInstance, resourceScaler, options)

	if err != nil {
		return nil, err
	}

	return newScaler, nil
}
