---
title: Deployment
description: Learn to deploy Chall-Manager, either for production or development purposes.
weight: 1
categories: [How-to Guides]
tags: [Kubernetes, Infrastructure, Infra as Code]
resources:
- src: "**.png"
---

You can deploy the Chall-Manager in many ways.
The following table summarize the properties of each one.

| Name | Maintained | Isolation | Scalable | Janitor |
|---|:---:|:---:|:---:|:---:|
| [Kubernetes (with Pulumi)](#kubernetes-with-pulumi) | ✅ | ✅ | ✅ | ✅ |
| [Kubernetes](#kubernetes) | ❌ | ✅ | ✅ | ✅ |
| [Docker](#docker) | ❌ | ✅ | ✅¹ | ❌² |
| [Binary](#binary) | ❌ | ❌ | ❌ | ❌² |

¹ Autoscaling is possible with an hypervisor (e.g. Docker Swarm).

² Cron could be configured through a cron on the host machine.

## Kubernetes (with Pulumi)

{{< alert title="Note" color="primary" >}}
We **highly recommend** the use of this deployment strategy.

We use it to [test the chall-manager](/docs/chall-manager/design/testing), and will ease parallel deployments.
{{< /alert >}}

This deployment strategy guarantee you a valid infrastructure regarding our functionalities and security guidelines.
Moreover, if you are afraid of Pulumi you'll have trouble [creating scenarios](/docs/chall-manager/challmaker-guides/create-scenario), so it's a good place to start !

The requirements are:
- a distributed block storage solution such as [Longhorn](https://longhorn.io), if you want replicas.
- an [etcd](https://etcd.io) cluster, if you want to scale.
- an [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/), if you want telemetry data.
- an _origin_ namespace in which the chall-manager will run.

```bash
# Get the repository and its own Pulumi factory
git clone git@github.com:ctfer-io/chall-manager.git
cd chall-manager/deploy

# Use it straightly !
# Don't forget to configure your stack if necessary.
# Refer to Pulumi's doc if necessary.
pulumi up
```

Now, you're done !

{{< imgproc infrastructure Fit "800x800" >}}
Micro Services Architecture of chall-manager deployed in a Kubernetes cluster.
{{< /imgproc >}}

## Kubernetes

With this deployment strategy, you are embracing the hard path of setting up a chall-manager to production.
You'll have to handle the functionalities, the security, and you won't implement variability easily.
We still highly recommend you [deploying with Pulumi](#kubernetes-with-pulumi), but if you love YAMLs, here is the doc.

The requirements are:
- a distributed block storage solution such as [Longhorn](https://longhorn.io), if you want replicas.
- an [etcd](https://etcd.io) cluster, if you want to scale.
- an [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/), if you want telemetry data.
- an _origin_ namespace in which the chall-manager will run.

We'll deploy the following:
- a _target_ **Namespace** to deploy instances into.
- a **ServiceAccount** for chall-manager to deploy instances in the _target_ namespace.
- a **Role** and its **RoleBinding** to assign permissions required for the ServiceAccount to deploy resources. Please do not give non-namespaced permissions as it would enable a CTF player to pivot into the cluster thus break isolation.
- a set of 4 **NetworkPolicies** to ensure security by default in the _target_ namespace.
- a **PersistentVolumeClaim** to replicate the Chall-Manager filesystem data among the replicas.
- a **Deployment** for the Chall-Manager pods.
- a **Service** to expose those Chall-Manager pods, required for the janitor and the CTF platform communications.
- a **CronJob** for the Chall-Manager-Janitor.

First of all, we are working on the target namespace the chall-manager will deploy challenge instances to.
Those steps are mandatory in order to obtain a secure and robust deployment, without players being able to pivot in your Kubernetes cluster, thus to your applications (databases, monitoring, etc.).

The first step is to create the _target_ namespace.

{{< card code=true header="`target-namespace.yaml`" lang="yaml" >}}
apiVersion: v1
kind: Namespace
metadata:
  name: target-ns
{{< /card >}}

To deploy challenge instances into this _target_ namespace, we are going to need 3 resources: the **ServiceAccount**, the **Role** and its **RoleBinding**.
This ServiceAccount should not be shared with other applications, and we are here detailing 1 way to build its permissions. As a Kubernetes administrator, you can modify those steps to aggregate roles, create a Cluster-wide Role and RoleBindng, etc. Nevertheless, we trust our documented approach to be wiser for maintenance and accessibility.

Adjust the role permissions to your needs. You can do this using `kubectl api-resources –-namespaced=true –o wide`.

{{< card code=true header="`role.yaml`" lang="yaml" >}}
apiVersion: rbac.authorieation.k8s.io/v1
kind: Role
metadata:
  name: chall-manager-role
  namespace: target-ns
  labels:
    app: chall-manager
rules:
- apiGroups:
  - ""
  resources:
  - configmaps
  - endpoints
  - persistentvolumeclaims
  - pods
  - resourcequotas
  - secrets
  - service
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - apps
  resources:
  - deployments
  - replicasets
  - statefulsets
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - batch
  resources:
  - cronjobs
  - jobs
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - networking.k8s.io
  resources:
  - ingresses
  - networkpolicies
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
{{< /card >}}

The the `ServiceAccount` it will refer to.

{{< card code=true header="`service-account.yaml`" lang="yaml" >}}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: chall-manager-sa
  metadata: source-ns
  labels:
    app: chall-manager
{{< /card >}}

Finally, bind the `Role` and `ServiceAccount`.

{{< card code=true header="`role-binding.yaml`" lang="yaml" >}}
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: chall-manager-role-binding
  namespace: target-ns
  labels:
    app: chall-manager
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: chall-manager-role
subjects:
- kind: ServiceAccount
  name: chall-manager-sa
  namespace: source-ns
{{< /card >}}

Now, we will prepare isolation of scenarios to avoid pivoting in the infrastructure.

First, we start by denying all networking.

{{< card code=true header="`netpol-deny-all.yaml`" lang="yaml" >}}
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: netpol-deny-all
  namespace: target-ns
  labels:
    app: chall-manager
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress
{{< /card >}}

Then, make sure that intra-cluster communications are not allowed from this namespace to any other.

{{< card code=true header="`netpol-inter-ns.yaml`" lang="yaml" >}}
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: netpol-inter-ns
  namespace: target-ns
  labels:
    app: chall-manager
spec:
  egress:
  - to:
    - namespaceSelector:
        matchExpressions:
        - key: kubernetes.io/metadata.name
          operator: NotIn
          values:
          - target-ns
  podSelector: {}
  policyTypes:
  - Egress
{{< /card >}}

For complex [scenarios](/docs/chall-manager/glossary#scenario) that require multiple pods, we need to be able to resolve intra-cluster DNS entries.

{{< card code=true header="`netpol-dns.yaml`" lang="yaml" >}}
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: netpol-dns
  namespace: target-ns
  labels:
    app: chall-manager
spec:
  egress:
  - ports:
    - port: 53
      protocol: UDP
    - port: 53
      protocol: TCP
    to:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: kube-system
      podSelector:
        matchLabels:
          k8s-app: kube-dns
  podSelector: {}
  policyTypes:
  - Egress
{{< /card >}}

Ultimately our challenges will probably need to access internet, or our players will operate online, so we need to grant access to internet addresses.

{{< card code=true header="`netpol-internet.yaml`" lang="yaml" >}}
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: netpol-internet
  namespace: target-ns
  labels:
    app: chall-manager
spec:
  egress:
  - to:
    - ipBlock:
        cidr: 0.0.0.0/0
        expect:
        - 10.0.0.0/8
        - 172.16.0.0/12
        - 192.168.0.0/16
  podSelector: {}
  policyTypes:
  - Egress
{{< /card >}}

At this step, no communication will be accepted by the _target_ namespace. Every scenario will need to define its own `NetworkPolicies` regarding its inter-pods and exposed services communications.

Before starting the chall-manager, we need to create the `PersistentVolumeClaim` to write the data to.

{{< card code=true header="`pvc.yaml`" lang="yaml" >}}
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: chall-manager-pvc
  namespace: source-ns
  labels:
    app: chall-manager
spec:
  accessModes:
  - ReadWriteMany
  resources:
    requests:
      storage: 2Gi # arbitrary, you may need more or less
  storageClassName: longhorn # or anything else compatible
  volumeName: chall-manager-pvc
{{< /card >}}

We'll now deploy the chall-manager and provide it the `ServiceAccount` we created before.
For additionnal configuration elements, refer to the CLI documentation (`chall-manager -h`).

{{< card code=true header="`deployment.yaml`" lang="yaml" >}}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: chall-manager-deploy
  namespace: source-ns
  labels:
    app: chall-manager
spec:
  replicas: 1 # scale if necessary
  selector:
    matchLabels:
      app: chall-manager
  template:
    metadata:
      namespace: source-ns
      labels:
        app: chall-manager
    spec:
      containers:
      - name: chall-manager
        image: ctferio/chall-manager:v1.0.0
        env:
        - name: PORT
          value: "8080"
        - name: DIR
          value: /etc/chall-manager
        - name: LOCK_KIND
          value: local # or "etcd" if you have an etcd cluster
        - name: KUBERNETES_NAMESPACE
          value: target-ns
        ports:
        - name: grpc
          containerPort: 8080
          protocol: TCP
        volumeMounts:
        - name: dir
          mountPath: /etc/chall-manager
      serviceAccount: chall-manager-sa
      # if you have an etcd cluster, we recommend creating an InitContainer to wait for the cluster to be up and running before starting chall-manager, else it will fail to handle requests
      volumes:
      - name: dir
        persistentVolumeClaim:
          claimName: chall-manager-pvc
{{< /card >}}

We need to expose the pods to integrate chall-manager with a CTF platform, and to enable the janitor to run.

{{< card code=true header="`service.yaml`" lang="yaml">}}
apiVersion: v1
kind: Service
metadata:
  name: chall-manager-svc
  namespace: source-ns
  labels:
    app: chall-manager
spec:
  ports:
  - name: grpc
    port: 8080
    targePort: 8080
    protocol: TCP
  # if you are using the chall-manager gateway (its REST API), don't forget to add an entry here
  selector:
    app: chall-manager
{{< /card >}}

Now, to enable the janitoring, we have to create the `CronJob` for the `chall-manager-janitor`.

{{< card code=true header="`cronjob.yaml`" lang="yaml" >}}
apiVersion: batch/v1
kind: CronJob
metadata:
  name: chall-manager-janitor
  namespace: source-ns
  labels:
    app: chall-manager
spec:
  schedule: "*/1 * * * *" # run every minute ; configure it elseway if necessary
  jobTemplate:
    spec:
      template:
        metadata:
          namespace: source-ns
          labels:
            app: chall-manager
        spec:
          containers:
            - name: chall-manager-janitor
              image: ctferio/chall-manager-janitor:v1.0.0
              env: 
              - name: URL
                value: chall-manager-svc:8080
{{< /card >}}

Finally, deploy them all.

```bash
kubectl apply -f target-namespace.yaml \
  -f role.yaml -f service-account.yaml -f role-binding.yaml \
  -f netpol-deny-all.yaml -f netpol-inter-ns.yaml -f netpol-dns.yaml -f netpol-internet.yaml \
  -f pvc.yaml -f deployment.yaml -f service.yaml \
  -f cronjob.yaml
```

## Docker

{{< alert title="Warning" color="warning" >}}
This mode does not cover the deployment of the janitor.
{{< /alert >}}

To deploy the docker container on a host machine, run the following.
It will come with a limited set of features, thus will need additional configurations for the Pulumi providers to communicate with their targets.

```bash
docker run -p 8080:8080 -v ./data:/etc/chall-manager ctferio/chall-manager:v1.0.0
```

For the janitor, you may use a cron service on your host machine.
In this case, you may also want to create a specific network to isolate them from other adjacent services.

## Binary

{{< alert title="Security" color="warning" >}}
We highly discourage the use of this mode as it does not guarantee proper isolation.
The chall-manager is basically a RCE-as-a-Service carrier, so if you run this on your host machine, prepare for dramatic issues.
{{< /alert >}}

{{< alert title="Warning" color="warning" >}}
This mode does not cover the deployment of the janitor.
{{< /alert >}}

To deploy the binary on a host machine, run the following.
It will come with a limited set of features, thus will need additional configurations for the Pulumi providers to communicate with their targets.

```bash
# Download the binary from https://github.com/ctfer-io/chall-manager/releases, then run it
./chall-manager
```

For the janitor, you may use a cron service on your host machine.
