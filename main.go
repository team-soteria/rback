package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/emicklei/dot"
	"github.com/mhausenblas/kubecuddler"
)

type RnB struct {
	Roles               []string
	ClusterRoles        string
	RoleBindings        string
	ClusterRoleBindings string
}

func main() {
	rnb, err := getRolesNBindings()
	if err != nil {
		fmt.Printf("Can't list roles and bindings due to :%v", err)
		os.Exit(-1)
	}
	g := genGraph(rnb)
	fmt.Println(g.String())
}

func getRolesNBindings() (RnB, error) {
	var rnb RnB
	res, err := kubecuddler.Kubectl(true, true, "", "get", "roles", "--all-namespaces", "--no-headers")
	if err != nil {
		return rnb, err
	}
	for _, r := range strings.Split(res, "\n") {
		f := strings.Fields(r)
		rnb.Roles = append(rnb.Roles, fmt.Sprintf("ns %v name %v", f[0], f[1]))
	}

	return rnb, nil
}

func genGraph(rnb RnB) *dot.Graph {
	g := dot.NewGraph(dot.Directed)
	groles := g.Subgraph("roles")
	for _, r := range rnb.Roles {
		groles.Node(r)
	}
	return g
}
