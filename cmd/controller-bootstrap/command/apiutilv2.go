// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package command

import (
	"fmt"
	"github.com/gertd/go-pluralize"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	awssdkmodel "github.com/aws/aws-sdk-go/private/model/api"
)

type metaVars struct {
	ServiceID           string
	ServicePackageName  string
	ServiceModelName    string
	ServiceAbbreviation string
	ServiceFullName     string
	CRDNames            []string
}

const (
	sdkRepoURL             = "https://github.com/aws/aws-sdk-go"
	defaultGitCloneTimeout = 180 * time.Second
)

// AWSSDKHelper is a helper struct for aws-sdk-go model API loader
type AWSSDKHelper struct {
	loader *awssdkmodel.Loader
	// Default is set by `FirstAPIVersion`
	apiVersion string
}

// getServiceResources infers aws-sdk-go to fetch the service metadata and custom resource names
func getServiceResources() (*metaVars, error) {
	h := newAWSSDKHelper()
	svcVars, err := h.API()
	if err != nil {
		return nil, fmt.Errorf("unable to find the supplied service's API file, please re-try specifying the service model name")
	}
	return svcVars, nil
}

// newAWSSDKHelper returns a new AWSSDKHelper struct
func newAWSSDKHelper() *AWSSDKHelper {
	return &AWSSDKHelper{
		loader: &awssdkmodel.Loader{
			BaseImport:            sdkDir,
			IgnoreUnsupportedAPIs: true,
		},
	}
}

// API returns the populated metaVars struct with the service metadata
// and custom resource names extracted from the aws-sdk-go model API object
func (h *AWSSDKHelper) API() (*metaVars, error) {
	serviceModelName := strings.ToLower(optModelName)
	if optModelName == "" {
		serviceModelName = strings.ToLower(optServiceAlias)
	}
	modelPath, err := h.findModelPath(serviceModelName)

	// loads the API model file(s) and returns the map of API package
	apis, err := h.loader.Load([]string{modelPath})
	if err != nil {
		return nil, err
	}
	// apis is a map, keyed by the service package name, of pointers
	// to aws-sdk-go model API objects
	for _, api := range apis {
		_ = api.ServicePackageDoc()
		svcMetaVars := serviceMetaVars(api)
		return svcMetaVars, nil
	}
	return nil, err
}

// findModelPath returns the path to the supplied service's API file
func (h *AWSSDKHelper) findModelPath(
	serviceModelName string,
) (string, error) {
	if h.apiVersion == "" {
		apiVersion, err := h.firstAPIVersion(serviceModelName)
		if err != nil {
			return "", err
		}
		h.apiVersion = apiVersion
	}
	versionPath := filepath.Join(
		sdkDir, "models", "apis", serviceModelName, h.apiVersion,
	)
	modelPath := filepath.Join(versionPath, "api-2.json")
	return modelPath, nil
}

// FirstAPIVersion returns the first found API version for a service API.
// (e.h. "2012-10-03")
func (h *AWSSDKHelper) firstAPIVersion(serviceModelName string) (string, error) {
	versions, err := h.getAPIVersions(serviceModelName)
	if err != nil {
		return "", err
	}
	sort.Strings(versions)
	return versions[0], nil
}

// GetAPIVersions returns the list of API Versions found in a service directory.
func (h *AWSSDKHelper) getAPIVersions(serviceModelName string) ([]string, error) {
	apiPath := filepath.Join(sdkDir, "models", "apis", serviceModelName)
	versionDirs, err := ioutil.ReadDir(apiPath)
	if err != nil {
		return nil, err
	}
	versions := []string{}
	for _, f := range versionDirs {
		version := f.Name()
		fp := filepath.Join(apiPath, version)
		fi, err := os.Lstat(fp)
		if err != nil {
			return nil, err
		}
		if !fi.IsDir() {
			return nil, fmt.Errorf("found %s: %v", version, "expected to find only directories in api model directory but found non-directory")
		}
		versions = append(versions, version)
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no valid version directories found")
	}
	return versions, nil
}

// serviceMetaVars returns a metaVars struct populated with metadata
// and custom resource names for the supplied AWS service
func serviceMetaVars(api *awssdkmodel.API) *metaVars {
	return &metaVars{
		ServicePackageName:  strings.ToLower(optServiceAlias),
		ServiceID:           api.Metadata.ServiceID,
		ServiceModelName:    strings.ToLower(optModelName),
		ServiceAbbreviation: api.Metadata.ServiceAbbreviation,
		ServiceFullName:     api.Metadata.ServiceFullName,
		CRDNames:            getCRDNames(api),
	}
}

// getCRDNames appends custom resource names with the prefix "Create" followed by a singular noun
// to the slice, crdNames
func getCRDNames(api *awssdkmodel.API) []string {
	var crdNames []string
	pluralize := pluralize.NewClient()
	for _, opName := range api.OperationNames() {
		if strings.HasPrefix(opName, "CreateBatch") {
			continue
		}
		if strings.HasPrefix(opName, "Create") {
			resName := strings.TrimPrefix(opName, "Create")
			if pluralize.IsSingular(resName) {
				crdNames = append(crdNames, resName)
			}
		}
	}
	return crdNames
}
