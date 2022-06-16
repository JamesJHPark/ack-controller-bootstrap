SHELL := /bin/bash # Use bash syntax

# Set up variables
GO111MODULE=on

GO_CMD_FLAGS=-tags codegen

AWS_SERVICE:=$(shell echo $(SERVICE))
MODEL_NAME:=$(shell echo $(MODEL_NAME))
ifeq ($(MODEL_NAME),)
  MODEL_NAME := ""
endif
CONTROLLER_BOOTSTRAP:=./bin/controller-bootstrap
ROOT_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
CODE_GEN_DIR:=${ROOT_DIR}/../code-generator
CONTROLLER_DIR:=${ROOT_DIR}/../${AWS_SERVICE}-controller
EXISTING_CONTROLLER=true
DRY_RUN=false

#AWS_SDK_GO_VERSION=$(shell curl -H "Accept: application/vnd.github.v3+json" \
#                                 https://api.github.com/repos/aws/aws-sdk-go/releases/latest | jq -r '.tag_name')
#RUNTIME_VERSION=$(shell curl -H "Accept: application/vnd.github.v3+json" \
#								  https://api.github.com/repos/aws-controllers-k8s/runtime/releases/latest | jq -r '.tag_name')

AWS_SDK_GO_VERSION:=$(shell curl -sL https://github.com/aws/aws-sdk-go/releases/latest | grep -oE 'v+[0-9]+\.[0-9]+\.[0-9]+' | head -n 1 )

RUNTIME_VERSION:=$(shell curl -sL https://github.com/aws-controllers-k8s/runtime/releases/latest | grep -oE 'v+[0-9]+\.[0-9]+\.[0-9]+' | head -n 1 )

build:
	@go build ${GO_CMD_FLAGS} -o ${CONTROLLER_BOOTSTRAP} ./cmd/controller-bootstrap/*.go

only-scaffolding: build
	@${CONTROLLER_BOOTSTRAP} generate -o ${ROOT_DIR}/../${AWS_SERVICE}-controller -v ${AWS_SDK_GO_VERSION} -r ${RUNTIME_VERSION} -d=${DRY_RUN} -m ${MODEL_NAME} -e=false -- ${AWS_SERVICE}

update-existing: build
	@${CONTROLLER_BOOTSTRAP} generate -o ${ROOT_DIR}/../${AWS_SERVICE}-controller -v ${AWS_SDK_GO_VERSION} -r ${RUNTIME_VERSION} -d=${DRY_RUN} -m ${MODEL_NAME} -e=${EXISTING_CONTROLLER} -- ${AWS_SERVICE}

run:
	@if [ -f ${CONTROLLER_DIR}/cmd/controller/main.go ]; then \
	  make update-existing; \
	  echo "controller exists"; \
	else \
	  make controller; \
	  echo "controller doesn't exist"; \
	fi

controller: only-scaffolding
	@export SERVICE=${AWS_SERVICE}
	@echo "build controller attempt #1"
	@cd ${CODE_GEN_DIR} && make -i build-controller >/dev/null 2>/dev/null
	@echo "missing go.sum entry, running go mod tidy"
	@cd ${CONTROLLER_DIR} && go mod tidy
	@echo "build controller attempt #2"
	@cd ${CODE_GEN_DIR} && make -i build-controller >/dev/null 2>/dev/null
	@echo "go.sum outdated, running go mod tidy"
	@cd ${CONTROLLER_DIR} && go mod tidy
	@echo "final build controller attempt"
	@cd ${CODE_GEN_DIR} && make build-controller
	@echo "look inside ${AWS_SERVICE}-controller/generator.yaml for further instructions"

clean-controller-dir:
	@cd ${CONTROLLER_DIR}
	@rm -rf ${CONTROLLER_DIR}/..?* ${CONTROLLER_DIR}/.[!.]* ${CONTROLLER_DIR}/*
