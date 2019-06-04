package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

type Rback struct {
	config      Config
	permissions Permissions
}

type Config struct {
	inputFile       string
	showRules       bool
	showLegend      bool
	namespaces      []string
	ignoredPrefixes []string
	resourceKind    string
	resourceNames   []string
	whoCan          WhoCan
}

type WhoCan struct {
	verb, resourceKind, resourceName string
	showMatchedOnly                  bool
}

func main() {
	config := parseConfigFromArgs()
	rback := Rback{config: config}

	var err error
	reader := os.Stdin
	if config.inputFile != "" {
		reader, err = os.Open(config.inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Can't open file %s: %v\n", config.inputFile, err)
			os.Exit(-1)
		}
	}

	err = rback.parseRBAC(reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't parse RBAC resources from stdin: %v\n", err)
		os.Exit(-1)
	}
	g := rback.genGraph()
	fmt.Println(g.String())
}

func parseConfigFromArgs() Config {
	config := Config{}
	flag.StringVar(&config.inputFile, "f", "", "The name of the file to use as input (otherwise stdin is used)")
	flag.BoolVar(&config.showLegend, "show-legend", true, "Whether to show the legend or not")
	flag.BoolVar(&config.showRules, "show-rules", true, "Whether to render RBAC access rules (e.g. \"get pods\") or not")
	flag.BoolVar(&config.whoCan.showMatchedOnly, "show-matched-rules-only", false, "When running who-can, only show the matched rule instead of all rules specified in the role")

	var namespaces string
	flag.StringVar(&namespaces, "n", "", "The namespace to render (also supports multiple, comma-delimited namespaces)")

	var ignoredPrefixes string
	flag.StringVar(&ignoredPrefixes, "ignore-prefixes", "system:", "Comma-delimited list of (Cluster)Role(Binding) prefixes to ignore ('none' to not ignore anything)")
	flag.Parse()

	if flag.NArg() > 0 {
		if flag.Arg(0) == "who-can" {
			if flag.NArg() < 3 {
				fmt.Println("Usage: rback who-can VERB RESOURCE [NAME]")
				os.Exit(-4)
			}
			config.resourceKind = kindRule
			config.whoCan.verb = flag.Arg(1)
			config.whoCan.resourceKind = flag.Arg(2)
			if flag.NArg() > 3 {
				config.whoCan.resourceName = flag.Arg(3)
			}
		} else {
			config.resourceKind = normalizeKind(flag.Arg(0))
			if flag.NArg() > 1 {
				config.resourceNames = flag.Args()[1:]
			}
		}
	}

	config.namespaces = strings.Split(namespaces, ",")

	if ignoredPrefixes != "none" {
		config.ignoredPrefixes = strings.Split(ignoredPrefixes, ",")
	}
	return config
}

const (
	kindServiceAccount     = "serviceaccount"
	kindRoleBinding        = "rolebinding"
	kindClusterRoleBinding = "clusterrolebinding"
	kindRole               = "role"
	kindClusterRole        = "clusterrole"
	kindUser               = "user"
	kindGroup              = "group"
	kindRule               = "rule" // internal kind used for nodes that list access rules defined in a role
)

var kindMap = map[string]string{
	"sa":                  kindServiceAccount,
	"serviceaccounts":     kindServiceAccount,
	"rb":                  kindRoleBinding,
	"rolebindings":        kindRoleBinding,
	"crb":                 kindClusterRoleBinding,
	"clusterrolebindings": kindClusterRoleBinding,
	"r":                   kindRole,
	"roles":               kindRole,
	"cr":                  kindClusterRole,
	"clusterroles":        kindClusterRole,
	"u":                   kindUser,
	"users":               kindUser,
	"g":                   kindGroup,
	"groups":              kindGroup,
}

func normalizeKind(kind string) string {
	kind = strings.ToLower(kind)
	entry, exists := kindMap[kind]
	if exists {
		return entry
	}
	return kind
}
