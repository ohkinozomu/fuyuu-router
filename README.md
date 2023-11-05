# fuyuu-router

Multi-cloud HTTP communication using MQTT

## How to use

### hub

```bash
fuyuu-router hub -b BROKER-IP:1883
```

### agent

```bash
fuyuu-router agent --id agent01 -b BROKER-IP:1883 --proxy-host PROXY-HOST-IP
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

### Why using Badger?

To share channel between the HTTP handler and paho.golang's Router, it is necessary to create a Router for every HTTP request.

Making a TCP connection and MQTT CONNECT for every HTTP request is disadvantageous in terms of performance(0.5 second delay in rough measurement).

First, a TCP connection and MQTT CONNECT are established. Then, for every HTTP request, Badger's Subscribe is executed.

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
