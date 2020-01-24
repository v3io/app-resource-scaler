package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	"github.com/nuclio/zap"
	"github.com/v3io/scaler-types"
	"k8s.io/apimachinery/pkg/types"
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

func (s *AppResourceScaler) SetScale(resources []scaler_types.Resource, scale int) error {
	serviceNames := make([]string, 0)
	for _, resource := range resources {
		serviceNames = append(serviceNames, resource.Name)
	}
	if scale == 0 {
		return s.scaleServicesToZero(s.namespace, serviceNames)
	}
	return s.scaleServicesFromZero(s.namespace, serviceNames)
}

func (s *AppResourceScaler) GetResources() ([]scaler_types.Resource, error) {
	resources := make([]scaler_types.Resource, 0)

	specServicesMap, statusServicesMap, _, err := s.getIguazioTenantAppServiceSets()
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

				resources = append(resources, scaler_types.Resource{
					Name:               statusServiceName,
					ScaleResources:     scaleResources,
					LastScaleEvent:     lastScaleEvent,
					LastScaleEventTime: lastScaleEventTime,
				})
			}
		}
	}

	s.logger.DebugWith("Found services", "services", resources)

	return resources, nil
}

func (s *AppResourceScaler) GetConfig() (*scaler_types.ResourceScalerConfig, error) {
	return nil, nil
}

func (s *AppResourceScaler) scaleServicesFromZero(namespace string, serviceNames []string) error {
	var jsonPatchMapper []map[string]interface{}
	s.logger.DebugWith("Scaling from zero", "namespace", namespace, "serviceNames", serviceNames)
	marshaledTime, err := time.Now().MarshalText()
	if err != nil {
		return errors.Wrap(err, "Failed to marshal time")
	}
	for _, serviceName := range serviceNames {
		jsonPatchMapper, err = s.appendServiceStateChangeJsonPatchOperations(jsonPatchMapper,
			serviceName,
			"ready",
			scaler_types.ScaleFromZeroStartedScaleEvent,
			marshaledTime)
		if err != nil {
			return errors.Wrap(err, "Failed appending service state change json patch operations")
		}
	}

	err = s.patchIguazioTenantAppServiceSets(namespace, jsonPatchMapper)

	if err != nil {
		return errors.Wrap(err, "Failed to patch iguazio tenant app service sets")
	}

	err = s.waitForServicesState(namespace, serviceNames, "ready")

	if err != nil {
		return errors.Wrap(err, "Failed to wait for services readiness")
	}

	return nil
}

func (s *AppResourceScaler) scaleServicesToZero(namespace string, serviceNames []string) error {
	var jsonPatchMapper []map[string]interface{}
	s.logger.DebugWith("Scaling to zero", "namespace", namespace, "serviceNames", serviceNames)
	marshaledTime, err := time.Now().MarshalText()
	if err != nil {
		return errors.Wrap(err, "Failed to marshal time")
	}
	for _, serviceName := range serviceNames {

		jsonPatchMapper, err = s.appendServiceStateChangeJsonPatchOperations(jsonPatchMapper,
			serviceName,
			"scaledToZero",
			scaler_types.ScaleToZeroStartedScaleEvent,
			marshaledTime)
		if err != nil {
			return errors.Wrap(err, "Failed appending service state change json patch operations")
		}
	}

	err = s.patchIguazioTenantAppServiceSets(namespace, jsonPatchMapper)

	if err != nil {
		return errors.Wrap(err, "Failed to patch iguazio tenant app service sets")
	}

	err = s.waitForServicesState(namespace, serviceNames, "scaledToZero")

	if err != nil {
		return errors.Wrap(err, "Failed to wait for services to scale to zero")
	}

	return nil
}

func (s *AppResourceScaler) appendServiceStateChangeJsonPatchOperations(jsonPatchMapper []map[string]interface{}, serviceName string, desiredState string, scaleEvent scaler_types.ScaleEvent, marshaledTime []byte) ([]map[string]interface{}, error) {
	desiredStatePath := fmt.Sprintf("/spec/spec/tenants/0/spec/services/%s/desired_state", string(serviceName))
	markForRestartPath := fmt.Sprintf("/spec/spec/tenants/0/spec/services/%s/mark_for_restart", string(serviceName))
	scaleToZeroStatusPath := fmt.Sprintf("/status/services/%s/scale_to_zero", string(serviceName))
	lastScaleStatePath := fmt.Sprintf("/status/services/%s/scale_to_zero/last_scale_event", string(serviceName))
	lastScaleStateTimePath := fmt.Sprintf("/status/services/%s/scale_to_zero/last_scale_event_time", string(serviceName))
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

func (s *AppResourceScaler) patchIguazioTenantAppServiceSets(namespace string, jsonPatchMapper []map[string]interface{}) error {
	jsonPatchMapper = append(jsonPatchMapper, map[string]interface{}{
		"op":    "add",
		"path":  "/status/state",
		"value": "waitingForScalingFromZero",
	})

	err := s.waitForNoProvisioningInProcess(namespace)
	if err != nil {
		return errors.Wrap(err, "Failed waiting for IguazioTenantAppServiceSet to finish provisioning")
	}

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

func (s *AppResourceScaler) waitForNoProvisioningInProcess(namespace string) error {
	s.logger.DebugWith("Waiting for IguazioTenantAppServiceSet to finish provisioning")
	timeout := time.After(5 * time.Minute)
	tick := time.Tick(10 * time.Second)
	for {
		_, _, state, err := s.getIguazioTenantAppServiceSets()
		if err != nil {
			return errors.Wrap(err, "Failed to get iguazio tenant app service sets")
		}

		if state == "ready" || state == "error" {
			s.logger.DebugWith("IguazioTenantAppServiceSet finished provisioning")
			return nil
		}

		s.logger.DebugWith("IguazioTenantAppServiceSet is still provisioning", "state", state)

		select {
		case <-timeout:
			return errors.New("Timed out waiting for IguazioTenantAppServiceSet to finish provisioning")
		case <-tick:
			continue
		}
	}
}

func (s *AppResourceScaler) waitForServicesState(namespace string, serviceNames []string, desiredState string) error {
	s.logger.DebugWith("Waiting for services to reach desired state", "serviceNames", serviceNames, "desiredState", desiredState)
	timeout := time.After(10 * time.Minute)
	tick := time.Tick(5 * time.Second)
	for {
		select {
		case <-timeout:
			return errors.New("Timed out waiting for services to reach desired state")
		case <-tick:
			servicesToCheck := append([]string(nil), serviceNames...)
			_, statusServicesMap, _, err := s.getIguazioTenantAppServiceSets()
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
					s.logger.DebugWith("Service did not reach desired state yet",
						"serviceName", serviceName,
						"currentState", currentState,
						"desiredState", desiredState)
					break
				}

				s.logger.DebugWith("Service reached desired state", "serviceName", serviceName, "desiredState", desiredState)
				servicesToCheck = removeStringFromSlice(serviceName, servicesToCheck)

				if len(servicesToCheck) == 0 {
					return nil
				}
			}
		}
	}
}

func getClientConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}

	return rest.InClusterConfig()
}

func (s *AppResourceScaler) getIguazioTenantAppServiceSets() (map[string]interface{}, map[string]interface{}, string, error) {
	var iguazioTenantAppServicesSetMap map[string]interface{}

	absPath := []string{"apis", "iguazio.com", "v1beta1", "namespaces", s.namespace, "iguaziotenantappservicesets", s.namespace}
	iguazioTenantAppServicesSet, err := s.kubeClientSet.Discovery().RESTClient().Get().AbsPath(absPath...).Do().Raw()

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

func (s *AppResourceScaler) parseScaleToZeroStatus(scaleToZeroStatus map[string]interface{}) (scaler_types.ScaleEvent, time.Time, error) {
	lastScaleEventString, ok := scaleToZeroStatus["last_scale_event"].(string)
	if !ok {
		return "", time.Now(), errors.New("Scale to zero status does not have last scale event")
	}

	lastScaleEvent, err := scaler_types.ParseScaleEvent(lastScaleEventString)
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

func (s *AppResourceScaler) parseLastScaleEvent(serviceStatus interface{}) (*scaler_types.ScaleEvent, *time.Time, error) {
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

func (s *AppResourceScaler) parseScaleResources(serviceSpecInterface interface{}) ([]scaler_types.ScaleResource, error) {
	var parsedScaleResources []scaler_types.ScaleResource
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

		parsedScaleResource := scaler_types.ScaleResource{
			MetricName: metricName,
			WindowSize: scaler_types.Duration{Duration: windowSize},
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
