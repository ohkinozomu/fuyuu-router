#!/bin/bash

set -eux

agent01_count=0
agent02_count=0

curl_tester_pod=$(kubectl get pods -n fuyuu-router -l app=curl-tester -o jsonpath='{.items[0].metadata.name}')

for _ in {1..30}; do
    response=$(kubectl exec -n fuyuu-router "${curl_tester_pod}" -- curl -H "FuyuuRouter-IDs: agent01,agent02" http://fuyuu-router-hub.fuyuu-router:8080 -s)

    if [[ "$response" == "Hello from agent01!" ]]; then
        ((agent01_count++))
    elif [[ "$response" == "Hello from agent02!" ]]; then
        ((agent02_count++))
    else
        echo "Unexpected response: $response"
    fi

    sleep 1
done

if [[ "$agent01_count" -gt 0 && "$agent02_count" -gt 0 ]]; then
    echo "Load balancing test succeeded."
    echo "agent01 count: $agent01_count"
    echo "agent02 count: $agent02_count"
    exit 0
else
    echo "Load balancing test failed."
    echo "agent01 count: $agent01_count"
    echo "agent02 count: $agent02_count"
    exit 1
fi
