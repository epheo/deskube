# Status and Monitoring

DeskuBe provides detailed status information about VDI instances through standard Kubernetes conditions. Conditions give users and system components a clear view of the VDI instance's state and help with debugging issues.

## Status Conditions

Each VDI instance reports the following standard conditions:

| Condition | Description |
|-----------|-------------|
| Ready | Aggregate condition indicating if the VDI instance is fully ready for use |
| Available | Indicates if the VDI instance is accessible for connections |
| Progressing | Indicates active changes being applied to the VDI instance |
| Degraded | Indicates the VDI instance is in a non-functional state |

Additionally, component-specific conditions provide more detailed status:

| Condition | Description |
|-----------|-------------|
| VMReady | Status of the underlying KubeVirt VM |
| StreamingReady | Status of the Sunshine streaming components |
| NetworkReady | Status of the networking resources (services, routes, ingress) |
| RBACReady | Status of RBAC permissions and role bindings |

## Checking Conditions

You can check the status of conditions using kubectl:

```bash
kubectl get vdiinstance <name> -o jsonpath='{.status.conditions}'
```

For a more readable format:

```bash
kubectl get vdiinstance <name> -o jsonpath='{range .status.conditions[*]}{.type}{"\t"}{.status}{"\t"}{.reason}{"\t"}{.message}{"\n"}{end}'
```

## Troubleshooting with Conditions

Conditions help pinpoint issues with VDI instances:

1. If `Ready` is `False`, check the specific component conditions to identify the issue.
2. If `VMReady` is `False`, ensure the KubeVirt VM exists and is running.
3. If `StreamingReady` is `False`, check the Sunshine deployment logs.
4. If `NetworkReady` is `False`, verify service and ingress/route configurations.
5. If `RBACReady` is `False`, check role bindings and permissions.

The `message` field of each condition provides more detailed information about the current status.
