# rback

A simple "RBAC in Kubernetes" visualizer. No matter how complex the setup, `rback` queries all RBAC related information of an Kubernetes cluster in constant time and generates a graph representation of service accounts, (cluster) roles, and the respective access rules in [dot](https://www.graphviz.org/doc/info/lang.html) format.

For example, here is an Amazon EKS cluster as seen by `rback`:

![EKS cluster](examples/eks.dot.png)

Another example would be a local K3S cluster:

![K3S cluster](examples/k3s.dot.png)

## Install

Dependencies:

- Access to a Kubernetes cluster
- `kubectl` installed and configured

For now, no binaries, use from source with Go 1.12:

```sh
$ git clone https://github.com/mhausenblas/rback.git && cd rback
$ go build
```

## Usage

Run `rback` locally against the target cluster and store its output in a `.dot` file. Then you can render the graph either online or locally.

### Render online

There are plenty of Graphviz (`dot`) online visualization tools available, for example [dreampuf.github.io/GraphvizOnline](https://dreampuf.github.io/GraphvizOnline/). Head over there and paste the output of `rbac` into it
.

### Render locally

Install [Graphviz](https://www.graphviz.org/), for example, on macOS you can do `brew install graphviz`. Then you can do the following (on macOS):

```sh
$ rback | dot -Tpng  > /tmp/rback.png && open /tmp/rback.png
```

## Background

How it works is that `rback` issues the following five queries:

```sh
kubectl get sa --all-namespaces --output json
kubectl get roles --all-namespaces --output json
kubectl get rolebindings --all-namespaces --output json
kubectl get clusterroles --output json
kubectl get clusterrolebindings --output json
```

Based on this information, the graphs are created.