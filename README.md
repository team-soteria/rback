# rback

A simple "RBAC in Kubernetes" visualizer. No matter how complex the setup, `rback` queries all RBAC related information of an Kubernetes cluster in constant time and generates a graph representation of service accounts, (cluster) roles, and the respective access rules in [dot](https://www.graphviz.org/doc/info/lang.html) format.

For example, here is an Amazon EKS cluster as seen by `rback`:

![EKS cluster](examples/eks.dot.png)

Another example would be a local K3S cluster:

![K3S cluster](examples/k3s.dot.png)

## Install

`rback` depends on you having access to a Kubernetes cluster, either in the cloud (like Amazon EKS)
or locally (k3s, kind, Minikube, Docker for Desktop) as well as  `kubectl` installed and configured, locally.


To install it for macOS, do:

```sh
$ curl -sL https://github.com/mhausenblas/rback/releases/download/v0.1.0/macos_rback -o rback
$ chmod +x rback && sudo mv rback /usr/local/bin
```

To install it for Linux, do:

```sh
$ curl -sL https://github.com/mhausenblas/rback/releases/download/v0.1.0/linux_rback -o rback
$ chmod +x rback && sudo mv rback /usr/local/bin
```


You can also build it from source, with Go 1.12 like so:

```sh
$ git clone https://github.com/mhausenblas/rback.git && cd rback
$ go build
```

## Usage

Run `rback` locally against the target cluster and store its output in a `.dot` file like shown in the following:

```sh
$ rback > result.dot
```

Now that you have `result.dot`, you can render the graph either online or locally.

### Render online

There are plenty of Graphviz (`dot`) online visualization tools available, for example [http://magjac.com/graphviz-visual-editor/](http://magjac.com/graphviz-visual-editor/) or [dreampuf.github.io/GraphvizOnline](https://dreampuf.github.io/GraphvizOnline/). Head over there and paste the output of `rbac` into it.

### Render locally

Install [Graphviz](https://www.graphviz.org/), for example, on macOS you can do `brew install graphviz`. Then you can do the following (on macOS):

```sh
$ rback | dot -Tpng  > /tmp/rback.png && open /tmp/rback.png
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