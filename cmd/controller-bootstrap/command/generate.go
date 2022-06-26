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
	"fmt"
	"github.com/spf13/cobra"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

type templateVars struct {
	ServiceID           string
	ServicePackageName  string
	ServiceAbbreviation string
	ServiceFullName     string
	CRDNames            []string
	AWSSDKGoVersion     string
	RuntimeVersion      string
	ServiceModelName    string
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
	cd, err := os.Getwd()
	if err != nil {
		fmt.Printf("unable to determine current working directory: %s\n", err)
		os.Exit(1)
	}
	err = getServiceResources()
	if err != nil {
		return err
	}
	// Append the template files inside the template directory to files.
	var tplFiles []string
	tplDir := filepath.Join(cd, "template")
	err = filepath.Walk(tplDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			tplFiles = append(tplFiles, path)
		}
		return nil
	})

	// Loop over the template file paths to parse and render the files
	// in an ACK service controller repository
	for _, tplFile := range tplFiles {
		if optExistingController {
			tplFile = filepath.Join(tplDir, tplFile)
		}
		tmp, err := template.ParseFiles(tplFile)
		if err != nil {
			return err
		}

		tplVars := getTplVars()

		var buf bytes.Buffer
		if err = tmp.Execute(&buf, tplVars); err != nil {
			return err
		}

		file := strings.TrimPrefix(tplFile, tplDir)
		file = strings.TrimSuffix(file, ".tpl")
		outPath := filepath.Join(optOutputPath, file)
		outDir := filepath.Dir(outPath)

		if optDryRun {
			fmt.Printf("============================= %s ======================================\n", file)
			fmt.Println(strings.TrimSpace(buf.String()))
			continue
		}

		if _, err = ensureDir(outDir); err != nil {
			return err
		}
		if err = ioutil.WriteFile(outPath, buf.Bytes(), 0666); err != nil {
			return err
		}
	}
	return nil
}

// getTplVars returns the templateVars struct populated with service metadata,
// custom resource names, ACK test-infra commit SHA, aws-sdk-go and ACK runtime versions
func getTplVars() templateVars {
	return templateVars{
		ServiceID:           svcID,
		ServicePackageName:  strings.ToLower(optServiceAlias),
		ServiceAbbreviation: svcAbbreviation,
		ServiceFullName:     svcFullName,
		CRDNames:            crdNames,
		AWSSDKGoVersion:     optAWSSDKGoVersion,
		RuntimeVersion:      optRuntimeVersion,
		ServiceModelName:    strings.ToLower(optModelName),
		//TestInfraCommitSHA:  optTestInfraCommitSHA,
	}
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
	if !isDirWriteable(fp) {
		return true, fmt.Errorf("%s is not a writeable directory", fp)
	}

	return true, nil
}

// isDirWriteable returns true if the supplied directory path is writeable,
// false otherwise
func isDirWriteable(fp string) bool {
	testPath := filepath.Join(fp, "test")
	f, err := os.Create(testPath)
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(testPath)
	return true
}
