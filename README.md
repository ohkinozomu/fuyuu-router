# fuyuu-router

Multi-cloud HTTP communication using MQTT

## How to use

### hub

```bash
fuyuu-router hub -c config.toml -b mqtt://BROKER-IP:1883
```

### agent

```bash
fuyuu-router agent -c config.toml --id agent01 -b mqtt://BROKER-IP:1883 --proxy-host PROXY-HOST-IP
```

### HTTP request

```
curl http://FUYUU-ROUTER-HUB-IP:8080/ -H "FuyuuRouter-IDs: agent01"
```

This request follows the path below:

`fuyuu-router hub -> MQTT broker -> fuyuu-router agent -> proxy host`

## Load balancing

When multiple IDs are passed to FuyuuRouter-IDs separated by commas, fuyuu-router performs load balancing between those IDs.

The current load balancing algorithm is random only.

## About service discovery

fuyuu-router doesn't perform any service discovery. 

Agents publish a message to the `fuyuu-router/launch` topic when they launch and to the `fuyuu-router/terminate` topic when they disconnect. 

Users can subscribe to these topics to create a mechanism, for example, to update Istio VirtualServices.

## large data policy

While many MQTT brokers have payload size limitations, there is a desire to send large HTTP request/response.

The fuyuu-router addresses this with two policies - 1) split and 2) storage relay.

### split

The fuyuu-router can bypass payload size limitations by splitting large HTTP requests/responses into multiple chunks and then combining them.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: example
data:
  config.toml: |
    [networking]
    format = "json"
    large_data_policy = "split"

    [split]
    chunk_bytes = 128000
```

### storage relay

fuyuu-router allows the hub and agent to transparently communicate with each other through object storage only when the HTTP request/response is large.

fuyuu-router uses [objstore](https://github.com/thanos-io/objstore), and its configuration file is needed.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: example
data:
  config.toml: |
    [networking]
    format = "json"
    large_data_policy = "storage_relay"
    [storage_relay]
    objstore_file = "/app/config/objstore.yaml"
    threshold_bytes = 128000
  objstore.yaml: |
    type: S3
    config:
      bucket: "example-bucket"
      endpoint: "s3.amazonaws.com"
      access_key: "<ACCESS_KEY>"
      secret_key: "<SECRET_KEY>"
```

The hub and agent must reference the same bucket and have appropriate permissions to it.

For instance, in S3, the shortest deletion period is one day. In fuyuu-router, since object storage is used only temporarily, if you want to reduce costs, you can have the hub delete objects by using the `deletion` settings.

```toml
[storage_relay]
objstore_file = "/app/config/objstore.yaml"
threshold_bytes = 128000
deletion = true
```

## limitation

- Currently only HTTP 1.1 is supported. Perhaps HTTP2 is the next roadmap.
- fuyuu-router introduces an additional hop to the MQTT broker. It is not suitable for cases where strict latency requirements are demanded.

## FAQ

### How about [Skupper](https://github.com/skupperproject)?

Skupper is great, but I had a few complaints.

- [skupper doesn't support revocation of a specific site only](https://github.com/skupperproject/skupper/issues/779). To work around that problem deploy Skupper and load balancer per site
- [Skupper doesn't support star topology](https://github.com/skupperproject/skupper/issues/1215). To work around that problem deploy Skupper and load balancer per site
- [skupper-router](https://github.com/skupperproject/skupper-router) is written in C. 
- Skupper uses AMQP, but [Open Cluster Management has MQTT-based client](https://github.com/open-cluster-management-io/api/tree/v0.12.0/cloudevents).

![Skupper comparison](docs/skupper-comparison.svg)

### Are there problems with MQTT topics per HTTP request?

I referred to the following articles:

- https://stackoverflow.com/questions/68533190/any-implications-of-using-uuids-in-mqtt-topic-names
- https://github.com/emqx/emqx/discussions/8261
- https://repost.aws/questions/QUkUTmC3fRRkWb41he5U-HwQ/is-there-a-limitation-on-the-number-of-topics-in-iot-core

### What does "fuyuu" mean?

"fuyuu" means floating in Japanese(浮遊). It flies above the clouds.

It is also inspirated by the word [sky computing](https://dl.acm.org/doi/abs/10.1145/3458336.3465301).

## Status

PoC
