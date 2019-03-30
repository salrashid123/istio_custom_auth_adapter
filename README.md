# Simple Istio Mixer Out of Process Authorization Adapter

## Introduction

Sample Istio out of process Mixer Adapter that handles authorization checks.   Istio already ships with baseline [Authentication](https://istio.io/docs/concepts/security/#authentication) and [Authorization](https://istio.io/docs/concepts/security/#authorization) but users are free to inject custom authorization directly into the Mixer as a custom policy [Adapter](https://istio.io/docs/concepts/policies-and-telemetry/#adapters)

The idea behind this article is to setup an external (external to the mixer, that is) service which accepts header from an inbound request and then makes yes/no determination to allow the request through or not.

You can run the customer external adapter as a separate k8s Service or entirely external to the cluster.   For either case, the authorization server should validate the inbound request and secure its endpoint (this example uses just clear, unencrypted gRPC just as a demo).


## References

- https://istio.io/docs/concepts/policies-and-telemetry/#adapters
- https://istio.io/help/faq/mixer/
- https://github.com/istio/istio/wiki/Mixer-Out-Of-Process-Adapter-Walkthrough
- https://venilnoronha.io/set-sail-a-production-ready-istio-adapter
- https://istio.io/help/ops/setup/validation/



## Prerequsites

* On your local system:
  - go `1.11`
  - protoc ```libprotoc 3.6.1```
  - docker

* k8s cluster running any Istio (1.0+) sample app
  - [Istio "HelloWorld"](https://github.com/salrashid123/istio_helloworld)
    - I'd suggest following [these instructions](https://github.com/salrashid123/istio_helloworld#create-a-110-gke-cluster-and-bootstrap-istio)
  - [BookInfo](https://istio.io/docs/examples/bookinfo/)

>> Note, in `istio-1.1`, [policy checks are disabled](https://istio.io/docs/reference/config/installation-options/).

While setting up the cluster using the instructions above, set the value 
```
--set global.disablePolicyChecks=false
```

## Setup

The setup here is rather lengthy but pretty much follows [Mixer Out of Process Adapter Walkthrough](https://github.com/istio/istio/wiki/Mixer-Out-Of-Process-Adapter-Walkthrough).

One variation on that walkthrough is we will build off of an [authorization](https://istio.io/docs/reference/config/policy-and-telemetry/templates/) and not a `metrics` one as in the example.

Lets begin...so what does this app do?

Nothing much, this adapter just checks to see if a very specific header is sent into the request, thats all...its just for a demo:

```bash
curl -H "x-custom-token: abc" https://<istio_gateway_ip>/
```

### 1. Setup Environment Variables

First clone this MIXER_REPO
```
git clone https://github.com/salrashid123/istio_custom_auth_adapter.git
cd istio_custom_auth_adapter
export ROOT_FOLDER=`pwd`

mkdir src
export GOPATH=`pwd`
export MIXER_REPO=$GOPATH/src/istio.io/istio/mixer
export ISTIO=$GOPATH/src/istio.io
```


### 2. Get istio source

```
mkdir -p $GOPATH/src/istio.io/
cd $GOPATH/src/istio.io/
git clone https://github.com/istio/istio
```

### 3. Build mixer server,client binary

```
pushd $ISTIO/istio && make mixs
pushd $ISTIO/istio && make mixc
```

### 4. (optional) Copy and build Custom Adapter skeleton

```
mkdir $MIXER_REPO/adapter/mygrpcadapter/
cp  $ROOT_FOLDER/mygrpcadapter/mygrpcadapter_skel.go $MIXER_REPO/adapter/mygrpcadapter/mygrpcadapter.go
cd $MIXER_REPO/adapter/mygrpcadapter
go build ./...
```


### 5. Copy Configuration .proto

The `config.proto` file defines the operator specified runtime parameters.  In our case, it just holds the Header value to check for,

```proto
message Params {  
    // header value to check for
    string auth_key = 1;
}
```

```
mkdir -p $MIXER_REPO/adapter/mygrpcadapter/config
cp  $ROOT_FOLDER/mygrpcadapter/config/config.proto $MIXER_REPO/adapter/mygrpcadapter/config/config.proto
```

### 6. Copy Adapter implementation with build directives

`mygrpcadapter_impl.go` contains the actual business logic on what to do in this handler once the mixer passes through the header.

In our case, if the value doesn't match, just reject the request.

Note the `mygrpcadapter_impl.go` file now contains some directives at the very top:

```
// nolint:lll
// Generates the mygrpcadapter adapter's resource yaml. It contains the adapter's configuration, name, supported template
// names (metric in this case), and whether it is session or no-session based.
//go:generate $GOPATH/src/istio.io/istio/bin/mixer_codegen.sh -a mixer/adapter/mygrpcadapter/config/config.proto -x "-s=false -n mygrpcadapter -t authorization"
```

so lets copy the impl and build it

```
cp $ROOT_FOLDER/mygrpcadapter/mygrpcadapter_impl.go $MIXER_REPO/adapter/mygrpcadapter/mygrpcadapter.go
cd $MIXER_REPO/adapter/mygrpcadapter

go generate ./...
go build ./...
```

### 7. Copy and Stage the generated files


Lets copy the generated files and stage them so we can run local tests and use this folder to deploy to istio

```
mkdir -p $MIXER_REPO/adapter/mygrpcadapter/testdata
cp $MIXER_REPO/adapter/mygrpcadapter/config/mygrpcadapter.yaml $MIXER_REPO/adapter/mygrpcadapter/testdata
cp $MIXER_REPO/testdata/config/attributes.yaml $MIXER_REPO/adapter/mygrpcadapter/testdata
cp $MIXER_REPO/template/authorization/template.yaml $MIXER_REPO/adapter/mygrpcadapter/testdata
```

### 8. Create Adapter Starter

This app just launches the adapter gRPC server:

```
mkdir -p $MIXER_REPO/adapter/mygrpcadapter/cmd
cp  $ROOT_FOLDER/mygrpcadapter/cmd/main.go $MIXER_REPO/adapter/mygrpcadapter/cmd/
```

### 9. Copy Operator config

The `sample_operator_cfg.yaml` file contains the definition and configuration parameters for the mixer.

For example, it defines where the Adapter runs:

```yaml
apiVersion: "config.istio.io/v1alpha2"
kind: handler
metadata:
 name: h1
 namespace: istio-system
spec:
 adapter: mygrpcadapter
 connection:
   address: "[::]:44225"
   #address: "mygrpcadapterservice:44225"
   #address: "35.184.34.117:44225"
 params:
   auth_key: "abc"
```

In the sample above, the address is local (meaning for this demo, we will run the a mixer server and Adapter gRPC server on our laptop).
The config above also has commented `address:` values showing how to run the Adapter as a service within the k8s cluster or entire outside somewhere.

So copy the file over

```
cp  $ROOT_FOLDER/mygrpcadapter/testdata/sample_operator_cfg.yaml $MIXER_REPO/adapter/mygrpcadapter/testdata/sample_operator_cfg.yaml
```

---

### 10. Test Locally

Lets test the mixer and adapter locally


- Open two new terminals to `$ROOT_FOLDER` where you cloned this repo and set environment variables in each:

```
export GOPATH=`pwd`
export MIXER_REPO=$GOPATH/src/istio.io/istio/mixer
export ISTIO=$GOPATH/src/istio.io
```

### 11. Start local Adapter

This starts the local grpc Adapter

```
cd $MIXER_REPO/adapter/mygrpcadapter
go run cmd/main.go 44225
```

### 12. Start Local mixer server

```
$GOPATH/out/linux_amd64/release/mixs server \
    --configStoreURL=fs://$GOPATH/src/istio.io/istio/mixer/adapter/mygrpcadapter/testdata \
    --log_output_level=attributes:debug
```

You should see it 'Connect' to the gRPC server:

```
2018-12-12T17:53:25.757960Z	info	grpcAdapter	Connected to: 35.184.34.117:44225
2018-12-12T17:53:25.758001Z	info	Cleaning up handler table, with config ID:0
2018-12-12T17:53:25.758102Z	info	ccResolverWrapper: sending new addresses to cc: [{35.184.34.117:44225 0  <nil>}]
2018-12-12T17:53:25.758140Z	info	ClientConn switching balancer to "pick_first"
2018-12-12T17:53:25.758188Z	info	pickfirstBalancer: HandleSubConnStateChange: 0xc0001c0a20, CONNECTING
2018-12-12T17:53:25.788397Z	info	Starting monitor server...
Starting gRPC server on port 9091
2018-12-12T17:53:25.790644Z	info	ControlZ available at 172.31.140.231:9876
2018-12-12T17:53:25.801450Z	info	pickfirstBalancer: HandleSubConnStateChange: 0xc0001c0a20, READY

```

### 13. Sent direct request to mixer

Now use the mixer client to send a request to the mixer directly

```
$GOPATH/out/linux_amd64/release/mixc check -s destination.service="svc.cluster.local" --stringmap_attributes "request.headers=x-custom-token:abc"
```
You should see the mixer process the request since we enabled debug:

```
2018-12-12T16:59:56.691967Z	debug	attributes	Returning bag with attributes:
destination.service           : svc.cluster.local
request.headers               : {request.headers map[x-custom-token:abc] 0xc00c201620}
```

as well as on the Adapter:

```
$ $GOPATH/out/linux_amd64/release/mixc check -s destination.service="svc.cluster.local" --stringmap_attributes "request.headers=x-custom-token:abc"

Check RPC completed successfully. Check status was OK
  Valid use count: 10000, valid duration: 1m0s
  Referenced Attributes
    context.reporter.kind ABSENCE
    destination.namespace ABSENCE
    request.headers::x-custom-token EXACT

```


Great!..the mixer and adapter


Now send no header or incorrect header value:

```
$ $GOPATH/out/linux_amd64/release/mixc check -s destination.service="svc.cluster.local" --stringmap_attributes "request.headers=x-custom-token:fooooo"

Check RPC completed successfully. Check status was PERMISSION_DENIED (h1.handler.istio-system:Unauthorized...)
  Valid use count: 0, valid duration: 0s
  Referenced Attributes
    context.reporter.kind ABSENCE
    destination.namespace ABSENCE
    request.headers::x-custom-token EXACT
```

ok now that we have it running locally, lets generate and run this 'in-cluster' as a Service


### 14. Create a Adapter docker image

Lets create a docker image that encapsulates the Adapter:

First create a dockerhub login and repo (or you can use mine above):

so,

```
cd $ROOT_FOLDER

docker build -t salrashid123/mygrpcadapter .
docker push salrashid123/mygrpcadapter
```

(ofcourse replace the repo with your own if you want)


NOTE: you have two choices here:  you can deploy the adapter as a k8s cluster service or as a stand-alone service.

### 15a.  Deploying Adapter as Cluster Service

In this mode, we will wrap the Adapter as a cluster service called `mygrpcadapterservice`:

- cluster_service.yaml:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: mygrpcadapterservice
  namespace: istio-system
  labels:
    app: mygrpcadapter
spec:
  type: ClusterIP
  ports:
  - name: grpc
    protocol: TCP
    port: 44225
    targetPort: 44225
  selector:
    app: mygrpcadapter
---
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: mygrpcadapter
  namespace: istio-system
  labels:
    app: mygrpcadapter
spec:
  replicas: 1
  template:
    metadata:
      labels:
        app: mygrpcadapter
      annotations:
        sidecar.istio.io/inject: "false"
        scheduler.alpha.kubernetes.io/critical-pod: ""
    spec:
      containers:
      - name: mygrpcadapter
        image: salrashid123/mygrpcadapter:latest
        imagePullPolicy: Always
        ports:
        - containerPort: 44225
```

now deploy the Adapter as a service:

```
kubectl apply -f $ROOT_FOLDER/cluster_service.yaml
```

### 15b.  Deploy as stand alone remote server

In this mode, the adapter runs entirely standalone outside of the GKE cluster.  Setting this up a couple more steps but the easiest is to 'just run' the docker container above:

Create a VM with a public IP address (yeah, this is just a demo so i can do this..)

```
docker run -p 44225:44225 salrashid123/mygrpcadapter:latest
```

### 16.  Deploy configurations for adapter to ISTIO

We're now at the point to deploy our full adapter config to istio.

Please make sure you have istio cluster setup running the sample helloworld or BookInfo


- First setup the attributes maps and deploy:

```
kubectl apply -f $MIXER_REPO/adapter/mygrpcadapter/testdata/attributes.yaml -f $MIXER_REPO/adapter/mygrpcadapter/testdata/template.yaml

kubectl apply -f $MIXER_REPO/adapter/mygrpcadapter/testdata/mygrpcadapter.yaml
```

### 17a. Run Adapter in Cluster

If you want to run the Adapter as a cluster service:

  Then edit  `$MIXER_REPO/adapter/mygrpcadapter/testdata/sample_operator_cfg.yaml` and change the 'connection' to the service:

  ```yaml
  connection:
    #address: "[::]:44225"
    address: "mygrpcadapterservice:44225"
    #address: "35.184.34.117:44225"  
  ```
  Deploy the conig:

  ```
    kubectl apply -f $MIXER_REPO/adapter/mygrpcadapter/testdata/sample_operator_cfg.yaml
  ```

  you should now see a connection established on the mixer logs:

  ```
  $ kubectl -n istio-system logs $(kubectl -n istio-system get pods -lapp=mixer -o jsonpath='{.items[0].metadata.name}') -c mixer

  2018-12-12T17:59:49.249312Z	info	grpcAdapter	Connected to: mygrpcadapterservice:44225
  2018-12-12T17:59:49.249433Z	info	ccResolverWrapper: sending new addresses to cc: [{mygrpcadapterservice:44225 0  <nil>}]
  2018-12-12T17:59:49.249591Z	info	ClientConn switching balancer to "pick_first"
  2018-12-12T17:59:49.249758Z	info	pickfirstBalancer: HandleSubConnStateChange: 0xc4211e2cb0, CONNECTING
  2018-12-12T17:59:49.251833Z	info	pickfirstBalancer: HandleSubConnStateChange: 0xc4211e2cb0, READY
  ```

### 17b. Run off-cluster
 If you want to run the Adapter as an off-cluster service, that host must be accessible from the istio cluster.
 In my case, i setup a GCE VM with a public IP (`35.184.34.117:44225`) and opened up the port specified.

 On that VM, i ran the docker image directly

 ```
 docker run -p 44225:44225 salrashid123/mygrpcadapter
 ```
 Then edit the operator config and set the address to the IP:

 ```yaml
 connection:
   #address: "[::]:44225"
   #address: "mygrpcadapterservice:44225"
   address: "35.184.34.117:44225"  
 ```

 Then apply
 ```
   kubectl apply -f $MIXER_REPO/adapter/mygrpcadapter/testdata/sample_operator_cfg.yaml
 ```

### 18.  Test

Now send over a request to the gatewayIP address of your istio cluster with the header name and value

```
curl -vk -H "x-custom-token: abc" https://35.184.93.12/`


< HTTP/2 200
< x-powered-by: Express
< content-type: text/html; charset=utf-8
< content-length: 19
< etag: W/"13-AQEDToUxEbBicITSJoQtsw"
< date: Wed, 12 Dec 2018 18:00:30 GMT
< x-envoy-upstream-service-time: 9
< server: istio-envoy

Hello from Express!
```

If you deployed the as a cluster service, you should see

```
$ kubectl logs po/mygrpcadapter-857697c9-n7bcc -n istio-system

2018-12-12T18:00:30.786266Z	info	received request {&InstanceMsg{Subject:&SubjectMsg{User:,Groups:,Properties:map[string]*istio_policy_v1beta11.Value{custom_token_header: &Value{Value:&Value_StringValue{StringValue:abc,},},},},Action:nil,Name:icheck.instance.istio-system,} &Any{TypeUrl:type.googleapis.com/adapter.mygrpcadapter.config.Params,Value:[10 3 97 98 99],XXX_unrecognized:[],} 899657512539659575}

2018-12-12T18:00:30.786330Z	info	map[custom_token_header:abc]
k: custom_token_header v: abc
2018-12-12T18:00:30.786620Z	info	success!!
```

Now send in a malformed header:

```
curl -vk -H "x-custom-token: foooo" https://35.184.93.12/

< HTTP/2 403
< content-length: 57
< content-type: text/plain
< date: Wed, 12 Dec 2018 18:03:07 GMT
< server: istio-envoy
<

PERMISSION_DENIED:h1.handler.istio-system:Unauthorized...
```

and in the Adapter logs:
```
2018-12-12T18:03:07.573464Z	info	received request {&InstanceMsg{Subject:&SubjectMsg{User:,Groups:,Properties:map[string]*istio_policy_v1beta11.Value{custom_token_header: &Value{Value:&Value_StringValue{StringValue:foooo,},},},},Action:nil,Name:icheck.instance.istio-system,} &Any{TypeUrl:type.googleapis.com/adapter.mygrpcadapter.config.Params,Value:[10 3 97 98 99],XXX_unrecognized:[],} 8151876179173745319}

2018-12-12T18:03:07.573511Z	info	map[custom_token_header:foooo]
k: custom_token_header v: foooo
2018-12-12T18:03:07.573538Z	info	failure; header not provided
```


If you ran the Adapter off cluster, then you should see something like this in the logs inline for the success test:

```
$ docker run -p 44225:44225 salrashid123/mygrpcadapter
listening on "[::]:44225"

2018-12-12T18:05:36.016746Z	info	received request {&InstanceMsg{Subject:&SubjectMsg{User:,Groups:,Properties:map[string]*istio_policy_v1beta11.Value{custom_token_header: &Value{Value:&Value_StringValue{StringValue:abc,},},},},Action:nil,Name:icheck.instance.istio-system,} &Any{TypeUrl:type.googleapis.com/adapter.mygrpcadapter.config.Params,Value:[10 3 97 98 99],XXX_unrecognized:[],} 14067510233808898025}

2018-12-12T18:05:36.016773Z	info	map[custom_token_header:abc]
k: custom_token_header v: abc
2018-12-12T18:05:36.016781Z	info	success!!
```

### 19.  Cleanup

You can either delete the whole cluster or revert the changes entirely:

```
cd $MIXER_REPO/adapter/mygrpcadapter/testdata/

$ kubectl delete -f .
attributemanifest "istio-proxy" deleted
attributemanifest "kubernetes" deleted
adapter "mygrpcadapter" deleted
handler "h1" deleted
instance "icheck" deleted
rule "r1" deleted
template "authorization" deleted
```

## Conclusion

THis repo is a simple demo i setup on behalf of a customer looking to do the same. While you can achieve simple header checks inline without a full blown mixer adapter (eg with a [LUA filters](https://github.com/salrashid123/istio_helloworld#lua-httpfilters)),
the simplicity of this check of running the adapter in and out of cluster is hopefully of use.  

I'm positive i didn't setup the setup and install instructions in the proper 'golang vendoring' structure but it does the job.   