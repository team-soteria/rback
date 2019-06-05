# rback

A simple "RBAC in Kubernetes" visualizer. No matter how complex the setup, `rback` queries all RBAC related information of an Kubernetes cluster in constant time and generates a graph representation of service accounts, (cluster) roles, and the respective access rules in [dot](https://www.graphviz.org/doc/info/lang.html) format.

For example, here is an Amazon EKS cluster as seen by `rback`:

![EKS cluster](examples/eks.dot.png)

Another example would be a local K3S cluster:

![K3S cluster](examples/k3s.dot.png)

Here in action in the [Katacoda Kubernetes playground](https://www.katacoda.com/courses/kubernetes/playground):

![Katacoda](examples/katacoda.dot.png)

See for more details the [examples/](examples/) directory …

## Install

`rback` depends on you having access to a Kubernetes cluster, either in the cloud (like Amazon EKS)
or locally (k3s, kind, Minikube, Docker for Desktop) as well as  `kubectl` installed and configured, locally.


To install it for macOS, do:

```sh
$ curl -sL https://github.com/team-soteria/rback/releases/download/v0.3.0/macos_rback -o rback
$ chmod +x rback && sudo mv rback /usr/local/bin
```

To install it for Linux, do:

```sh
$ curl -sL https://github.com/team-soteria/rback/releases/download/v0.3.0/linux_rback -o rback
$ chmod +x rback && sudo mv rback /usr/local/bin
```


You can also build it from source, with Go 1.12 like so:

```sh
$ git clone https://github.com/mhausenblas/rback.git && cd rback
$ go build
```

## Using rback directly

Run `rback` locally against the target cluster and store its output in a `.dot` file like shown in the following:

```sh
$ kubectl get sa,roles,rolebindings,clusterroles,clusterrolebindings --all-namespaces -o json | rback > result.dot
```

Now that you have `result.dot`, you can render the graph either online or locally.

### Render online

There are plenty of Graphviz (`dot`) online visualization tools available, for example, use [magjac.com/graphviz-visual-editor/](http://magjac.com/graphviz-visual-editor/) for interaction or the simpler [dreampuf.github.io/GraphvizOnline](https://dreampuf.github.io/GraphvizOnline/). Head over there and paste the output of `rback` into it.

### Render locally

Install [Graphviz](https://www.graphviz.org/), for example, on macOS you can do `brew install graphviz`. Then you can do the following (on macOS):

```sh
$ kubectl get sa,roles,rolebindings,clusterroles,clusterrolebindings --all-namespaces -o json | rback | dot -Tpng  > /tmp/rback.png && open /tmp/rback.png
```


## Using Rback as a kubectl plugin

There is also a very crude first version of a kubectl plugin in https://github.com/team-soteria/rback/blob/master/kubectl-plugin/kubectl-rback. Add the file to your path, ensure it is executable and modify it to suit your environment. Then, you'll be able to simply run:
```sh
$ kubectl rback
```
This will generate the `.dot` file, render it using GraphViz (must be installed on your system) and open the rendered image using `xgd-open`. 

We welcome contributions to make the plugin work in other environments.

## More usage examples

By default, `rback` shows all RBAC resources in your cluster, but you can also focus on a single namespace by using the `-n` switch. The switch supports multiple namespaces as well:
```sh
$ kubectl rback -n my-namespace
$ kubectl rback -n my-namespace1,my-namespace2
```

If you're particularly interested in a single `ServiceAccount`, you can run:
```sh
$ kubectl rback serviceaccount my-service-account
or
$ kubectl rback sa my-service-account
```
This makes the specified `ServiceAccount` the focal point of the graph, meaning that only it and directly-related RBAC resources are shown. 

Instead of `ServiceAccounts`, you can also focus on `Roles`, `RoleBindings`, `ClusterRoles` or `ClusterRoleBindings`:
```sh
$ kubectl rback role my-role
$ kubectl rback clusterrole my-cluster-role
$ kubectl rback rolebinding my-role-binding
$ kubectl rback clusterrolebinding my-cluster-role-binding
```
You can also use the abbreviated form:
```sh
$ kubectl rback r my-role
$ kubectl rback cr my-cluster-role
$ kubectl rback rb my-role-binding
$ kubectl rback crb my-cluster-role-binding
```

If you'd like to inspect more than one resource, you can specify multiple resource names:
```sh
$ kubectl rback r my-role1 my-role2
```

In addition to focusing on a specific resource, `rback` can also show you who can perform a particular action. For example, if you'd like to see who can create pods, run:
```sh
$ kubectl rback who-can create pods
```
This renders the matched `(Cluster)Roles`, all directly-related `(Cluster)RoleBindings` and subjects (`ServiceAccounts`, `Users` and `Groups`). The matched access rule will be shown in bold font. 

Whether using `who-can` or not, you can turn off the rendering of the (possibly long) list of access rules with:
```sh
$ kubectl rback --show-rules=false
```

When using `who-can`, you can also tell `rback` to only show matched rules instead of hiding rules completely:
```sh
$ kubectl rback --show-matched-rules-only who-can create pods
```

## Background

How it works is that `rback` issues the following five queries by shelling out to `kubectl`:

```sh
kubectl get sa --all-namespaces --output json
kubectl get roles --all-namespaces --output json
kubectl get rolebindings --all-namespaces --output json
kubectl get clusterroles --output json
kubectl get clusterrolebindings --output json
```

Then, based on this information, the graphs are created using the [github.com/emicklei/dot](https://github.com/emicklei/dot) package.
