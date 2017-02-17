// Copyright 2016 Jacob Taylor jacob@ablox.io
// License: Apache2 - http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"encoding/json"
	"github.com/ablox/replicat"
	"github.com/urfave/cli"
	"math/rand"
	"os"
	"strconv"
)

// SetupCli sets up the command line environment. Provide help and read the settings in.
func SetupCli() {
	app := cli.NewApp()
	app.Name = "Replicat"
	app.Usage = "rsync for the cloud"
	app.Action = func(c *cli.Context) error {
		globalSettings := replicat.GetGlobalSettings()
		if c.GlobalString("name") != "" {
			globalSettings.Name = c.GlobalString("name")
		} else {
			name, err := os.Hostname()

			if err != nil {
				panic(err)
			}
			globalSettings.Name = name + "-" + strconv.Itoa(rand.Intn(100))
		}

		if globalSettings.Name == "" {
			panic("Name is currently a required parameter. Name has to be one of the predefined names (e.g. NodeA, NodeB). This will improve.\n")
		}

		json_file := "nodes.json"
		if c.GlobalString("config") != "" {
			json_file = c.GlobalString("config")
		}

		// read defaults
		configFile, err := os.Open(json_file)
		if err != nil {
			panic("cannot load config file.")
		}
		jsonParser := json.NewDecoder(configFile)
		if err = jsonParser.Decode(&globalSettings); err != nil {
			panic("cannot decode config file.")
		}

		if c.GlobalString("address") != "" {
			globalSettings.Address = c.GlobalString("address")
		}
		if c.GlobalString("directory") != "" {
			globalSettings.Directory = c.GlobalString("directory")
		}

		if c.GlobalString("manager") != "" {
			globalSettings.ManagerAddress = c.GlobalString("manager")
		}
		if c.GlobalString("manager_credentials") != "" {
			globalSettings.ManagerCredentials = c.GlobalString("manager_credentials")
		}

		if c.GlobalString("cluster_key") != "" {
			globalSettings.ClusterKey = c.GlobalString("cluster_key")
		}

		replicat.SetGlobalSettings(globalSettings)
		return nil
	}

	globalSettings := replicat.GetGlobalSettings()

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
			Name:   "cluster_key, ck",
			Usage:  "Specify cluster's key.",
			EnvVar: "cluster_key, ck",
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
