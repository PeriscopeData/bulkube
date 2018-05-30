# bulkube
cmdline tool for bulk updating kubernetes config files

Installation:
```
go get github.com/PeriscopeData/bulkube/... && \
go install github.com/PeriscopeData/bulkube/...
```


Running:
```
$GOPATH/bin/bulkube [-l <labelSelector>] [-fmt] [-image <repo/name>] [-sha abc123]  <dir-or-file>

  -fmt
    	Reformat even if version does not change.
  -image string
    	Image to modify. Only modifies containers that match this image/repository. If @sha256: is included, will use that as sha.
  -l string
    	Filter deployments by label.
  -sha string
    	Set image version by sha.
```
