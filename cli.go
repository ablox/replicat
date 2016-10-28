// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"github.com/urfave/cli"
	"os"
	"encoding/json"
)

// Settings for the server
type Node struct {
	Directory          string
	Address            string
}

type Settings struct {
	Name			   string
	ManagerAddress     string
	ManagerCredentials string
	Nodes		       map[string]Node
}

var globalSettings Settings

// SetupCli sets up the command line environment. Provide help and read the settings in.
func SetupCli() {

	app := cli.NewApp()
	app.Name = "Replicat"
	app.Usage = "rsync for the cloud"
	app.Action = func(c *cli.Context) error {

		if c.GlobalString("name") != "" {
			globalSettings.Name = c.GlobalString("name")
		}

		if globalSettings.Name == "" {
			panic("Name is currently a required parameter. Name has to be one of the predefined names (e.g. NodeA, NodeB). This will improve.\n")
		}

		if c.GlobalString("config") != "" {
			configFile, err := os.Open(c.GlobalString("config"))
			if err != nil {
				panic("cannot load config file.")
			}
			jsonParser := json.NewDecoder(configFile)
			if err = jsonParser.Decode(&globalSettings); err != nil {
				panic("cannot decode config file.")
			}
		}

		node := globalSettings.Nodes[globalSettings.Name]
		if c.GlobalString("address") != "" {
			node.Address = c.GlobalString("address")
		}
		if c.GlobalString("directory") != "" {
			node.Directory = c.GlobalString("directory")
		}
		globalSettings.Nodes[globalSettings.Name] = node

		if c.GlobalString("manager") != "" {
			globalSettings.ManagerAddress = c.GlobalString("manager")
		}
		if c.GlobalString("manager_credentials") != "" {
			globalSettings.ManagerCredentials = c.GlobalString("manager_credentials")
		}

		return nil
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "config, c",
			Usage:  "Specify a path to a config file.",
			EnvVar: "config, c",
		},
		cli.StringFlag{
			Name:   "directory, d",
			Usage:  "Specify a directory where the files to share are located.",
			EnvVar: "directory, d",
		},
		cli.StringFlag{
			Name:   "manager, m",
			Value:  globalSettings.ManagerAddress,
			Usage:  "Specify a host and port for reaching the manager",
			EnvVar: "manager, m",
		},
		cli.StringFlag{
			Name:   "manager_credentials, mc",
			Value:  globalSettings.ManagerCredentials,
			Usage:  "Specify a usernmae:password for login to the manager",
			EnvVar: "manager_credentials, mc",
		},
		cli.StringFlag{
			Name:   "address, a",
			Usage:  "Specify a listen address for this node. e.g. '127.0.0.1:8000' or ':8000' for where updates are accepted from",
			EnvVar: "address, a",
		},
		cli.StringFlag{
			Name:   "name, n",
			Value:  globalSettings.Name,
			Usage:  "Specify a name for this node. e.g. 'NodeA' or 'NodeB'",
			EnvVar: "name, n",
		},
	}

	app.Run(os.Args)
}
