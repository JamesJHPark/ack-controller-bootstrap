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
	"github.com/spf13/cobra"
	"os"
)

const (
	appName      = "controller-bootstrap"
	appShortDesc = "controller-bootstrap initializes a new ACK service controller repository"
)

var (
	optOutputPath         string
	optAWSSDKGoVersion    string
	optRuntimeVersion     string
	optModelName          string
	optDryRun             bool
	optExistingController bool
)

// rootCmd represents the base command when called without any subcommands
// placeholder for cobra description
var rootCmd = &cobra.Command{
	Use:   appName,
	Short: appShortDesc,
}

func init() {
	_, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("unable to determine $HOME: %s\n", err)
		os.Exit(1)
	}
	_, err = os.Getwd()
	if err != nil {
		fmt.Printf("unable to determine current working directory: %s\n", err)
		os.Exit(1)
	}
	rootCmd.PersistentFlags().StringVarP(
		&optOutputPath, "output", "o", "", "Path to ACK service controller directory to bootstrap",
	)
	rootCmd.PersistentFlags().StringVarP(
		&optAWSSDKGoVersion, "aws-sdk-go-version", "v", "", "aws-sdk-go-version",
	)
	rootCmd.PersistentFlags().StringVarP(
		&optRuntimeVersion, "aws-controllers-k8s/runtime version", "r", "", "aws-controllers-k8s/runtime version",
	)
	rootCmd.PersistentFlags().StringVarP(
		&optModelName, "service model name", "m", "", "service model name of the supplied service alias",
	)
	rootCmd.PersistentFlags().BoolVarP(
		&optDryRun, "dry-run", "d", false, "If true, output files to stdout",
	)
	rootCmd.PersistentFlags().BoolVarP(
		&optExistingController, "existing service controller", "e", false, "If true, update the existing controller",
	)
	rootCmd.AddCommand(templateCmd)
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
