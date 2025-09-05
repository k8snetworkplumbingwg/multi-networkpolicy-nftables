package main

import (
	"flag"
	"fmt"
	"os"

	multinetworkscheme "github.com/k8snetworkplumbingwg/multi-networkpolicy/pkg/client/clientset/versioned/scheme"
	netdefscheme "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned/scheme"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	nodeutil "k8s.io/component-helpers/node/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/marguerr/multi-network-policy-nftables/pkg/controller"
	"github.com/marguerr/multi-network-policy-nftables/pkg/cri"
	"github.com/marguerr/multi-network-policy-nftables/pkg/datastore"
	"github.com/marguerr/multi-network-policy-nftables/pkg/nftables"
	"github.com/marguerr/multi-network-policy-nftables/pkg/utils"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(multinetworkscheme.AddToScheme(scheme))
	utilruntime.Must(netdefscheme.AddToScheme(scheme))
}

func main() {
	var hostnameOverride string
	var networkPlugins string
	var criEndpoint string
	var hostPrefix string
	var acceptICMP bool
	var acceptICMPv6 bool
	var customIPv4IngressRuleFile string
	var customIPv4EgressRuleFile string
	var customIPv6IngressRuleFile string
	var customIPv6EgressRuleFile string

	flag.StringVar(&hostnameOverride, "hostname-override", "", "The hostname to use for the node. If not set, the hostname will be determined by the node controller.")
	flag.StringVar(&networkPlugins, "network-plugins", "macvlan", "Comma-separated list of network plugins to be considered for network policies.")
	flag.StringVar(&criEndpoint, "container-runtime-endpoint", "", "Path to cri socket.")
	flag.StringVar(&hostPrefix, "host-prefix", "", "If non-empty, will use this string as prefix for host filesystem.")
	flag.BoolVar(&acceptICMP, "accept-icmp", false, "accept all ICMP traffic")
	flag.BoolVar(&acceptICMPv6, "accept-icmpv6", false, "accept all ICMPv6 traffic")
	flag.StringVar(&customIPv4IngressRuleFile, "custom-v4-ingress-rule-file", "", "custom rule file for IPv4 ingress")
	flag.StringVar(&customIPv4EgressRuleFile, "custom-v4-egress-rule-file", "", "custom rule file for IPv4 egress")
	flag.StringVar(&customIPv6IngressRuleFile, "custom-v6-ingress-rule-file", "", "custom rule file for IPv6 ingress")
	flag.StringVar(&customIPv6EgressRuleFile, "custom-v6-egress-rule-file", "", "custom rule file for IPv6 egress")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog.Info("Starting multi-network-policy-nftables")

	hostname, err := nodeutil.GetHostname(hostnameOverride)
	if err != nil {
		setupLog.Error(err, "unable to get hostname")
		os.Exit(1)
	}

	if criEndpoint == "" {
		setupLog.Error(fmt.Errorf("container runtime endpoint is not set"), "unable to get container runtime endpoint")
		os.Exit(1)
	}

	// Process network plugins flag
	plugins, err := utils.ParseCommaSeparatedList(networkPlugins)
	if err != nil {
		setupLog.Error(err, "unable to parse network plugins")
		os.Exit(1)
	}

	if len(plugins) == 0 {
		setupLog.Error(fmt.Errorf("no network plugins provided"), "unable to parse network plugins")
		os.Exit(1)
	}

	setupLog.Info("Valid network plugins", "plugins", plugins)

	// Get custom nftables rules
	commonRules, err := getCustomRules(customIPv4IngressRuleFile, customIPv4EgressRuleFile, customIPv6IngressRuleFile, customIPv6EgressRuleFile)
	if err != nil {
		setupLog.Error(err, "unable to get custom nftables rules")
		os.Exit(1)
	}

	// Set ICMP acceptance rules
	commonRules.AcceptICMP = acceptICMP
	commonRules.AcceptICMPv6 = acceptICMPv6

	ctx := ctrl.SetupSignalHandler()

	criRuntime := cri.New(criEndpoint, hostPrefix)
	if err := criRuntime.Connect(ctx); err != nil {
		setupLog.Error(err, "unable to connect to CRI runtime")
		os.Exit(1)
	}
	defer criRuntime.Close(ctx)

	// Create manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	ds := &datastore.Datastore{
		Policies: make(map[types.NamespacedName]*datastore.Policy),
	}

	nft := &nftables.NFTables{
		Client:      mgr.GetClient(),
		Hostname:    hostname,
		CriRuntime:  criRuntime,
		CommonRules: commonRules,
	}

	if err = (&controller.MultiNetworkReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		DS:           ds,
		NFT:          nft,
		ValidPlugins: plugins,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MultiNetwork")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err = mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// getCustomRules reads custom nftables rules from the provided files and returns a CommonRules struct
func getCustomRules(customIPv4IngressRuleFile, customIPv4EgressRuleFile, customIPv6IngressRuleFile, customIPv6EgressRuleFile string) (*nftables.CommonRules, error) {
	commonRules := &nftables.CommonRules{}

	if customIPv4IngressRuleFile != "" {
		rules, err := utils.ReadRulesFromFile(customIPv4IngressRuleFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read custom IPv4 ingress rules from file: %w", err)
		}
		commonRules.CustomIPv4IngressRules = rules
	}

	if customIPv4EgressRuleFile != "" {
		rules, err := utils.ReadRulesFromFile(customIPv4EgressRuleFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read custom IPv4 egress rules from file: %w", err)
		}
		commonRules.CustomIPv4EgressRules = rules
	}

	if customIPv6IngressRuleFile != "" {
		rules, err := utils.ReadRulesFromFile(customIPv6IngressRuleFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read custom IPv6 ingress rules from file: %w", err)
		}
		commonRules.CustomIPv6IngressRules = rules
	}

	if customIPv6EgressRuleFile != "" {
		rules, err := utils.ReadRulesFromFile(customIPv6EgressRuleFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read custom IPv6 egress rules from file: %w", err)
		}
		commonRules.CustomIPv6EgressRules = rules
	}

	return commonRules, nil
}
