#!/bin/bash

set -eux

picture_file_local="e2e/split/picture.jpg"

curl_tester_pod=$(kubectl get pods -n fuyuu-router -l app=curl-tester -o jsonpath='{.items[0].metadata.name}')

kubectl cp "$picture_file_local" fuyuu-router/"${curl_tester_pod}":/tmp/picture.jpg

response=$(kubectl exec -n fuyuu-router "${curl_tester_pod}" -- sh -c "
    curl -H 'FuyuuRouter-IDs: agent01' -F 'file=@/tmp/picture.jpg' http://fuyuu-router-hub.fuyuu-router:8080 -s
")

if [[ "$response" == "ok" ]]; then
    exit 0
else
    exit 1
fi
