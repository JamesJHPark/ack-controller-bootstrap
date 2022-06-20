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
	svcID           string
	svcAbbreviation string
	svcFullName     string
	crdNames        []string
	svcFile         string
)

const (
	sdkRepoURL             = "https://github.com/aws/aws-sdk-go"
	defaultGitCloneTimeout = 180 * time.Second
)

// AWSSDKHelper is a helper struct for aws-sdk-go model API
type AWSSDKHelper struct {
	loader *awssdkmodel.Loader
}

// getServiceResources infers aws-sdk-go to find the service metadata and custom resources
func getServiceResources(svcAlias string) error {
	hd, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("unable to determine $HOME: %s\n", err)
		os.Exit(1)
	}
	repoPath := filepath.Join(hd, ".cache", "aws-controllers-k8s")
	ctx, cancel := contextWithSigterm(context.Background())
	defer cancel()
	repoDirPath, err := ensureSDKRepo(ctx, repoPath)

	// If the supplied service alias and service ID do not match,
	// pass in the service model name to findModelAPI
	if optModelName != "" {
		svcFile, err = findModelAPI(repoDirPath, optModelName)
	} else {
		svcFile, err = findModelAPI(repoDirPath, svcAlias)
	}
	if svcFile == "" {
		return fmt.Errorf("unable to find the service api-2.json file")
	}
	if err != nil {
		return err
	}

	// Find the service metadata and resource names of a supplied service alias
	h := newAWSSDKHelper(repoDirPath)
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

// findModelAPI returns the api-2.json file of a supplied AWS service alias
func findModelAPI(repoDirPath string, svcIdentifier string) (string, error) {
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
	outFile := ""
	for _, file := range filePaths {
		openFile, err := os.Open(file)
		if err != nil {
			return outFile, err
		}
		defer openFile.Close()
		scanner := bufio.NewScanner(openFile)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "serviceId") {
				getServiceID := strings.Split(scanner.Text(), ":")[1]
				re := regexp.MustCompile(`[," \t]`)
				getServiceID = strings.ToLower(re.ReplaceAllString(getServiceID, ``))
				if getServiceID == svcIdentifier {
					outFile = file
					return outFile, err
				}
			}
		}
	}
	return outFile, err
}

// modelAPI infers service metadata and custom resources from the aws-sdk-go model API object
func (a *AWSSDKHelper) modelAPI(filePath string) error {
	// loads the API model file(s) and returns the map of API package
	apis, err := a.loader.Load([]string{filePath})
	if err != nil {
		return err
	}
	var operationsList []string
	// apis is a map with key containing service package name and value referencing aws-sdk-go model API object
	for _, api := range apis {
		_ = api.ServicePackageDoc()
		svcID = api.Metadata.ServiceID
		svcAbbreviation = api.Metadata.ServiceAbbreviation
		svcFullName = api.Metadata.ServiceFullName
		operationsList = api.OperationNames()
	}
	pluralize := pluralize.NewClient()
	// Search for operations with the prefix "Create" and append the filtered resource names to svcResources
	for _, opName := range operationsList {
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
// repository, which we use model JSON files from. Upon successful return of
// this function, the sdkDir global variable will be set to the directory where
// the aws-sdk-go is found. It will also optionally fetch all the remote tags
// and checkout the given tag.
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
