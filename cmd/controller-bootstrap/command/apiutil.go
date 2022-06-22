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
	"bufio"
	"context"
	"fmt"
	"github.com/gertd/go-pluralize"
	"gopkg.in/src-d/go-git.v4"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	awssdkmodel "github.com/aws/aws-sdk-go/private/model/api"
)

var (
	svcFile         string
	svcID           string
	svcAbbreviation string
	svcFullName     string
	crdNames        []string
)

const (
	sdkRepoURL             = "https://github.com/aws/aws-sdk-go"
	defaultGitCloneTimeout = 180 * time.Second
)

// AWSSDKHelper is a helper struct for aws-sdk-go model API loader
type AWSSDKHelper struct {
	loader *awssdkmodel.Loader
}

// getServiceResources infers aws-sdk-go to fetch the service metadata and custom resource names
func getServiceResources(svcAlias string) error {
	hd, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("unable to determine $HOME: %s\n", err)
		os.Exit(1)
	}
	sdkDirDefaultPath := filepath.Join(hd, ".cache", "aws-controllers-k8s")
	ctx, cancel := contextWithSigterm(context.Background())
	defer cancel()
	sdkDir, err := ensureSDKRepo(ctx, sdkDirDefaultPath)

	// If the supplied service alias and service ID do not match,
	// pass in the service model name to modelAPIPath
	if optModelName != "" {
		svcFile, err = modelAPIPath(sdkDir, optModelName)
	} else {
		svcFile, err = modelAPIPath(sdkDir, svcAlias)
	}
	if err != nil {
		return err
	}
	if svcFile == "" {
		return fmt.Errorf("unable to find the service api-2.json file, please specify the service model name")
	}
	h := newAWSSDKHelper(sdkDir)
	err = h.modelAPI(svcFile)
	if err != nil {
		return err
	}
	return nil
}

// newAWSSDKHelper returns a new AWSSDKHelper struct
func newAWSSDKHelper(repoDirPath string) *AWSSDKHelper {
	return &AWSSDKHelper{
		loader: &awssdkmodel.Loader{
			BaseImport:            repoDirPath,
			IgnoreUnsupportedAPIs: true,
		},
	}
}

// modelAPIPath returns the api-2.json file path of a supplied AWS service alias
func modelAPIPath(repoDirPath string, svcIdentifier string) (string, error) {
	apisPath := filepath.Join(repoDirPath, "models", "apis")
	var filePaths []string
	err := filepath.Walk(apisPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(path, "api-2.json") {
			filePaths = append(filePaths, path)
		}
		return nil
	})
	outPath := ""
	for _, filePath := range filePaths {
		openFile, err := os.Open(filePath)
		if err != nil {
			return outPath, err
		}
		defer openFile.Close()
		scanner := bufio.NewScanner(openFile)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "serviceId") {
				getServiceID := strings.Split(scanner.Text(), ":")[1]
				re := regexp.MustCompile(`[," \t]`)
				getServiceID = strings.ToLower(re.ReplaceAllString(getServiceID, ``))
				if getServiceID == svcIdentifier {
					outPath = filePath
					return outPath, err
				}
			}
		}
	}
	return outPath, err
}

// modelAPI finds the service metadata and custom resource names from aws-sdk-go model API object
func (a *AWSSDKHelper) modelAPI(filePath string) error {
	// loads the API model file(s) and returns the map of API package
	apis, err := a.loader.Load([]string{filePath})
	if err != nil {
		return err
	}
	var opNames []string
	// apis is a map, keyed by the service package names, of pointers to aws-sdk-go model API objects
	for _, api := range apis {
		_ = api.ServicePackageDoc()
		svcID = api.Metadata.ServiceID
		svcAbbreviation = api.Metadata.ServiceAbbreviation
		svcFullName = api.Metadata.ServiceFullName
		opNames = api.OperationNames()
	}
	pluralize := pluralize.NewClient()
	// search for operations with the prefix "Create" and append the custom resource names to crdNames
	for _, opName := range opNames {
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
	return nil
}

// ensureSDKRepo ensures that we have a git clone'd copy of the aws-sdk-go
// repository, which we use model JSON files from
func ensureSDKRepo(
	ctx context.Context,
	cacheDir string,
) (string, error) {
	var err error
	srcPath := filepath.Join(cacheDir, "src")
	if err = os.MkdirAll(srcPath, os.ModePerm); err != nil {
		return "cannot create directory", err
	}

	// Clone repository if it doen't exist
	sdkDir := filepath.Join(srcPath, "aws-sdk-go")

	if _, err = os.Stat(sdkDir); os.IsNotExist(err) {

		ct, cancel := context.WithTimeout(ctx, defaultGitCloneTimeout)
		defer cancel()
		err = CloneRepository(ct, sdkDir, sdkRepoURL)
		if err != nil {
			return "cannot clone repository: %v", err
		}
	}
	return sdkDir, err
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
