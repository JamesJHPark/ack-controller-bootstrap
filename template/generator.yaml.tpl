ignore:
  resource_names:
{{- range $crdName := .CRDNames }}
      - {{ $crdName }}
{{- end }}
