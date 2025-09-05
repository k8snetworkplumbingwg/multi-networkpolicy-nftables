# Multi-Network Policy NFTables

A Kubernetes controller that enforces `MultiNetworkPolicy` resources using NFTables on Linux nodes. This project enables fine-grained network security policies for multi-network pods in Kubernetes environments.

## Overview

This controller watches for `MultiNetworkPolicy` resources and translates them into NFTables rules within pod network namespaces. It supports complex network policies including ingress/egress rules, IP blocks, pod selectors, namespace selectors, and port-based filtering.

## Features

- **Multi-Network Support**: Enforces policies on additional network interfaces (beyond the default pod network)
- **NFTables Integration**: Uses the modern NFTables framework for high-performance packet filtering
- **Dynamic Policy Management**: Real-time policy updates as pods, namespaces, and policies change
- **Comprehensive Rule Types**: 
  - Ingress and egress traffic control
  - IP block filtering with CIDR ranges and exceptions
  - Pod selector-based rules
  - Namespace selector-based rules
  - Port and protocol filtering
- **Connection Tracking**: Stateful connection tracking for established connections
- **Pod Lifecycle Management**: Automatic cleanup when pods are deleted

## Architecture

### Components

1. **MultiNetworkReconciler**: Kubernetes controller that watches `MultiNetworkPolicy` resources
2. **NFTables Engine**: Translates policies into NFTables rules and applies them to pod network namespaces
3. **Datastore**: In-memory cache of policies, pods, and network information
4. **CRI Integration**: Container runtime interface for accessing pod network namespaces
5. **Watchers**: Monitor changes to pods, namespaces, and network attachment definitions

### Supported CNI Plugins

The controller validates that network attachment definitions use supported CNI plugins:
- `macvlan`
- `ipvlan` 
- `sriov`
- `host-device`
- `bridge`

## Quick Start

### Prerequisites

- Kubernetes cluster with multi-network support
- NFTables support on cluster nodes
- `MultiNetworkPolicy` CRD installed
- Network Attachment Definition CRD installed

### Installation

1. Build the container image:
```bash
docker build -t multi-networkpolicy-nftables:latest .
```

2. Deploy to your cluster:
```bash
kubectl apply -f deploy.yaml
```

### Example Policy

```yaml
apiVersion: k8s.cni.cncf.io/v1beta1
kind: MultiNetworkPolicy
metadata:
  name: web-policy
  namespace: default
  annotations:
    k8s.v1.cni.cncf.io/policy-for: "macvlan-network"
spec:
  podSelector:
    matchLabels:
      app: web
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: frontend
    - ipBlock:
        cidr: 10.0.0.0/8
        except:
        - 10.0.1.0/24
    ports:
    - protocol: TCP
      port: 8080
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          name: database
    ports:
    - protocol: TCP
      port: 5432
```

## Documentation

For detailed documentation, see the [docs/](./docs/) directory:

- [Architecture & Design](./docs/architecture.md)
- [NFTables Implementation](./docs/nftables.md)
- [Policy Examples](./docs/examples.md)
- [Troubleshooting](./docs/troubleshooting.md)
- [Development Guide](./docs/development.md)

## Development

### Building

```bash
go build -o multi-networkpolicy-nftables ./cmd/main.go
```

### Testing

```bash
# Run unit tests
go test ./...

# Run integration tests (requires root and nftables)
sudo go test ./pkg/nftables -tags=integration
```

### Code Structure

```
├── cmd/                    # Main application entry point
├── pkg/
│   ├── controller/         # Kubernetes controller logic
│   ├── nftables/          # NFTables rule generation and management
│   ├── datastore/         # In-memory data store
│   ├── cri/               # Container runtime integration
│   └── utils/             # Utility functions
├── docs/                  # Documentation
└── deploy.yaml           # Kubernetes deployment manifest
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Ensure all tests pass
6. Submit a pull request

## License

This project is licensed under the Apache License 2.0 - see the LICENSE file for details.

## Acknowledgments

- Built on top of the [knftables](https://github.com/kubernetes-sigs/knftables) library
- Inspired by the [multi-networkpolicy](https://github.com/k8snetworkplumbingwg/multi-networkpolicy) project
- Uses the [network-attachment-definition](https://github.com/k8snetworkplumbingwg/network-attachment-definition-client) client
