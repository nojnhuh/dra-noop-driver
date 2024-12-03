package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"
	coreclientset "k8s.io/client-go/kubernetes"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/klog/v2"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1alpha4"
	"sigs.k8s.io/dra-example-driver/pkg/flags"
)

const (
	DriverName = "noop.example.com"

	PluginRegistrationPath = "/var/lib/kubelet/plugins_registry/" + DriverName + ".sock"
	DriverPluginPath       = "/var/lib/kubelet/plugins/" + DriverName
	DriverPluginSocketPath = DriverPluginPath + "/plugin.sock"
)

type Flags struct {
	kubeClientConfig flags.KubeClientConfig
	loggingConfig    *flags.LoggingConfig

	nodeName string
}

type Config struct {
	flags      *Flags
	coreclient coreclientset.Interface
}

func main() {
	if err := newApp().Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newApp() *cli.App {
	flags := &Flags{
		loggingConfig: flags.NewLoggingConfig(),
	}
	cliFlags := []cli.Flag{
		&cli.StringFlag{
			Name:        "node-name",
			Usage:       "The name of the node to be worked on.",
			Required:    true,
			Destination: &flags.nodeName,
			EnvVars:     []string{"NODE_NAME"},
		},
	}
	cliFlags = append(cliFlags, flags.kubeClientConfig.Flags()...)
	cliFlags = append(cliFlags, flags.loggingConfig.Flags()...)

	app := &cli.App{
		Name:            "dra-noop-kubeletplugin",
		Usage:           "dra-noop-kubeletplugin implements a DRA driver plugin.",
		ArgsUsage:       " ",
		HideHelpCommand: true,
		Flags:           cliFlags,
		Before: func(c *cli.Context) error {
			if c.Args().Len() > 0 {
				return fmt.Errorf("arguments not supported: %v", c.Args().Slice())
			}
			return flags.loggingConfig.Apply()
		},
		Action: func(c *cli.Context) error {
			ctx := c.Context
			clientSets, err := flags.kubeClientConfig.NewClientSets()
			if err != nil {
				return fmt.Errorf("create client: %v", err)
			}

			config := &Config{
				flags:      flags,
				coreclient: clientSets.Core,
			}

			return StartPlugin(ctx, config)
		},
	}

	return app
}

func StartPlugin(ctx context.Context, config *Config) error {
	err := os.MkdirAll(DriverPluginPath, 0750)
	if err != nil {
		return err
	}

	driver, err := NewDriver(ctx, config)
	if err != nil {
		return err
	}

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-sigc

	err = driver.Shutdown(ctx)
	if err != nil {
		klog.FromContext(ctx).Error(err, "Unable to cleanly shutdown driver")
	}

	return nil
}

type driver struct {
	plugin kubeletplugin.DRAPlugin
}

func NewDriver(ctx context.Context, config *Config) (*driver, error) {
	driver := &driver{}

	plugin, err := kubeletplugin.Start(
		ctx,
		driver,
		kubeletplugin.KubeClient(config.coreclient),
		kubeletplugin.NodeName(config.flags.nodeName),
		kubeletplugin.DriverName(DriverName),
		kubeletplugin.RegistrarSocketPath(PluginRegistrationPath),
		kubeletplugin.PluginSocketPath(DriverPluginSocketPath),
		kubeletplugin.KubeletPluginSocketPath(DriverPluginSocketPath))
	if err != nil {
		return nil, err
	}
	driver.plugin = plugin

	return driver, nil
}

func (d *driver) Shutdown(ctx context.Context) error {
	d.plugin.Stop()
	return nil
}

func (d *driver) NodePrepareResources(ctx context.Context, req *drapbv1.NodePrepareResourcesRequest) (*drapbv1.NodePrepareResourcesResponse, error) {
	klog.Infof("NodePrepareResource is called: number of claims: %d", len(req.Claims))
	preparedResources := &drapbv1.NodePrepareResourcesResponse{Claims: map[string]*drapbv1.NodePrepareResourceResponse{}}

	for _, claim := range req.Claims {
		preparedResources.Claims[claim.UID] = &drapbv1.NodePrepareResourceResponse{Devices: nil}
	}

	return preparedResources, nil
}

func (d *driver) NodeUnprepareResources(ctx context.Context, req *drapbv1.NodeUnprepareResourcesRequest) (*drapbv1.NodeUnprepareResourcesResponse, error) {
	klog.Infof("NodeUnPrepareResource is called: number of claims: %d", len(req.Claims))
	unpreparedResources := &drapbv1.NodeUnprepareResourcesResponse{Claims: map[string]*drapbv1.NodeUnprepareResourceResponse{}}

	for _, claim := range req.Claims {
		unpreparedResources.Claims[claim.UID] = &drapbv1.NodeUnprepareResourceResponse{}
	}

	return unpreparedResources, nil
}
