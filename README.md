[![Go Report Card](https://goreportcard.com/badge/github.com/ablox/replicat)](https://goreportcard.com/report/github.com/ablox/replicat)

# replicat
rsync for the cloud

usage:
directory=/tmp/foo go run replicat


Start Replicat with my custom key

1. Ray:    replicat --clusterKey aa88aa88aa88 --directory /tmp/foo

2. Jacob:  replicat --clusterKey aa88aa88aa88 --directory /tmp/foo (this uses machine's hostname:port as the name)

3. James:  replicat --clusterKey aa88aa88aa88 --directory /tmp/foo
