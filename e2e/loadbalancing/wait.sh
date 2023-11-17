#!/bin/bash

set -eux

while [[ $(kubectl get pods -n mosquitto -l app=mosquitto -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do echo "waiting for mosquitto" && sleep 5; done
while [[ $(kubectl get pods -n fuyuu-router -l app=fuyuu-router-hub -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do echo "waiting for hub pod" && sleep 5; done
while [[ $(kubectl get pods -n fuyuu-router -l app=fuyuu-router-agent-1 -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do echo "waiting for agent pod" && sleep 5; done
while [[ $(kubectl get pods -n fuyuu-router -l app=fuyuu-router-agent-2 -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do echo "waiting for agent pod" && sleep 5; done
while [[ $(kubectl get pods -n fuyuu-router -l app=curl-tester -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do echo "waiting for curl-tester pod" && sleep 5; done