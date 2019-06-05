package generator

var SourceTemplate = `
{{define "source-rke"}}
<source>
  @type  tail
  path  /var/lib/rancher/rke/log/*.log
  pos_file  /fluentd/log/{{ .RkeLogPosFilename }}
  time_format  %Y-%m-%dT%H:%M:%S.%N
  tag  {{ .RkeLogTag }}.*
  format  json
  read_from_head  true
</source>
{{end}}

{{define "source-container"}}
<source>
  @type  tail
  path  /var/log/containers/*.log
  pos_file  /fluentd/log/{{ .ContainerLogPosFilename}}
  time_format  %Y-%m-%dT%H:%M:%S.%N
  tag  {{ .ContainerLogSourceTag }}.*
  format  json
  read_from_head  true
</source>
{{end}}
`
