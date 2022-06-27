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
	"bytes"
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"gopkg.in/src-d/go-git.v4"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"
)

type templateVars struct {
	*metaVars
	AWSSDKGoVersion  string
	RuntimeVersion   string
	ServiceModelName string
	//TestInfraCommitSHA  string
}

var templateCmd = &cobra.Command{
	Use:   "generate",
	Short: "generate template files in an ACK service controller repository",
	RunE:  generateController,
}

// generateController creates the initial directories and files for a service controller
// repository by rendering go template files.
// TODO: When a controller is already existing, then this method only updates the project
// description files.
func generateController(cmd *cobra.Command, args []string) error {
	hd, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("unable to determine $HOME: %s\n", err)
		os.Exit(1)
	}
	cacheACKDir := filepath.Join(hd, ".cache", "aws-controllers-k8s")
	ctx, cancel := contextWithSigterm(context.Background())
	defer cancel()
	if err = ensureSDKRepo(ctx, cacheACKDir); err != nil {
		return err
	}

	cd, err := os.Getwd()
	if err != nil {
		fmt.Printf("unable to determine current working directory: %s\n", err)
		os.Exit(1)
	}
	tplDir := filepath.Join(cd, "template")

	svcVars, err := getServiceResources()
	if err != nil {
		return err
	}
	tplVars := &templateVars{
		svcVars,
		optAWSSDKGoVersion,
		optRuntimeVersion,
		optModelName,
	}

	// Append the template files inside the template directory to tplPaths.
	var tplPaths []string
	err = filepath.Walk(tplDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			tplPaths = append(tplPaths, path)
		}
		return nil
	})

	// Loop over the template files from the template directory
	// and parse, render the files in an ACK service controller repository
	for _, tplPath := range tplPaths {
		tmp, err := template.ParseFiles(tplPath)
		if err != nil {
			return err
		}

		var buf bytes.Buffer
		if err = tmp.Execute(&buf, tplVars); err != nil {
			return err
		}

		file := strings.TrimPrefix(tplPath, tplDir)
		file = strings.TrimSuffix(file, ".tpl")

		if optDryRun {
			fmt.Printf("============================= %s ======================================\n", file)
			fmt.Println(strings.TrimSpace(buf.String()))
			continue
		}

		outPath := filepath.Join(optOutputPath, file)
		outDir := filepath.Dir(outPath)
		if _, err = ensureDir(outDir); err != nil {
			return err
		}
		if err = ioutil.WriteFile(outPath, buf.Bytes(), 0666); err != nil {
			return err
		}
	}
	return nil
}

// ensureDir makes sure that a supplied directory exists and
// returns whether the directory already existed.
func ensureDir(fp string) (bool, error) {
	fi, err := os.Stat(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return false, os.MkdirAll(fp, os.ModePerm)
		}
		return false, err
	}
	if !fi.IsDir() {
		return false, fmt.Errorf("expected %s to be a directory", fp)
	}

	return true, nil
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
