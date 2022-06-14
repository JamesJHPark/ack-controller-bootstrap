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
	awssdkmodel "github.com/aws/aws-sdk-go/private/model/api"
	"github.com/gertd/go-pluralize"
	"gopkg.in/src-d/go-git.v4"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var (
	svcID           string
	svcAbbreviation string
	svcFullName     string
	svcResources    []string
)

const (
	sdkRepoURL             = "https://github.com/aws/aws-sdk-go"
	defaultGitCloneTimeout = 180 * time.Second
)

// AWSSDKHelper is a helper struct for the aws-sdk-go load method with model API file
type AWSSDKHelper struct {
	loader *awssdkmodel.Loader
}

// getServiceResources infers aws-sdk-go to find the service metadata and resources
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

	// default value of model could be service alias - if nobody provides the option -> service alias
	// if they provide it, they will pass the model name into param of findModelAPI

	// svcFile is assigned to the service api-2.json file path
	svcFile, err := findModelAPI(repoDirPath, svcAlias)
	if err != nil {
		return err
	}

	// If service alias and service ID does not match, request user input for service model name
	if svcFile == "" {
		fmt.Print("unable to find the service api-2.json file. Please enter the service model name: ")
		r := bufio.NewReader(os.Stdin)
		modelName, err := r.ReadString('\n')
		if err != nil {
			fmt.Printf("unable to read the input for service model name %s\n", err)
			return err
		}
		modelName = strings.TrimSuffix(modelName, "\n")
		svcFile, err = findModelAPI(repoDirPath, modelName)
		if err != nil {
			return err
		}
	}
	if svcFile == "" {
		return fmt.Errorf("unable to find the service api-2.json file")
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
func findModelAPI(repoDirPath string, svcAlias string) (string, error) {
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
				if getServiceID == svcAlias {
					outFile = file
					return outFile, err
				}
			}
		}
	}
	return outFile, err
}

// modelAPI finds the aws-sdk-go model API object with the service api-2.json file.
// Service metadata and resources are inferred from the API object.
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
				svcResources = append(svcResources, resName)
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

// todo: implementation # 2
// remove the logs, err output
//log.SetOutput(ioutil.Discard)
//os.Stderr = nil
//
//h := newAWSSDKHelper(repoDirPath)
//h.altFunc(repoDirPath, svcID)
//testFunc2(repoDirPath, svcID)

// todo: implementation # 3
//f, err := os.Open(svcFile)
//if err != nil {
//	return err
//}
//defer f.Close()
//
//b, err := ioutil.ReadAll(f)
//if err != nil {
//	return err
//}
//
//var data map[string]interface{}
//err = json.Unmarshal(b, &data)
//if err != nil {
//	return err
//}

// parse the api-2.json file of a specified service
//parseServiceAPI(data)
// returns the api-2.json file for a specified service
//func (a *AWSSDKHelper) altFunc(repoDirPath string, svcAlias string) error {
//	apisPath := filepath.Join(repoDirPath, "models", "apis")
//	var operationsList []string
//	err := filepath.Walk(apisPath, func(path string, info os.FileInfo, err error) error {
//		if err != nil {
//			return err
//		}
//		if strings.Contains(path, "api-2.json") {
//			apis, err := a.loader.Load([]string{path})
//			if err != nil {
//				return err
//			}
//
//			// apis is a map of specified import path and service package name with pointers to aws-sdk-go
//			// model API object
//			for _, api := range apis {
//				//println("PRINT OUT KEY", key)
//				_ = api.ServicePackageDoc()
//				getServiceID := strings.ToLower(api.Metadata.ServiceID)
//				if getServiceID == svcAlias {
//					svcID = api.Metadata.ServiceID
//					svcAbbreviation = api.Metadata.ServiceAbbreviation
//					svcFullName = api.Metadata.ServiceFullName
//					operationsList = api.OperationNames()
//				}
//			}
//		}
//		return nil
//	})
//
//	if err != nil {
//		return err
//	}
//
//	pluralize := pluralize.NewClient()
//	// Search for operations with the prefix "Create" and append the resource names to the resources slice
//	for _, opName := range operationsList {
//		if strings.HasPrefix(opName, "CreateBatch") {
//			continue
//		}
//		if strings.HasPrefix(opName, "Create") {
//			resName := strings.TrimPrefix(opName, "Create")
//			if pluralize.IsSingular(resName) {
//				resources = append(resources, resName)
//			}
//		}
//	}
//	return nil
//}

//// todo: traverse the api-2.json file - note parseServiceAPI does not parse the array i.e. []interface{}
// parse the key-value pair of map[string]interface{} for the specified service api-2.json file
//func parseServiceAPI(dataMap map[string]interface{}) {
//	for s, v := range dataMap {
//		switch val := v.(type) {
//		case map[string]interface{}:
//			parseResources(s, "")
//			parseServiceAPI(val)
//		default:
//			getServiceName := fmt.Sprintf("%v", v)
//			parseResources(s, getServiceName)
//		}
//	}
//}
//
//func parseResources(resName string, serviceName string) {
//	if strings.HasPrefix(resName, "CreateBatch") {
//		return
//	}
//	pluralize := pluralize.NewClient()
//	if strings.HasPrefix(resName, "Create") {
//		trimName := strings.TrimPrefix(resName, "Create")
//		if pluralize.IsSingular(trimName) {
//			resources = append(resources, trimName)
//		}
//	}
//}
