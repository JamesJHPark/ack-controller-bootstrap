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

type svcVars struct {
	ServiceID           string
	ServicePackageName  string
	ServiceAbbreviation string
	ServiceFullName     string
	CRDNames            []string
	AWSSDKGoVersion     string
	RuntimeVersion      string
	ServiceModelName    string
}

var staticFiles = []string{
	"CODE_OF_CONDUCT.md.tpl",
	"CONTRIBUTING.md.tpl",
	"GOVERNANCE.md.tpl",
	"LICENSE.tpl",
	"NOTICE.tpl",
	"SECURITY.md.tpl",
}

var templateCmd = &cobra.Command{
	Use:   "generate",
	Short: "bootstrap an ACK service controller",
	RunE:  generateTemplates,
}

// generateTemplates generates the template files in an ACK service controller repository
func generateTemplates(cmd *cobra.Command, args []string) error {
	cd, err := os.Getwd()
	if err != nil {
		fmt.Printf("unable to determine current working directory: %s\n", err)
		os.Exit(1)
	}

	svcAlias := strings.ToLower(optServiceAlias)
	err = getServiceResources(svcAlias)
	if err != nil {
		return err
	}
	tplVars := svcVars{
		ServiceID:           svcID,
		ServicePackageName:  svcAlias,
		ServiceAbbreviation: svcAbbreviation,
		ServiceFullName:     svcFullName,
		CRDNames:            crdNames,
		AWSSDKGoVersion:     optAWSSDKGoVersion,
		RuntimeVersion:      optRuntimeVersion,
		ServiceModelName:    optModelName,
	}

	// Append the template files inside the template directory to filePaths.
	// For an existing service controller, only update the list of files in the staticFiles slice
	var filePaths []string
	basePath := filepath.Join(cd, "template")
	if optExistingController {
		filePaths = staticFiles
	} else {
		err = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				filePaths = append(filePaths, path)
			}
			return nil
		})
	}

	// Loop over the template file paths to parse and render the files
	// in an ACK service controller repository
	for _, filePath := range filePaths {

		if optExistingController {
			filePath = filepath.Join(basePath, filePath)
		}
		tmp, err := template.ParseFiles(filePath)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err = tmp.Execute(&buf, tplVars); err != nil {
			return err
		}
		trimPath := strings.TrimPrefix(filePath, basePath)
		trimPath = strings.TrimSuffix(trimPath, ".tpl")
		if optDryRun {
			fmt.Printf("============================= %s ======================================\n", trimPath)
			fmt.Println(strings.TrimSpace(buf.String()))
			continue
		}
		outPath := filepath.Join(optOutputPath, trimPath)
		outDir := filepath.Dir(outPath)
		if _, err := ensureDir(outDir); err != nil {
			return err
		}
		if err := ioutil.WriteFile(outPath, buf.Bytes(), 0666); err != nil {
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
