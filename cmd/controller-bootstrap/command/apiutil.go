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
	"context"
	"fmt"
	"github.com/gertd/go-pluralize"
	"gopkg.in/src-d/go-git.v4"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
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
}

// getServiceResources infers aws-sdk-go to fetch the service metadata and custom resource names
func getServiceResources() (*metaVars, error) {
	hd, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("unable to determine $HOME: %s\n", err)
		os.Exit(1)
	}
	cacheACKDir := filepath.Join(hd, ".cache", "aws-controllers-k8s")
	ctx, cancel := contextWithSigterm(context.Background())
	defer cancel()
	if err = ensureSDKRepo(ctx, cacheACKDir); err != nil {
		return nil, err
	}

	modelPath, err := findModelPath()
	if err != nil {
		return nil, err
	}
	if modelPath == "" {
		return nil, fmt.Errorf("unable to find the supplied service's API file, please try specifying the service model name")
	}
	h := newAWSSDKHelper()
	svcVars, err := h.modelAPI(modelPath)
	if err != nil {
		return nil, err
	}
	return svcVars, nil
}

// findModelPath returns path to the supplied service's API file
func findModelPath() (string, error) {
	serviceModelName := strings.ToLower(optModelName)
	if optModelName == "" {
		serviceModelName = strings.ToLower(optServiceAlias)
	}
	apiPath := filepath.Join(sdkDir, "models", "apis", serviceModelName)
	apiFile := ""
	err := filepath.Walk(apiPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == "api-2.json" {
			_, err = os.Open(path)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return apiFile, nil
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

// modelAPI extracts the service metadata and API operations from aws-sdk-go model API object
func (a *AWSSDKHelper) modelAPI(modelPath string) (*metaVars, error) {
	// loads the API model file(s) and returns the map of API package
	apis, err := a.loader.Load([]string{modelPath})
	if err != nil {
		return nil, err
	}
	// apis is a map, keyed by the service package names, of pointers to aws-sdk-go model API objects
	for _, api := range apis {
		_ = api.ServicePackageDoc()
		svcMetaVars := serviceMetaVars(api)
		return svcMetaVars, nil
	}
	return nil, err
}

// getMetaVars returns a MetaVars struct populated with service metadata
// and custom resource names of the AWS service
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

// ensureSDKRepo ensures that we have a git clone'd copy of the aws-sdk-go
// repository, which we use model JSON files from.
func ensureSDKRepo(
	ctx context.Context,
	cacheDir string,
) error {
	var err error
	srcPath := filepath.Join(cacheDir, "src")
	if err = os.MkdirAll(srcPath, os.ModePerm); err != nil {
		return err
	}

	// Clone repository if it doen't exist
	sdkDir = filepath.Join(srcPath, "aws-sdk-go")

	if _, err = os.Stat(sdkDir); os.IsNotExist(err) {

		ct, cancel := context.WithTimeout(ctx, defaultGitCloneTimeout)
		defer cancel()
		err = CloneRepository(ct, sdkDir, sdkRepoURL)
		if err != nil {
			return fmt.Errorf("canot clone repository: %v", err)
		}
	}
	return err
}

// CloneRepository clones a git repository into a given directory.
// Calling this function is equivalent to executing `git clone $repositoryURL $path`
func CloneRepository(ctx context.Context, path, repositoryURL string) error {
	_, err := git.PlainCloneContext(ctx, path, false, &git.CloneOptions{
		URL:      repositoryURL,
		Progress: nil,
		// Clone and fetch all tags
		Tags: git.AllTags,
	})
	return err
}

func contextWithSigterm(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	signalCh := make(chan os.Signal, 1)

	// recreate the context.CancelFunc
	cancelFunc := func() {
		signal.Stop(signalCh)
		cancel()
	}

	// notify on SIGINT or SIGTERM
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-signalCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancelFunc
}
