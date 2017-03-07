FROM ubuntu:latest

RUN mkdir -p /usr/src/app
WORKDIR /usr/src/app
RUN apt-get update -y
RUN apt-get upgrade -y
# RUN apt-get -y install supervisor nodejs build-essential libssl-dev npm git
RUN apt-get install -y apt-utils nodejs golang build-essential libssl-dev npm git inetutils-ping
#RUN apt-get install -y build-essential libssl-dev npm git

RUN PATH=$PATH:/usr/local/bin
RUN ln -s /usr/bin/nodejs /usr/bin/node
RUN npm update -g minimatch@3.0.2
RUN npm update -g
RUN npm install -g bower polymer-cli
#RUN npm i -g npm-upgrade

RUN apt-get install telnet curl

# delete all the apt list files since they're big and get stale quickly
RUN rm -rf /var/lib/apt/lists/*

RUN mkdir -p github.com/ablox/webcat


EXPOSE 8000-10000

# ENV cluster_key Replicat Demo
# ENV d /tmp/replicat
# ENV name NodeA
#
# ENV config nodes.json
# cluster_key=Replicat Demo;d=/tmp/NodeA;name=NodeA;config=nodes.json

RUN mkdir -p /usr/src/github.com/ablox/
COPY . /usr/src/github.com/ablox/replicat
WORKDIR /usr/src/github.com/ablox/replicat

ENV GOPATH="/usr" PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:" cluster_key="Replicat Demo" d="/tmp/replicat" m="192.16.1.156:8100"

RUN go build
# a=hostname":8100"

CMD ./replicat

# globalSettings.Address

# CMD ["/usr/local/lib/node_modules/polymer-cli/bin/polymer.js serve --hostname 0.0.0.0 ."]
# WORKDIR /usr/src/github.com/ablox/webcat
# CMD ["/usr/bin/go run main.go"]

