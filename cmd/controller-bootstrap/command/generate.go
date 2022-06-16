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

type metaVars struct {
	ServiceID           string
	ServicePackageName  string
	ServiceAbbreviation string
	ServiceFullName     string
	ServiceResources    []string
	AWSSDKGoVersion     string
	RuntimeVersion      string
}

var templateCmd = &cobra.Command{
	Use:   "generate <service>",
	Short: "generate template files for an ACK service controller",
	RunE:  generateTemplates,
}

// generateTemplates generate the template files in an ACK service controller repository
func generateTemplates(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("please specify the AWS service alias to generate template files")
	}
	cd, err := os.Getwd()
	if err != nil {
		fmt.Printf("unable to determine current working directory: %s\n", err)
		os.Exit(1)
	}
	svcAlias := strings.ToLower(args[0])
	err = getServiceResources(svcAlias)
	if err != nil {
		return err
	}

	// Initialize tplVars with the inferred service metadata and resources
	tplVars := metaVars{
		ServiceID:           svcID,
		ServicePackageName:  svcAlias,
		ServiceAbbreviation: svcAbbreviation,
		ServiceFullName:     svcFullName,
		ServiceResources:    svcResources,
		AWSSDKGoVersion:     optAWSSDKGoVersion,
		RuntimeVersion:      optRuntimeVersion,
	}

	// Append the template files inside the template directory to filePaths
	var filePaths []string
	basePath := filepath.Join(cd, "template")
	err = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			filePaths = append(filePaths, path)
		}
		return nil
	})

	// Loop over the template file paths to parse and apply the template files to the tplVars data object
	for _, file := range filePaths {
		tmp, err := template.ParseFiles(file)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err = tmp.Execute(&buf, tplVars); err != nil {
			return err
		}
		// For an existing service controller, update README.md, OWNERS, OWNERS_ALIAS files
		if optExistingController {
			var updateFile bool
			updateFile = strings.HasSuffix(file, "README.md.tpl") || strings.Contains(file, "OWNERS")
			// Provide dry-run functionality, default value set to false.
			if optDryRun && updateFile {
				fmt.Printf("============================= %s ======================================\n", file)
				fmt.Println(strings.TrimSpace(buf.String()))
				continue
			}
			if updateFile {
				err = writeFiles(file, cd, buf)
				if err != nil {
					return err
				}
			}
		} else {
			if optDryRun {
				fmt.Printf("============================= %s ======================================\n", file)
				fmt.Println(strings.TrimSpace(buf.String()))
				continue
			}
			err = writeFiles(file, cd, buf)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// writeFiles processes and generates the template files/directories in an ACK service controller repository
func writeFiles(file string, cd string, buf bytes.Buffer) error {
	trimFile := strings.TrimPrefix(file, filepath.Join(cd, "template"))
	trimFile = strings.TrimSuffix(trimFile, ".tpl")
	filePath := filepath.Join(optOutputPath, trimFile)
	outDir := filepath.Dir(filePath)
	if _, err := ensureDir(outDir); err != nil {
		return err
	}
	if err := ioutil.WriteFile(filePath, buf.Bytes(), 0666); err != nil {
		return err
	}
	return nil
}

// ensureDir makes sure that a supplied directory exists and
// returns whether the directory already existed.
func ensureDir(fp string) (bool, error) {
	fi, err := os.Stat(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return false,
				os.MkdirAll(fp, os.ModePerm)
		}
		return false, err
	}
	if !fi.IsDir() {
		return false, fmt.Errorf("expected %s to be a directory", fp)
	}
	return true, nil
}
