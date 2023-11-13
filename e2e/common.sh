#!/bin/bash

set -eux

TEST_SCRIPT=""
WAIT_SCRIPT=""
MANIFEST_DIR=""
KIND_CONFIG=""

while [[ "$#" -gt 0 ]]; do
    case $1 in
        --run_test=*)
            TEST_SCRIPT="${1#*=}"
            ;;
        --manifest_dir=*)
            MANIFEST_DIR="${1#*=}"
            ;;
        --wait_script=*)
            WAIT_SCRIPT="${1#*=}"
            ;;
        --kind_config=*)
            KIND_CONFIG="${1#*=}"
            ;;
        *)
            echo "Unknown parameter passed: $1"
            exit 1
            ;;
    esac
    shift
done

create_cluster() {
    if [ -n "$KIND_CONFIG" ]; then
        kind create cluster --name fuyuu-test-cluster --config "$KIND_CONFIG"
    else
        kind create cluster --name fuyuu-test-cluster
    fi
    kubectl create namespace fuyuu-router
    kubectl create namespace nanomq
}

build_and_load_image() {
    docker build -t fuyuu-router:dev .
    kind load docker-image --name fuyuu-test-cluster "fuyuu-router:dev"
}

wait_pods() {
    if [ -n "$WAIT_SCRIPT" ]; then
        "$WAIT_SCRIPT"
    else
        while [[ $(kubectl get pods -n nanomq -l app=nanomq -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do echo "waiting for nanomq" && sleep 5; done
        while [[ $(kubectl get pods -n fuyuu-router -l app=fuyuu-router-hub -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do echo "waiting for hub pod" && sleep 5; done
        while [[ $(kubectl get pods -n fuyuu-router -l app=fuyuu-router-agent -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do echo "waiting for agent pod" && sleep 5; done
        while [[ $(kubectl get pods -n fuyuu-router -l app=curl-tester -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do echo "waiting for curl-tester pod" && sleep 5; done
    fi
}

run_test() {
    if [ -n "$TEST_SCRIPT" ]; then
        echo "Running test script: $TEST_SCRIPT"
        "$TEST_SCRIPT"
        result=$?
    else
        curl_tester_pod=$(kubectl get pods -n fuyuu-router -l app=curl-tester -o jsonpath='{.items[0].metadata.name}')
        response=$(kubectl exec -n fuyuu-router "${curl_tester_pod}" -- curl -H "FuyuuRouter-IDs: agent01" http://fuyuu-router-hub.fuyuu-router:8080 -s)
        if [[ "$response" == "Hello, world!" ]]; then
            result=0
        else
            result=1
        fi
    fi

    if [[ "$result" == 0 ]]; then
        echo "Test passed"
        delete_cluster
        exit 0
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
kubectl apply -f "$MANIFEST_DIR"
wait_pods
run_test
