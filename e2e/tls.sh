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
        - name: certs-volume
          mountPath: /etc/certs
      volumes:
      - name: config-volume
        configMap:
          name: nanomq-config
      - name: certs-volume
        configMap:
          name: nanomq-certs
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
  
    listeners.ssl {
    	bind = "0.0.0.0:8883"
    	keyfile = "/etc/certs/key.pem"
    	certfile = "/etc/certs/cert.pem"
    	cacertfile = "/etc/certs/cacert.pem"
    	verify_peer = true
    	fail_if_no_peer_cert = true
    }
  
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
        allow_anonymous = true
        no_match = allow
        deny_action = ignore
  
        cache = {
            max_size = 32
            ttl = 1m
        }
  
        # password = {include "/etc/nanomq_pwd.conf"}
        # acl = {include "/etc/nanomq_acl.conf"}
    }
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: nanomq-certs
  namespace: nanomq
data:
  key.pem: |
    -----BEGIN PRIVATE KEY-----
    MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDF75+wAxyMjqOs
    EhRcUFusmRuzbFT+o6vAvEfPS56+RrAmBiU79bOM8aixlctVL6tdH8tJDW8pe7Yi
    PwarloUGISN1qA+FxUb3E1v8qPdndiJmJWRGXm1qodgTTte0ExxLBCVyG14AKiJ8
    Q7mJ/EfvEXbbLxX60Q3FTi6LE8kVqHV5N4k/R3yK58GCruPYo17Edp3RcVWTEYnb
    2hYUUf1ky4DfOwQQN1KwsNJhASFQATwjvALXZ9jmkM638mhBnfxW/ZkVu9lQkiDW
    nPDkHLs3Qz3Olb/SAlzA/4qwzmM6DPx+6j7W8ML+G4RdiuLRqIzgff2olOPzveK5
    j5Eah3ObAgMBAAECggEAPBfhGnYHX+Eabe5bQh+fhYpCb7nPIDQeu/gtsRDbVBdv
    +UtaWJbi+UKRHcFFp0o+s5oohLhQbH7DsCgEZWngXxkGg/0PIWTgg7jb75x46G9k
    SDDH/dlDTOFwEYSZVnGK4HeUyszmQBSKvcFt/ieay0k5FZh5CtoXXTS8SrsqDKm8
    OIpebWzULmDZvmVYlZAgBMjvHSHeLeK74q5PjwrSK10FCv8GJA+F6LeDv8tUO83Y
    DQEAOhLENYiHcRLveomI5roWNugAcJ30oENuxCg9TjkF6wqpDDZDVU+rNoEqlrwF
    pwYpggpgpt98/uxzXf/MFzyDRb7UhLI5vfX3HT6eMQKBgQD1Qy8thBC8km1xpu1K
    k6GtrbuwVj32pRKJtiGzTzVRCzHh2THMK3bRTpnkfthWy3iBIiG9641uy1jist07
    QOeboHU6LmqDhjySTzNxdHf0Pg/W0ER6+AsZVSZynTQEllMB+H/bZwKY1GQcKkcH
    uIogDlVeaE3xZiS6vvEAEZGN0wKBgQDOmgWKgLh1sOcBKkpugNaliTpHetp/4iJv
    7JDYP0xJ3ogoDI7vhQ6an6l7mfCkcdnua/ruM6WiKSft9HRkUlX3GncQ/FALRh2L
    +oaZ1ICQzO/Ggv/wrpb3GqYmV9Cmt7Z5WOJfUUI2CQjJrr/r5UqbTmnLxyPVAWKS
    ZLfmNPq+GQKBgEjjx6CaUDMKvXX6aykvyOwJ5u7YIqArnN/KfieBEdJdJlz9pJwO
    CsjXuEq9G+Rnog+Wqjp8R9M2odr112PlvS92N4CsDMG74kKFQT+looS28RQhX0jA
    cOP9d2i2qZ/3YQID7VOyQIZVEM+CDQwRXxN5zws4qnlkpuPNHWisz/o7AoGBAMUt
    0qQBfgs1LwO5rRgR9so+UlTuN6Nd26gei48XumO18xTmB3Up9Go2f7brkPQhhPE8
    NV0qBabiyK0eZgdpXYpcw85+QJbB8GksTVJ7sciBD0bSuBqpRoPH91MY9JZpN8pQ
    vpxiHWMc9DooghtN1wqqp+ZIxTYCAGXfonQflD/hAoGBALv2258IGnI+mMz0GCvi
    CK2EVdNtEAUtjDFplPCvegMNX8eZlvAGG+VHlr90xjUpwmcWwvaUxm/ujq4bWNDb
    CKvftx043UREHTsvVXDGXWUkTycwUXeDSFYYSK2CgMpTnH9rD/9uSnL2Pqv9p5fQ
    /uK6BwDtFPKFfZA3uc1K/WHa
    -----END PRIVATE KEY-----
  cert.pem: |
    -----BEGIN CERTIFICATE-----
    MIIDpDCCAoygAwIBAgIUV8VP0yZzFH1zEhX6la5HEDXPJcUwDQYJKoZIhvcNAQEL
    BQAwXTELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
    GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDEWMBQGA1UEAwwNbmFub21xLm5hbm9t
    cTAeFw0yMzEwMjUyMjQyNTNaFw0zMzEwMjIyMjQyNTNaMF0xCzAJBgNVBAYTAkFV
    MRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRz
    IFB0eSBMdGQxFjAUBgNVBAMMDW5hbm9tcS5uYW5vbXEwggEiMA0GCSqGSIb3DQEB
    AQUAA4IBDwAwggEKAoIBAQDF75+wAxyMjqOsEhRcUFusmRuzbFT+o6vAvEfPS56+
    RrAmBiU79bOM8aixlctVL6tdH8tJDW8pe7YiPwarloUGISN1qA+FxUb3E1v8qPdn
    diJmJWRGXm1qodgTTte0ExxLBCVyG14AKiJ8Q7mJ/EfvEXbbLxX60Q3FTi6LE8kV
    qHV5N4k/R3yK58GCruPYo17Edp3RcVWTEYnb2hYUUf1ky4DfOwQQN1KwsNJhASFQ
    ATwjvALXZ9jmkM638mhBnfxW/ZkVu9lQkiDWnPDkHLs3Qz3Olb/SAlzA/4qwzmM6
    DPx+6j7W8ML+G4RdiuLRqIzgff2olOPzveK5j5Eah3ObAgMBAAGjXDBaMBgGA1Ud
    EQQRMA+CDW5hbm9tcS5uYW5vbXEwHQYDVR0OBBYEFMAQMNLecvuu4NFRi6HKfGjh
    G/UcMB8GA1UdIwQYMBaAFASvpL+3lOR2Qe55/kNAsl6fn0FGMA0GCSqGSIb3DQEB
    CwUAA4IBAQALIrMaMRNllFfI9dCKW317YVLaYE7yrnbYrzHS3hrUq0MVNqncuiZU
    ZE3GEDIn/f9Z2rpsRTFSe1q1LubGhhRNLNBlENHWECh8c/hrmiucbJvCMSGWSKh4
    pVBoSItiTcS4MZM9b9KOJo4j52SDKsP8D1ojMSSOzsuuIDIrZrE4VOxtHMZc5+fB
    bu0LOCnYHzrakfZaa4jtKfTgN55Bjt8J1jodv2BtSdHB4JlwPv4S2irCTZpcke4T
    jLK1SufpoyzCyCr/4PTHlC/d2AkhRuNOZNe78ZzI9yUlQAeJCbqYBcDheckd4pNc
    GsVCjbLGcu8YOgYRqR5iuHHt3hccWafh
    -----END CERTIFICATE-----
  cacert.pem: |
    -----BEGIN CERTIFICATE-----
    MIIDmzCCAoOgAwIBAgIUUFHqLQNUHyTVYRPg+frSSkre2PMwDQYJKoZIhvcNAQEL
    BQAwXTELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
    GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDEWMBQGA1UEAwwNbmFub21xLm5hbm9t
    cTAeFw0yMzEwMjUyMjQyMjNaFw0yNjA4MTQyMjQyMjNaMF0xCzAJBgNVBAYTAkFV
    MRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRz
    IFB0eSBMdGQxFjAUBgNVBAMMDW5hbm9tcS5uYW5vbXEwggEiMA0GCSqGSIb3DQEB
    AQUAA4IBDwAwggEKAoIBAQDRT9pTC0XlfWepVCNZRonEeV6M0wbfuDmT4ofubPx4
    VydkqUXYvkbjYVrX2uJjtc2+QKFCgIiiCLVR0aVZgF5RtNNBc26lCionIdALUqzj
    ed3fKld7M6yLFT5VOTNkskVxNZTAkMQIMje+9rYkDgNHTTznQXWA2IbtfrehWGdF
    EK2HwJncBXJDHS5IKSYtPnnkPIZKfc3JyF/JsPPZDU71a6o0ycYvObLPimCGR7uY
    nx0bqXGdoBHrNSSN6W+gTovO+RVfwnhPA3r1OKQdRAgLocNkiPaaDizAlGYJgP+t
    yVO57ItZDyz1M/fwEqL2/pL97yI5FXv+xtHNgSnmDW5FAgMBAAGjUzBRMB0GA1Ud
    DgQWBBQEr6S/t5TkdkHuef5DQLJen59BRjAfBgNVHSMEGDAWgBQEr6S/t5TkdkHu
    ef5DQLJen59BRjAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQAy
    uSwvx49xVEheG8VCNAJfridqORYWu/RZsid/L6Pl1yZ00ZAxrCQzlLLs/rsLPYgc
    vD5Rr5Gr3koTo0WqV1vfdAWgnF8sJ0pBT4DnrVWxRaJJ2JpPx1n7aVxjuGxzTeIV
    W14tClRZ0jRNPUV9yPIFyI2m/Y2RDkr/4iW+5ogQtAjUhZd0Pe/3Ro75XCOJ1zxo
    mCx9D5Qj4CcyjnP8J3hTIckpUcL19VMW9pJwX9uvUztgEVuxAj4VhTXTIaw47w5N
    jPn2L8Wkat0rv4okP2fPDcliiSDcmdjV3cjlZkBF5O2cdQK8MgJVFSncF9QL9Y1E
    58tnLwAXnZxcRJAGYITQ
    -----END CERTIFICATE-----
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
      port: 8883
      targetPort: 8883
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
          /app/fuyuu-router hub -b nanomq.nanomq:8883 --loglevel debug --cafile /etc/certs/cacert.pem --cert /etc/certs/cert.pem --key /etc/certs/key.pem
        ports:
        - containerPort: 8080
        volumeMounts:
        - name: certs-volume
          mountPath: /etc/certs
      volumes:
      - name: certs-volume
        configMap:
          name: fuyuu-router-certs
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: fuyuu-router-certs
  namespace: fuyuu-router
data:
  key.pem: |
    -----BEGIN PRIVATE KEY-----
    MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDF75+wAxyMjqOs
    EhRcUFusmRuzbFT+o6vAvEfPS56+RrAmBiU79bOM8aixlctVL6tdH8tJDW8pe7Yi
    PwarloUGISN1qA+FxUb3E1v8qPdndiJmJWRGXm1qodgTTte0ExxLBCVyG14AKiJ8
    Q7mJ/EfvEXbbLxX60Q3FTi6LE8kVqHV5N4k/R3yK58GCruPYo17Edp3RcVWTEYnb
    2hYUUf1ky4DfOwQQN1KwsNJhASFQATwjvALXZ9jmkM638mhBnfxW/ZkVu9lQkiDW
    nPDkHLs3Qz3Olb/SAlzA/4qwzmM6DPx+6j7W8ML+G4RdiuLRqIzgff2olOPzveK5
    j5Eah3ObAgMBAAECggEAPBfhGnYHX+Eabe5bQh+fhYpCb7nPIDQeu/gtsRDbVBdv
    +UtaWJbi+UKRHcFFp0o+s5oohLhQbH7DsCgEZWngXxkGg/0PIWTgg7jb75x46G9k
    SDDH/dlDTOFwEYSZVnGK4HeUyszmQBSKvcFt/ieay0k5FZh5CtoXXTS8SrsqDKm8
    OIpebWzULmDZvmVYlZAgBMjvHSHeLeK74q5PjwrSK10FCv8GJA+F6LeDv8tUO83Y
    DQEAOhLENYiHcRLveomI5roWNugAcJ30oENuxCg9TjkF6wqpDDZDVU+rNoEqlrwF
    pwYpggpgpt98/uxzXf/MFzyDRb7UhLI5vfX3HT6eMQKBgQD1Qy8thBC8km1xpu1K
    k6GtrbuwVj32pRKJtiGzTzVRCzHh2THMK3bRTpnkfthWy3iBIiG9641uy1jist07
    QOeboHU6LmqDhjySTzNxdHf0Pg/W0ER6+AsZVSZynTQEllMB+H/bZwKY1GQcKkcH
    uIogDlVeaE3xZiS6vvEAEZGN0wKBgQDOmgWKgLh1sOcBKkpugNaliTpHetp/4iJv
    7JDYP0xJ3ogoDI7vhQ6an6l7mfCkcdnua/ruM6WiKSft9HRkUlX3GncQ/FALRh2L
    +oaZ1ICQzO/Ggv/wrpb3GqYmV9Cmt7Z5WOJfUUI2CQjJrr/r5UqbTmnLxyPVAWKS
    ZLfmNPq+GQKBgEjjx6CaUDMKvXX6aykvyOwJ5u7YIqArnN/KfieBEdJdJlz9pJwO
    CsjXuEq9G+Rnog+Wqjp8R9M2odr112PlvS92N4CsDMG74kKFQT+looS28RQhX0jA
    cOP9d2i2qZ/3YQID7VOyQIZVEM+CDQwRXxN5zws4qnlkpuPNHWisz/o7AoGBAMUt
    0qQBfgs1LwO5rRgR9so+UlTuN6Nd26gei48XumO18xTmB3Up9Go2f7brkPQhhPE8
    NV0qBabiyK0eZgdpXYpcw85+QJbB8GksTVJ7sciBD0bSuBqpRoPH91MY9JZpN8pQ
    vpxiHWMc9DooghtN1wqqp+ZIxTYCAGXfonQflD/hAoGBALv2258IGnI+mMz0GCvi
    CK2EVdNtEAUtjDFplPCvegMNX8eZlvAGG+VHlr90xjUpwmcWwvaUxm/ujq4bWNDb
    CKvftx043UREHTsvVXDGXWUkTycwUXeDSFYYSK2CgMpTnH9rD/9uSnL2Pqv9p5fQ
    /uK6BwDtFPKFfZA3uc1K/WHa
    -----END PRIVATE KEY-----
  cert.pem: |
    -----BEGIN CERTIFICATE-----
    MIIDpDCCAoygAwIBAgIUV8VP0yZzFH1zEhX6la5HEDXPJcUwDQYJKoZIhvcNAQEL
    BQAwXTELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
    GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDEWMBQGA1UEAwwNbmFub21xLm5hbm9t
    cTAeFw0yMzEwMjUyMjQyNTNaFw0zMzEwMjIyMjQyNTNaMF0xCzAJBgNVBAYTAkFV
    MRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRz
    IFB0eSBMdGQxFjAUBgNVBAMMDW5hbm9tcS5uYW5vbXEwggEiMA0GCSqGSIb3DQEB
    AQUAA4IBDwAwggEKAoIBAQDF75+wAxyMjqOsEhRcUFusmRuzbFT+o6vAvEfPS56+
    RrAmBiU79bOM8aixlctVL6tdH8tJDW8pe7YiPwarloUGISN1qA+FxUb3E1v8qPdn
    diJmJWRGXm1qodgTTte0ExxLBCVyG14AKiJ8Q7mJ/EfvEXbbLxX60Q3FTi6LE8kV
    qHV5N4k/R3yK58GCruPYo17Edp3RcVWTEYnb2hYUUf1ky4DfOwQQN1KwsNJhASFQ
    ATwjvALXZ9jmkM638mhBnfxW/ZkVu9lQkiDWnPDkHLs3Qz3Olb/SAlzA/4qwzmM6
    DPx+6j7W8ML+G4RdiuLRqIzgff2olOPzveK5j5Eah3ObAgMBAAGjXDBaMBgGA1Ud
    EQQRMA+CDW5hbm9tcS5uYW5vbXEwHQYDVR0OBBYEFMAQMNLecvuu4NFRi6HKfGjh
    G/UcMB8GA1UdIwQYMBaAFASvpL+3lOR2Qe55/kNAsl6fn0FGMA0GCSqGSIb3DQEB
    CwUAA4IBAQALIrMaMRNllFfI9dCKW317YVLaYE7yrnbYrzHS3hrUq0MVNqncuiZU
    ZE3GEDIn/f9Z2rpsRTFSe1q1LubGhhRNLNBlENHWECh8c/hrmiucbJvCMSGWSKh4
    pVBoSItiTcS4MZM9b9KOJo4j52SDKsP8D1ojMSSOzsuuIDIrZrE4VOxtHMZc5+fB
    bu0LOCnYHzrakfZaa4jtKfTgN55Bjt8J1jodv2BtSdHB4JlwPv4S2irCTZpcke4T
    jLK1SufpoyzCyCr/4PTHlC/d2AkhRuNOZNe78ZzI9yUlQAeJCbqYBcDheckd4pNc
    GsVCjbLGcu8YOgYRqR5iuHHt3hccWafh
    -----END CERTIFICATE-----
  cacert.pem: |
    -----BEGIN CERTIFICATE-----
    MIIDmzCCAoOgAwIBAgIUUFHqLQNUHyTVYRPg+frSSkre2PMwDQYJKoZIhvcNAQEL
    BQAwXTELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
    GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDEWMBQGA1UEAwwNbmFub21xLm5hbm9t
    cTAeFw0yMzEwMjUyMjQyMjNaFw0yNjA4MTQyMjQyMjNaMF0xCzAJBgNVBAYTAkFV
    MRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRz
    IFB0eSBMdGQxFjAUBgNVBAMMDW5hbm9tcS5uYW5vbXEwggEiMA0GCSqGSIb3DQEB
    AQUAA4IBDwAwggEKAoIBAQDRT9pTC0XlfWepVCNZRonEeV6M0wbfuDmT4ofubPx4
    VydkqUXYvkbjYVrX2uJjtc2+QKFCgIiiCLVR0aVZgF5RtNNBc26lCionIdALUqzj
    ed3fKld7M6yLFT5VOTNkskVxNZTAkMQIMje+9rYkDgNHTTznQXWA2IbtfrehWGdF
    EK2HwJncBXJDHS5IKSYtPnnkPIZKfc3JyF/JsPPZDU71a6o0ycYvObLPimCGR7uY
    nx0bqXGdoBHrNSSN6W+gTovO+RVfwnhPA3r1OKQdRAgLocNkiPaaDizAlGYJgP+t
    yVO57ItZDyz1M/fwEqL2/pL97yI5FXv+xtHNgSnmDW5FAgMBAAGjUzBRMB0GA1Ud
    DgQWBBQEr6S/t5TkdkHuef5DQLJen59BRjAfBgNVHSMEGDAWgBQEr6S/t5TkdkHu
    ef5DQLJen59BRjAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQAy
    uSwvx49xVEheG8VCNAJfridqORYWu/RZsid/L6Pl1yZ00ZAxrCQzlLLs/rsLPYgc
    vD5Rr5Gr3koTo0WqV1vfdAWgnF8sJ0pBT4DnrVWxRaJJ2JpPx1n7aVxjuGxzTeIV
    W14tClRZ0jRNPUV9yPIFyI2m/Y2RDkr/4iW+5ogQtAjUhZd0Pe/3Ro75XCOJ1zxo
    mCx9D5Qj4CcyjnP8J3hTIckpUcL19VMW9pJwX9uvUztgEVuxAj4VhTXTIaw47w5N
    jPn2L8Wkat0rv4okP2fPDcliiSDcmdjV3cjlZkBF5O2cdQK8MgJVFSncF9QL9Y1E
    58tnLwAXnZxcRJAGYITQ
    -----END CERTIFICATE-----
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
          /app/fuyuu-router agent --id agent01 -b nanomq.nanomq:8883 --proxy-host 127.0.0.1:8000 --loglevel debug --cafile /etc/certs/cacert.pem --cert /etc/certs/cert.pem --key /etc/certs/key.pem
        volumeMounts:
        - name: certs-volume
          mountPath: /etc/certs
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
      volumes:
      - name: certs-volume
        configMap:
          name: fuyuu-router-certs
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
  kubectl logs -n fuyuu-router "${fuyuu_router_agent_pod}" -c appserver

  kind delete cluster --name fuyuu-test-cluster
  exit 1
fi

kind delete cluster --name fuyuu-test-cluster
exit 0
