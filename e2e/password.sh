#!/bin/bash

set -eux

kind create cluster --name fuyuu-test-cluster

kubectl create namespace fuyuu-router
kubectl create namespace nanomq

docker build -t fuyuu-router:dev .

kind load docker-image --name fuyuu-test-cluster "fuyuu-router:dev"

cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nanomq
  namespace: nanomq
  labels:
    app: nanomq
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nanomq
  template:
    metadata:
      labels:
        app: nanomq
    spec:
      containers:
      - name: nanomq
        image: emqx/nanomq:latest
        ports:
        - containerPort: 1883
        - containerPort: 8083
        - containerPort: 8883
        volumeMounts:
        - name: config-volume
          mountPath: /etc/nanomq.conf
          subPath: nanomq.conf
        - name: nanomq-pwd
          mountPath: /etc/nanomq_pwd.conf
          subPath: nanomq_pwd.conf
      volumes:
      - name: config-volume
        configMap:
          name: nanomq-config
      - name: nanomq-pwd
        configMap:
          name: nanomq-pwd
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: nanomq-config
  namespace: nanomq
data:
  nanomq.conf: |
    # NanoMQ Configuration 0.18.0
  
    # #============================================================
    # # NanoMQ Broker
    # #============================================================
  
    mqtt {
        property_size = 32
        max_packet_size = 1MB
        max_mqueue_len = 2048
        retry_interval = 10s
        keepalive_multiplier = 1.25
  
        # Three of below, unsupported now
        max_inflight_window = 2048
        max_awaiting_rel = 10s
        await_rel_timeout = 10s
    }
  
    listeners.tcp {
        bind = "0.0.0.0:1883"
    }
  
    # listeners.ssl {
    # 	bind = "0.0.0.0:8883"
    # 	keyfile = "/etc/certs/key.pem"
    # 	certfile = "/etc/certs/cert.pem"
    # 	cacertfile = "/etc/certs/cacert.pem"
    # 	verify_peer = false
    # 	fail_if_no_peer_cert = false
    # }
  
    listeners.ws {
        bind = "0.0.0.0:8083/mqtt"
    }
  
    http_server {
        port = 8081
        limit_conn = 2
        username = admin
        password = public
        auth_type = basic
        jwt {
            public.keyfile = "/etc/certs/jwt/jwtRS256.key.pub"
        }
    }
  
    log {
        to = [file, console]
        level = debug
        dir = "/tmp"
        file = "nanomq.log"
        rotation {
            size = 10MB
            count = 5
        }
    }
  
    auth {
        allow_anonymous = false
        no_match = allow
        deny_action = ignore
  
        cache = {
            max_size = 32
            ttl = 1m
        }
  
        password = {include "/etc/nanomq_pwd.conf"}
        # acl = {include "/etc/nanomq_acl.conf"}
    }
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: nanomq-pwd
  namespace: nanomq
data:
  nanomq_pwd.conf: |
    admin: adminpassword
---
apiVersion: v1
kind: Service
metadata:
  name: nanomq
  namespace: nanomq
spec:
  selector:
    app: nanomq
  ports:
    - protocol: TCP
      port: 1883
      targetPort: 1883
  type: NodePort
EOF

cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: fuyuu-router-hub
  namespace: fuyuu-router
spec:
  replicas: 1
  selector:
    matchLabels:
      app: fuyuu-router-hub
  template:
    metadata:
      labels:
        app: fuyuu-router-hub
    spec:
      containers:
      - name: fuyuu-router-hub
        image: fuyuu-router:dev
        command: ["/bin/sh", "-c"]
        args:
        - |
          /app/fuyuu-router hub -b nanomq.nanomq:1883 --username admin --password adminpassword
        ports:
        - containerPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: fuyuu-router-hub
  namespace: fuyuu-router
spec:
  selector:
    app: fuyuu-router-hub
  ports:
    - protocol: TCP
      port: 8080
      targetPort: 8080
  type: NodePort
EOF

cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: fuyuu-router-agent
  namespace: fuyuu-router
spec:
  replicas: 1
  selector:
    matchLabels:
      app: fuyuu-router-agent
  template:
    metadata:
      labels:
        app: fuyuu-router-agent
    spec:
      containers:
      - name: fuyuu-router-agent
        image: fuyuu-router:dev
        command: ["/bin/sh", "-c"]
        args:
        - |
          /app/fuyuu-router agent --id agent01 -b nanomq.nanomq:1883 --proxy-host 127.0.0.1:8000 --loglevel debug --username admin --password adminpassword
      - name: appserver
        image: golang:1.21.3
        command: ["/bin/sh", "-c"]
        args:
        - |
          echo 'package main; import ("net/http"; "fmt"); func main() { http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, "Hello, world!") }); http.ListenAndServe(":8000", nil) }' > server.go && go run server.go
        ports:
        - containerPort: 8000
        livenessProbe:
          httpGet:
            path: /
            port: 8000
          initialDelaySeconds: 15
          periodSeconds: 5
        readinessProbe:
          httpGet:
            path: /
            port: 8000
          initialDelaySeconds: 15
          periodSeconds: 5
EOF

cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: curl-tester
  namespace: fuyuu-router
spec:
  replicas: 1
  selector:
    matchLabels:
      app: curl-tester
  template:
    metadata:
      labels:
        app: curl-tester
    spec:
      containers:
      - name: curl-tester
        image: curlimages/curl:latest
        command:
        - /bin/sh
        - -c
        - sleep 3600
EOF

while [[ $(kubectl get pods -n nanomq -l app=nanomq -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do echo "waiting for nanomq" && sleep 5; done
while [[ $(kubectl get pods -n fuyuu-router -l app=fuyuu-router-hub -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do echo "waiting for hub pod" && sleep 5; done
while [[ $(kubectl get pods -n fuyuu-router -l app=fuyuu-router-agent -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do echo "waiting for agent pod" && sleep 5; done
while [[ $(kubectl get pods -n fuyuu-router -l app=curl-tester -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do echo "waiting for curl-tester pod" && sleep 5; done

curl_tester_pod=$(kubectl get pods -n fuyuu-router -l app=curl-tester -o jsonpath='{.items[0].metadata.name}')
response=$(kubectl exec -n fuyuu-router "${curl_tester_pod}" -- curl -H "FuyuuRouter-ID: agent01" http://fuyuu-router-hub.fuyuu-router:8080 -s)

if [[ "$response" == "Hello, world!" ]]; then
  echo "Test passed"
else
  echo "Test failed"
  nanomq_pod=$(kubectl get pods -n nanomq -l app=nanomq -o jsonpath='{.items[0].metadata.name}')
  echo "Displaying logs for nanomq:"
  kubectl logs -n nanomq "${nanomq_pod}"

  fuyuu_router_hub_pod=$(kubectl get pods -n fuyuu-router -l app=fuyuu-router-hub -o jsonpath='{.items[0].metadata.name}')
  echo "Displaying logs for fuyuu-router-hub:"
  kubectl logs -n fuyuu-router "${fuyuu_router_hub_pod}"

  fuyuu_router_agent_pod=$(kubectl get pods -n fuyuu-router -l app=fuyuu-router-agent -o jsonpath='{.items[0].metadata.name}')
  echo "Displaying logs for fuyuu-router-agent:"
  kubectl logs -n fuyuu-router "${fuyuu_router_agent_pod}"
  kubectl logs -n fuyuu-router "${fuyuu_router_agent_pod}" -c sinatra-server

  kind delete cluster --name fuyuu-test-cluster
  exit 1
fi

kind delete cluster --name fuyuu-test-cluster
exit 0
