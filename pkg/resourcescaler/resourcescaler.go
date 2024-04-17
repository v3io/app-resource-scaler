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

package resourcescaler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	"github.com/v3io/scaler/pkg/scalertypes"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

type ProvisioningState string

const (
	scaleFromZeroProvisioningState ProvisioningState = "waitingForScalingFromZero"
	scaleToZeroProvisioningState   ProvisioningState = "waitingForScalingToZero"
)

type AppResourceScaler struct {
	logger        logger.Logger
	namespace     string
	kubeClientSet kubernetes.Interface

	autoScalerOptions scalertypes.AutoScalerOptions
	dlxOptions        scalertypes.DLXOptions
}

func New(logger logger.Logger,
	kubeClientSet kubernetes.Interface,
	namespace string,
	dlxOptions scalertypes.DLXOptions,
	autoScalerOptions scalertypes.AutoScalerOptions) (scalertypes.ResourceScaler, error) { // nolint: deadcode

	return &AppResourceScaler{
		logger:            logger.GetChild("resourcescaler"),
		namespace:         namespace,
		kubeClientSet:     kubeClientSet,
		autoScalerOptions: autoScalerOptions,
		dlxOptions:        dlxOptions,
	}, nil
}

// SetScale scales a service
// Deprecated: use SetScaleCtx instead
func (s *AppResourceScaler) SetScale(resources []scalertypes.Resource, scale int) error {
	setScaleContext, cancelFunc := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancelFunc()

	return s.SetScaleCtx(setScaleContext, resources, scale)
}

// SetScaleCtx scales a service
func (s *AppResourceScaler) SetScaleCtx(ctx context.Context, resources []scalertypes.Resource, scale int) error {
	serviceNames := make([]string, 0)
	for _, resource := range resources {
		serviceNames = append(serviceNames, resource.Name)
	}
	if scale == 0 {
		return s.scaleServicesToZero(ctx, s.namespace, serviceNames)
	}
	return s.scaleServicesFromZero(ctx, s.namespace, serviceNames)
}

func (s *AppResourceScaler) GetResources() ([]scalertypes.Resource, error) {
	resources := make([]scalertypes.Resource, 0)

	specServicesMap, statusServicesMap, _, err := s.getIguazioTenantAppServiceSets(context.Background())
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get iguazio tenant app service sets")
	}

	for statusServiceName, serviceStatus := range statusServicesMap {

		// Nuclio is a special service since it's a controller itself, so its scale to zero spec is configuring
		// how and when it should scale its resources, and not how and when we should scale him
		if statusServiceName == "nuclio" {
			continue
		}

		stateString, err := s.parseServiceState(serviceStatus)
		if err != nil {
			s.logger.WarnWith("Failed parsing the service state, continuing",
				"err", errors.GetErrorStackString(err, 10),
				"serviceStatus", serviceStatus)
			continue
		}

		_, serviceSpecExists := specServicesMap[statusServiceName]

		if stateString == "ready" && serviceSpecExists {

			scaleResources, err := s.parseScaleResources(specServicesMap[statusServiceName])
			if err != nil {
				s.logger.WarnWith("Failed parsing the scale resources, continuing",
					"err", errors.GetErrorStackString(err, 10),
					"serviceSpec", specServicesMap[statusServiceName])
				continue
			}

			if len(scaleResources) != 0 {

				lastScaleEvent, lastScaleEventTime, err := s.parseLastScaleEvent(serviceStatus)
				if err != nil {
					return nil, errors.Wrap(err, "Failed to parse last scale event")
				}

				resources = append(resources, scalertypes.Resource{
					Name:               statusServiceName,
					ScaleResources:     scaleResources,
					LastScaleEvent:     lastScaleEvent,
					LastScaleEventTime: lastScaleEventTime,
				})
			}
		}
	}

	if len(resources) != 0 {
		s.logger.DebugWith("Found services", "services", resources)
	}

	return resources, nil
}

func (s *AppResourceScaler) GetConfig() (*scalertypes.ResourceScalerConfig, error) {
	return &scalertypes.ResourceScalerConfig{
		AutoScalerOptions: s.autoScalerOptions,
		DLXOptions:        s.dlxOptions,
	}, nil
}

func (s *AppResourceScaler) ResolveServiceName(resource scalertypes.Resource) (string, error) {
	return resource.Name, nil
}

func (s *AppResourceScaler) scaleServicesFromZero(ctx context.Context, namespace string, serviceNames []string) error {
	var jsonPatchMapper []map[string]interface{}
	s.logger.DebugWithCtx(ctx, "Scaling from zero", "namespace", namespace, "serviceNames", serviceNames)
	marshaledTime, err := time.Now().MarshalText()
	if err != nil {
		return errors.Wrap(err, "Failed to marshal time")
	}
	for _, serviceName := range serviceNames {
		jsonPatchMapper, err = s.appendServiceStateChangeJSONPatchOperations(jsonPatchMapper,
			serviceName,
			"ready",
			scalertypes.ScaleFromZeroStartedScaleEvent,
			marshaledTime)
		if err != nil {
			return errors.Wrap(err, "Failed appending service state change json patch operations")
		}
	}

	if err := s.patchIguazioTenantAppServiceSets(ctx,
		namespace,
		jsonPatchMapper,
		scaleFromZeroProvisioningState); err != nil {
		return errors.Wrap(err, "Failed to patch iguazio tenant app service sets")
	}

	if err := s.waitForServicesState(ctx, serviceNames, "ready"); err != nil {
		return errors.Wrap(err, "Failed to wait for services readiness")
	}

	return nil
}

func (s *AppResourceScaler) scaleServicesToZero(ctx context.Context, namespace string, serviceNames []string) error {
	var jsonPatchMapper []map[string]interface{}
	s.logger.DebugWithCtx(ctx, "Scaling to zero", "namespace", namespace, "serviceNames", serviceNames)
	marshaledTime, err := time.Now().MarshalText()
	if err != nil {
		return errors.Wrap(err, "Failed to marshal time")
	}
	for _, serviceName := range serviceNames {

		jsonPatchMapper, err = s.appendServiceStateChangeJSONPatchOperations(jsonPatchMapper,
			serviceName,
			"scaledToZero",
			scalertypes.ScaleToZeroStartedScaleEvent,
			marshaledTime)
		if err != nil {
			return errors.Wrap(err, "Failed appending service state change json patch operations")
		}
	}

	if err := s.patchIguazioTenantAppServiceSets(ctx,
		namespace,
		jsonPatchMapper,
		scaleToZeroProvisioningState); err != nil {
		return errors.Wrap(err, "Failed to patch iguazio tenant app service sets")
	}

	if err := s.waitForServicesState(ctx, serviceNames, "scaledToZero"); err != nil {
		return errors.Wrap(err, "Failed to wait for services to scale to zero")
	}

	return nil
}

func (s *AppResourceScaler) appendServiceStateChangeJSONPatchOperations(jsonPatchMapper []map[string]interface{},
	serviceName string,
	desiredState string,
	scaleEvent scalertypes.ScaleEvent,
	marshaledTime []byte) ([]map[string]interface{}, error) {

	desiredStatePath := fmt.Sprintf("/spec/spec/tenants/0/spec/services/%s/desired_state", serviceName)
	markForRestartPath := fmt.Sprintf("/spec/spec/tenants/0/spec/services/%s/mark_for_restart", serviceName)

	// Added To signal Provazio controller to apply the changes
	markAsChangedPath := fmt.Sprintf("/spec/spec/tenants/0/spec/services/%s/mark_as_changed", serviceName)
	scaleToZeroStatusPath := fmt.Sprintf("/status/services/%s/scale_to_zero", serviceName)
	lastScaleStatePath := fmt.Sprintf("/status/services/%s/scale_to_zero/last_scale_event", serviceName)
	lastScaleStateTimePath := fmt.Sprintf("/status/services/%s/scale_to_zero/last_scale_event_time", serviceName)
	jsonPatchMapper = append(jsonPatchMapper, map[string]interface{}{
		"op":    "add",
		"path":  desiredStatePath,
		"value": desiredState,
	})
	jsonPatchMapper = append(jsonPatchMapper, map[string]interface{}{
		"op":    "add",
		"path":  markForRestartPath,
		"value": false,
	})
	jsonPatchMapper = append(jsonPatchMapper, map[string]interface{}{
		"op":    "add",
		"path":  markAsChangedPath,
		"value": true,
	})
	jsonPatchMapper = append(jsonPatchMapper, map[string]interface{}{
		"op":    "add",
		"path":  scaleToZeroStatusPath,
		"value": map[string]interface{}{},
	})
	jsonPatchMapper = append(jsonPatchMapper, map[string]interface{}{
		"op":    "add",
		"path":  lastScaleStatePath,
		"value": string(scaleEvent),
	})
	jsonPatchMapper = append(jsonPatchMapper, map[string]interface{}{
		"op":    "add",
		"path":  lastScaleStateTimePath,
		"value": string(marshaledTime),
	})

	return jsonPatchMapper, nil
}

func (s *AppResourceScaler) patchIguazioTenantAppServiceSets(ctx context.Context,
	namespace string,
	jsonPatchMapper []map[string]interface{},
	provisioningState ProvisioningState) error {
	jsonPatchMapper = append(jsonPatchMapper, map[string]interface{}{
		"op":    "add",
		"path":  "/status/state",
		"value": string(provisioningState),
	})
	jsonPatchMapper = append(jsonPatchMapper, map[string]interface{}{
		"op":    "add",
		"path":  "/spec/spec/tenants/0/spec/force_apply_all_mode",
		"value": "disabled",
	})
	if err := s.waitForNoProvisioningInProcess(ctx); err != nil {
		return errors.Wrap(err, "Failed waiting for IguazioTenantAppServiceSet to finish provisioning")
	}

	body, err := json.Marshal(jsonPatchMapper)
	if err != nil {
		return errors.Wrap(err, "Could not marshal json patch mapper")
	}

	s.logger.DebugWithCtx(ctx, "Patching iguazio tenant app service sets", "body", string(body))
	absPath := []string{"apis", "iguazio.com", "v1beta1", "namespaces", namespace, "iguaziotenantappservicesets", namespace}
	if _, err := s.kubeClientSet.
		Discovery().
		RESTClient().
		Patch(types.JSONPatchType).
		Body(body).
		AbsPath(absPath...).
		Do(ctx).
		Raw(); err != nil {
		return errors.Wrap(err, "Failed to patch iguazio tenant app service sets")
	}
	return nil
}

func (s *AppResourceScaler) waitForNoProvisioningInProcess(ctx context.Context) error {
	s.logger.DebugWithCtx(ctx, "Waiting for IguazioTenantAppServiceSet to finish provisioning")

	finiteStateDiscoveryTimeout := 30 * time.Second

	var finiteStateDiscoveryTime *time.Time
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-time.After(5 * time.Second):
			s.logger.DebugWithCtx(ctx, "Checking the state of the Iguazio tenant app service sets")
			_, _, state, err := s.getIguazioTenantAppServiceSets(ctx)
			if err != nil {
				s.logger.WarnWithCtx(ctx, "Failed to get iguazio tenant app service sets",
					"err", err.Error())
				continue
			}

			if state == "ready" || state == "error" {
				if finiteStateDiscoveryTime == nil {
					now := time.Now()
					finiteStateDiscoveryTime = &now
					s.logger.DebugWithCtx(ctx,
						"IguazioTenantAppServiceSet finished provisioning", "state", state)
				}
			} else {

				// reset the time if the state is not stable
				if finiteStateDiscoveryTime != nil {
					s.logger.DebugWithCtx(ctx, "IguazioTenantAppServiceSet is provisioning again, "+
						"resetting discovery time",
						"state", state,
						"finiteStateDiscoveryTime", time.Since(*finiteStateDiscoveryTime).String())
				}
				finiteStateDiscoveryTime = nil
			}

			if finiteStateDiscoveryTime != nil {

				// it has been finite for finiteStateDiscoveryTimeout amount of time
				// we can safely assume that other services waiting for app services to be ready
				// captured the state change. now it would be our turn
				timeSince := time.Since(*finiteStateDiscoveryTime)
				if timeSince >= finiteStateDiscoveryTimeout {
					s.logger.DebugWithCtx(ctx, "IguazioTenantAppServiceSet state is stabled")
					return nil

				}
				s.logger.DebugWithCtx(ctx, "IguazioTenantAppServiceSet waiting for the state to be stable",
					"state", state,
					"timeSince", timeSince.String())
			} else {
				s.logger.DebugWithCtx(ctx, "IguazioTenantAppServiceSet is still provisioning",
					"state", state)
			}
		}

	}
}

func (s *AppResourceScaler) waitForServicesState(ctx context.Context, serviceNames []string, desiredState string) error {
	s.logger.DebugWithCtx(ctx,
		"Waiting for services to reach desired state",
		"serviceNames", serviceNames,
		"desiredState", desiredState)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			servicesToCheck := append([]string(nil), serviceNames...)
			_, statusServicesMap, _, err := s.getIguazioTenantAppServiceSets(ctx)
			if err != nil {
				return errors.Wrap(err, "Failed to get iguazio tenant app service sets")
			}

			for serviceName, serviceStatus := range statusServicesMap {
				if !stringSliceContainsString(servicesToCheck, serviceName) {
					continue
				}

				currentState, err := s.parseServiceState(serviceStatus)
				if err != nil {
					return errors.Wrap(err, "Failed parsing the service state")
				}

				if currentState != desiredState {
					s.logger.DebugWithCtx(ctx,
						"Service did not reach desired state yet",
						"serviceName", serviceName,
						"currentState", currentState,
						"desiredState", desiredState)
					break
				}

				s.logger.DebugWithCtx(ctx,
					"Service reached desired state",
					"serviceName", serviceName,
					"desiredState", desiredState)
				servicesToCheck = removeStringFromSlice(serviceName, servicesToCheck)

				if len(servicesToCheck) == 0 {
					return nil
				}
			}
		}
	}
}

func (s *AppResourceScaler) getIguazioTenantAppServiceSets(ctx context.Context) (
	map[string]interface{}, map[string]interface{}, string, error) {
	var iguazioTenantAppServicesSetMap map[string]interface{}

	absPath := []string{"apis", "iguazio.com", "v1beta1", "namespaces", s.namespace, "iguaziotenantappservicesets", s.namespace}
	iguazioTenantAppServicesSet, err := s.kubeClientSet.
		Discovery().
		RESTClient().
		Get().
		AbsPath(absPath...).
		Do(ctx).
		Raw()

	if err != nil {
		return nil, nil, "", errors.Wrap(err, "Failed to get iguazio tenant app service sets")
	}

	if err := json.Unmarshal(iguazioTenantAppServicesSet, &iguazioTenantAppServicesSetMap); err != nil {
		return nil, nil, "", errors.Wrap(err, "Failed to unmarshal response")
	}

	statusServicesMap, state, err := s.parseStatus(iguazioTenantAppServicesSetMap)
	if err != nil {
		return nil, nil, "", errors.Wrap(err, "Failed to parse iguazio tenant app service sets status")
	}
	specServicesMap := s.parseSpecServices(iguazioTenantAppServicesSetMap)

	return specServicesMap, statusServicesMap, state, nil
}

func (s *AppResourceScaler) parseSpecServices(iguazioTenantAppServicesSetMap map[string]interface{}) map[string]interface{} {
	var servicesMap map[string]interface{}

	spec, ok := iguazioTenantAppServicesSetMap["spec"].(map[string]interface{})
	if !ok {
		s.logger.WarnWith("Service set does not have spec", "serviceSet", iguazioTenantAppServicesSetMap)
		return servicesMap
	}

	internalSpec, ok := spec["spec"].(map[string]interface{})
	if !ok {
		s.logger.WarnWith("Spec does not have internal spec", "spec", spec)
		return servicesMap
	}

	tenants, ok := internalSpec["tenants"].([]interface{})
	if !ok || len(tenants) != 1 {
		s.logger.WarnWith("Internal spec does not have tenants or its length is invalid", "internalSpec", internalSpec)
		return servicesMap
	}

	tenant, ok := tenants[0].(map[string]interface{})
	if !ok {
		s.logger.WarnWith("Tenant is not an object", "tenants", tenants)
		return servicesMap
	}

	tenantSpec, ok := tenant["spec"].(map[string]interface{})
	if !ok {
		s.logger.WarnWith("Tenant does not have spec", "tenant", tenant)
		return servicesMap
	}

	servicesMap, ok = tenantSpec["services"].(map[string]interface{})
	if !ok {
		s.logger.WarnWith("Tenant spec does not have services", "tenantSpec", tenantSpec)
		return servicesMap
	}

	return servicesMap
}

func (s *AppResourceScaler) parseStatus(iguazioTenantAppServicesSetMap map[string]interface{}) (map[string]interface{}, string, error) {
	var servicesMap map[string]interface{}
	status, ok := iguazioTenantAppServicesSetMap["status"].(map[string]interface{})
	if !ok {
		return nil, "", errors.New("Service set does not have status")
	}

	state, ok := status["state"].(string)
	if !ok {
		return nil, "", errors.New("Status does not have state")
	}

	servicesMap, ok = status["services"].(map[string]interface{})
	if !ok {
		s.logger.WarnWith("Status does not have services", "status", status)
		return servicesMap, state, nil
	}

	return servicesMap, state, nil
}

func (s *AppResourceScaler) parseScaleToZeroStatus(scaleToZeroStatus map[string]interface{}) (scalertypes.ScaleEvent, time.Time, error) {
	lastScaleEventString, ok := scaleToZeroStatus["last_scale_event"].(string)
	if !ok {
		return "", time.Now(), errors.New("Scale to zero status does not have last scale event")
	}

	lastScaleEvent, err := scalertypes.ParseScaleEvent(lastScaleEventString)
	if err != nil {
		return "", time.Now(), errors.Wrap(err, "Failed to parse scale event")
	}

	lastScaleEventTimeString, ok := scaleToZeroStatus["last_scale_event_time"].(string)
	if !ok {
		return "", time.Now(), errors.New("Scale to zero status does not have last scale event time")
	}

	lastScaleEventTime, err := time.Parse(time.RFC3339, lastScaleEventTimeString)
	if err != nil {
		return "", time.Now(), errors.Wrap(err, "Failed to parse last scale event time")
	}

	return lastScaleEvent, lastScaleEventTime, nil
}

func (s *AppResourceScaler) parseLastScaleEvent(serviceStatus interface{}) (*scalertypes.ScaleEvent, *time.Time, error) {
	serviceStatusMap, ok := serviceStatus.(map[string]interface{})
	if !ok {
		return nil, nil, errors.New("Service status type assertion failed")
	}

	scaleToZeroStatus, ok := serviceStatusMap["scale_to_zero"].(map[string]interface{})
	if !ok {
		return nil, nil, nil
	}

	lastScaleEvent, lastScaleEventTime, err := s.parseScaleToZeroStatus(scaleToZeroStatus)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Failed parsing scale to zero status")
	}

	return &lastScaleEvent, &lastScaleEventTime, nil
}

func (s *AppResourceScaler) parseServiceState(serviceStatus interface{}) (string, error) {
	serviceStatusMap, ok := serviceStatus.(map[string]interface{})
	if !ok {
		return "", errors.New("Service status type assertion failed")
	}

	stateString, ok := serviceStatusMap["state"].(string)
	if !ok {
		return "", errors.New("Service status does not have state")
	}

	return stateString, nil
}

func (s *AppResourceScaler) parseScaleResources(serviceSpecInterface interface{}) ([]scalertypes.ScaleResource, error) {
	var parsedScaleResources []scalertypes.ScaleResource
	serviceSpec, ok := serviceSpecInterface.(map[string]interface{})
	if !ok {
		return nil, errors.New("Service spec type assertion failed")
	}

	scaleToZeroSpec, ok := serviceSpec["scale_to_zero"].(map[string]interface{})
	if !ok {

		// It's ok for a service to not have the scale_to_zero spec
		return nil, nil
	}

	scaleToZeroMode, ok := scaleToZeroSpec["mode"].(string)
	if !ok {
		return nil, errors.New("Scale to zero spec does not have mode")
	}

	// if it's not enabled there's no reason to parse the rest
	if scaleToZeroMode != "enabled" {
		return nil, nil
	}

	scaleResourcesList, ok := scaleToZeroSpec["scale_resources"].([]interface{})
	if !ok {
		return nil, errors.New("Scale to zero spec does not have scale resources")
	}

	for _, scaleResourceInterface := range scaleResourcesList {
		scaleResource, ok := scaleResourceInterface.(map[string]interface{})
		if !ok {
			return nil, errors.New("Scale resource type assertion failed")
		}

		metricName, ok := scaleResource["metric_name"].(string)
		if !ok {
			return nil, errors.New("Scale resource does not have metric name")
		}

		threshold, ok := scaleResource["threshold"].(float64)
		if !ok {
			return nil, errors.New("Scale resource does not have threshold")
		}

		windowSizeString, ok := scaleResource["window_size"].(string)
		if !ok {
			return nil, errors.New("Scale resource does not have metric window size")
		}

		windowSize, err := time.ParseDuration(windowSizeString)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to parse window size")
		}

		parsedScaleResource := scalertypes.ScaleResource{
			MetricName: metricName,
			WindowSize: scalertypes.Duration{Duration: windowSize},
			Threshold:  int(threshold),
		}

		parsedScaleResources = append(parsedScaleResources, parsedScaleResource)
	}

	return parsedScaleResources, nil
}

func removeStringFromSlice(someString string, slice []string) []string {
	var newSlice []string
	for _, item := range slice {
		if item != someString {
			newSlice = append(newSlice, item)
		}
	}
	return newSlice
}

func stringSliceContainsString(slice []string, str string) bool {
	for _, stringInSlice := range slice {
		if stringInSlice == str {
			return true
		}
	}
	return false
}
