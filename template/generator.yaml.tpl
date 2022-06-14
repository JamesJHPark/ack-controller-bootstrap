# This generator.yaml file is generated using controller-bootstrap.
# To get started, ignore the resource(s) from the following resource list and execute make build-controller from the ACK code-generator.
# Add any custom code inside the templates directory, add e2e test inside the test directory.
# Lastly, remove these comments after completing above instructions.
ignore:
  resource_names:
{{- range $resource := .ServiceResources }}
      - {{ $resource }}
{{- end }}
