package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/emicklei/dot"
	"github.com/mhausenblas/kubecuddler"
)

type RnB struct {
	Roles               map[string][]string
	ClusterRoles        []string
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
	rnb := RnB{
		Roles:               make(map[string][]string),
		ClusterRoles:        []string{},
		RoleBindings:        "",
		ClusterRoleBindings: "",
	}
	// get all roles (namespaced):
	res, err := kubecuddler.Kubectl(true, true, "", "get", "roles", "--all-namespaces", "--no-headers")
	if err != nil {
		return rnb, err
	}
	for _, r := range strings.Split(res, "\n") {
		f := strings.Fields(r)
		rnb.Roles[f[0]] = append(rnb.Roles[f[0]], f[1])
	}
	// get all cluster roles:
	res, err = kubecuddler.Kubectl(true, true, "", "get", "clusterroles", "--no-headers")
	if err != nil {
		return rnb, err
	}
	for _, r := range strings.Split(res, "\n") {
		f := strings.Fields(r)
		rnb.ClusterRoles = append(rnb.ClusterRoles, f[0])
	}
	return rnb, nil
}

func genGraph(rnb RnB) *dot.Graph {
	g := dot.NewGraph(dot.Directed)
	for ns, names := range rnb.Roles {
		groles := g.Subgraph(ns, dot.ClusterOption{})
		for _, name := range names {
			groles.Node(name)
		}
	}
	gclusterroles := g.Subgraph("cluster-wide", dot.ClusterOption{})
	gclusterroles.Attr("color", "#2f6de1")
	for _, name := range rnb.ClusterRoles {
		gclusterroles.Node(name)
	}
	return g
}
