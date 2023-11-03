#!/bin/bash

set -eux

create_cluster() {
    kind create cluster --name fuyuu-test-cluster
    kubectl create namespace fuyuu-router
    kubectl create namespace nanomq
}

build_and_load_image() {
    docker build -t fuyuu-router:dev .
    kind load docker-image --name fuyuu-test-cluster "fuyuu-router:dev"
}

wait_pods() {
    while [[ $(kubectl get pods -n nanomq -l app=nanomq -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do echo "waiting for nanomq" && sleep 5; done
    while [[ $(kubectl get pods -n fuyuu-router -l app=fuyuu-router-hub -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do echo "waiting for hub pod" && sleep 5; done
    while [[ $(kubectl get pods -n fuyuu-router -l app=fuyuu-router-agent -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do echo "waiting for agent pod" && sleep 5; done
    while [[ $(kubectl get pods -n fuyuu-router -l app=curl-tester -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do echo "waiting for curl-tester pod" && sleep 5; done
}

run_test() {
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
      kubectl logs -n fuyuu-router "${fuyuu_router_agent_pod}" -c appserver

      delete_cluster
      exit 1
    fi
}

delete_cluster() {
  kind delete cluster --name fuyuu-test-cluster
}

create_cluster
build_and_load_image
source "$1"
wait_pods
run_test
delete_cluster
exit 0