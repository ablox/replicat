[![Go Report Card](https://goreportcard.com/badge/github.com/ablox/replicat)](https://goreportcard.com/report/github.com/ablox/replicat)

# replicat
Replication for the cloud

usage:
directory=/tmp/foo go run replicat


Start Replicat with my custom key

1. Ray:    replicat --clusterKey aa88aa88aa88 --directory /tmp/foo

2. Jacob:  replicat --clusterKey aa88aa88aa88 --directory /tmp/foo (this uses machine's hostname:port as the name)

3. James:  replicat --clusterKey aa88aa88aa88 --directory /tmp/foo

// Instances are using tmp folder for testing. Switch to a permanant folder for production or long term use.

go get golang.org/x/crypto/blake2b




# catapp
docker run -ti -p 8080:8080 cbd58a4e2054

# webcat 
docker run -ti -p 8000:8000 -p 8100:8100 eefc3a03ea1b /bin/bash
go build
./webcat &

<!-- docker run -ti -p 8100:8100 -p 8000:8000 13d17943db5b /bin/bash -->


# replicat
docker run -ti -p 8101:8101 9ca776207015 /bin/bash
./replicat --m=192.168.1.156:8100 --a=0.0.0.0:8101

docker run -ti -p 8102:8102 9ca776207015 /bin/bash
./replicat --m=192.168.1.156:8100 --a=0.0.0.0:8102

fc48c7fff890



# open URL
http://0.0.0.0:8080/


docker run -ti -p 8080:8080 -ip 172.17.0.2 cbd58a4e2054


