package main

import (
	goflag "flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/spf13/pflag"

	"github.com/caicloud/kube-storage-monitor/cmd/kube_storage_monitor/app"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	command := app.NewMonitorServerCommand()

	// TODO: once we switch everything over to Cobra commands, we can go back to calling
	// utilflag.InitFlags() (by removing its pflag.Parse() call). For now, we have to set the
	// normalize func and add the go flag set by hand.
	pflag.CommandLine.SetNormalizeFunc(func(f *pflag.FlagSet, name string) pflag.NormalizedName {
		if strings.Contains(name, "_") {
			return pflag.NormalizedName(strings.Replace(name, "_", "-", -1))
		}
		return pflag.NormalizedName(name)
	})
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)
	// utilflag.InitFlags()

	if err := command.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

}
