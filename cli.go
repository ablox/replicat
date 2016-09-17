package main

import (
	"github.com/urfave/cli"
	"os"
)

// settings for the server
type Settings struct {
	Directory          string
	ManagerAddress     string
	ManagerCredentials string
	ManagerEnabled     bool
	Address            string
	Name               string
	BootstrapAddress   string
}

var globalSettings Settings = Settings{
	Directory:          "",
	ManagerAddress:     "localhost:8080",
	ManagerCredentials: "replicat:isthecat",
	Address:            ":8001",
	Name:               "",
	BootstrapAddress:   ":8080",
}

func SetupCli() {

	app := cli.NewApp()
	app.Name = "Replicat"
	app.Usage = "rsync for the cloud"
	app.Action = func(c *cli.Context) error {
		globalSettings.Directory = c.GlobalString("directory")
		globalSettings.ManagerAddress = c.GlobalString("manager")
		globalSettings.ManagerCredentials = c.GlobalString("manager_credentials")
		globalSettings.Address = c.GlobalString("address")
		globalSettings.Name = c.GlobalString("name")
		globalSettings.BootstrapAddress = c.GlobalString("bootstrap_address")

		if globalSettings.Directory == "" {
			panic("directory is required to serve files\n")
		}

		if globalSettings.Name == "" {
			panic("Name is currently a required parameter. Name has to be one of the predefined names (e.g. NodeA, NodeB). This will improve.\n")
		}

		return nil
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "directory, d",
			Value:  globalSettings.Directory,
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
			Value:  globalSettings.Address,
			Usage:  "Specify a listen address for this node. e.g. '127.0.0.1:8000' or ':8000' for where updates are accepted from",
			EnvVar: "address, a",
		},
		cli.StringFlag{
			Name:   "name, n",
			Value:  globalSettings.Name,
			Usage:  "Specify a name for this node. e.g. 'NodeA' or 'NodeB'",
			EnvVar: "name, n",
		},
		cli.StringFlag{
			Name:   "bootstrap_address, ba",
			Value:  globalSettings.BootstrapAddress,
			Usage:  "Specify a bootstrap address. e.g. '10.10.10.10:12345'",
			EnvVar: "bootstrap_address, ba",
		},
	}

	app.Run(os.Args)
}
