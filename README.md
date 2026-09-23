# DeskuBe - KubeVirt VDI Operator

![DeskuBe Logo](https://raw.githubusercontent.com/deskube/deskube/main/docs/images/logo.png)

> The Virtual Desktop Infrastructure (VDI) operator for Kubernetes and OpenShift

DeskuBe is an OpenShift/Kubernetes operator that provides Virtual Desktop Infrastructure (VDI) capabilities by leveraging [KubeVirt](https://kubevirt.io/) virtual machines and [LizardByte's Sunshine](https://github.com/LizardByte/Sunshine) streaming technology.

## Table of Contents

- [Description](#description)
  - [Architecture](#architecture)
  - [Key Features](#key-features)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Required Components](#required-components)
  - [Quick Start](#quick-start)
  - [To Deploy on the cluster](#to-deploy-on-the-cluster)
  - [To Uninstall](#to-uninstall)
- [VDI Instance Configuration](#vdi-instance-configuration)
  - [KubeVirt VM Reference](#kubevirt-vm-reference)
  - [Sunshine Streaming Configuration](#sunshine-streaming-configuration)
  - [Moonlight Client Configuration](#moonlight-client-configuration)
  - [User Access Control](#user-access-control)
  - [Resource Requirements](#resource-requirements)
  - [Network Configuration](#network-configuration)
- [Accessing VDI Instances](#accessing-vdi-instances)
- [Advanced Usage Scenarios](#advanced-usage-scenarios)
- [Status and Monitoring](#status-and-monitoring)
  - [Kubernetes Conditions](#kubernetes-conditions)
  - [Metrics and Monitoring](#metrics-and-monitoring)
- [Troubleshooting](#troubleshooting)
- [Integration with OpenShift](#integration-with-openshift)
- [RBAC Integration](#rbac-integration)
- [Development Guide](#development-guide)
  - [Project Structure](#project-structure)
  - [Building the Operator Locally](#building-the-operator-locally)
  - [Running Tests](#running-tests)
- [Technical Deep Dive](#technical-deep-dive)
  - [Controller Reconciliation Flow](#controller-reconciliation-flow)
  - [Custom Resource Status](#custom-resource-status)
  - [Resource Ownership and Garbage Collection](#resource-ownership-and-garbage-collection)
  - [Security Considerations](#security-considerations)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [Community](#community)
- [References](#references)
- [Project Distribution](#project-distribution)
- [Additional Contribution Resources](#additional-contribution-resources)
- [License](#license)

## Description

DeskuBe enables cloud-native VDI deployments by seamlessly integrating KubeVirt virtual machines with Sunshine/Moonlight streaming technology. This allows you to:

- Deploy virtual desktops on Kubernetes/OpenShift clusters
- Stream virtual desktop sessions with high performance and low latency
- Manage access control to virtual desktops
- Dynamically provision VDI resources based on demand
- Integrate with existing authentication systems

### Architecture

DeskuBe follows the Kubernetes operator pattern and consists of:

1. **Custom Resource Definitions (CRDs)**:
   - `VdiInstance`: Defines a VDI instance connected to a KubeVirt VM

2. **Controller**:
   - Watches for VdiInstance resources
   - Manages the lifecycle of VDI components
   - Handles integration with KubeVirt VMs
   - Deploys Sunshine streaming servers
   - Configures networking for access

3. **Components**:
   - **KubeVirt Integration**: Connects to existing KubeVirt VMs
   - **Sunshine Streaming**: Handles video/audio streaming from VMs
   - **Moonlight Client**: Optional client configuration for connecting to VDI
   - **Network Management**: Exposes VDI via various methods (Ingress, Route, etc.)

#### Component Architecture Diagram

The following diagram illustrates how DeskuBe components interact with each other and with the Kubernetes/OpenShift cluster:

```ascii
                                 ┌───────────────────┐
                                 │                   │
                   watches       │   VdiInstance     │
              ┌─────────────────►│   Custom Resource │
              │                  │                   │
              │                  └───────────────────┘
              │                           │
              │                           │ references
┌─────────────┴──────────────┐            │
│                            │            │
│    DeskuBe Controller      │            ▼
│                            │    ┌───────────────────┐
│  ┌────────────────────┐    │    │                   │
│  │ Reconciliation     │    │    │   KubeVirt VM     │
│  │ Loop               │    │    │                   │
│  └────────────────────┘    │    └───────────────────┘
│           │                │             ▲
│           │                │             │
│           ▼                │             │ connects to
│  ┌────────────────────┐    │    ┌───────┴───────────┐
│  │ Networking         │    │    │                   │
│  │ Configuration      │────┼───►│  Sunshine Server  │
│  └────────────────────┘    │    │  Deployment       │
│           │                │    │                   │
└───────────┼────────────────┘    └───────────────────┘
            │                               │
            │                               │ streams to
            │                               ▼
            │                     ┌───────────────────┐
            │                     │                   │
            │                     │  Moonlight Client │
            ▼                     │                   │
┌─────────────────────────┐       └───────────────────┘
│                         │                 ▲
│ Kubernetes Services     │                 │
│ Ingress / Route /       │─────────────────┘
│ LoadBalancer            │      connects via
│                         │
└─────────────────────────┘
```

#### Deployment Architecture Diagrams

The following diagrams illustrate different deployment scenarios for DeskuBe.

**Standard OpenShift Deployment:**

```ascii
┌─────────────────────────────────────────────────────────┐
│ OpenShift Cluster                                       │
│                                                         │
│  ┌─────────────────┐      ┌─────────────────────────┐   │
│  │                 │      │                         │   │
│  │  Control Plane  │      │  DeskuBe Operator       │   │
│  │                 │      │  Namespace              │   │
│  └─────────────────┘      │                         │   │
│                           │  ┌───────────────────┐  │   │
│                           │  │ Controller        │  │   │
│                           │  │ Manager           │  │   │
│                           │  └───────────────────┘  │   │
│                           │                         │   │
│                           └─────────────────────────┘   │
│                                        │                │
│                                        ▼                │
│  ┌────────────────────────┐   ┌─────────────────────┐   │
│  │                        │   │                     │   │
│  │  User Workload         │   │  VdiInstance        │   │
│  │  Namespace             │   │  Namespace          │   │
│  │                        │   │                     │   │
│  │  ┌──────────────────┐  │   │  ┌───────────────┐  │   │
│  │  │                  │  │   │  │ KubeVirt VMs  │  │   │
│  │  │ User             │  │   │  │               │  │   │
│  │  │ Applications     │  │   │  └───────────────┘  │   │
│  │  │                  │  │   │         │           │   │
│  │  └──────────────────┘  │   │         ▼           │   │
│  │                        │   │  ┌───────────────┐  │   │
│  └────────────────────────┘   │  │ Sunshine Pods │  │   │
│                               │  │               │  │   │
│                               │  └───────────────┘  │   │
│                               │         │           │   │
│                               │         ▼           │   │
│                               │  ┌───────────────┐  │   │
│                               │  │ OpenShift     │  │   │
│                               │  │ Routes        │  │   │
│                               │  └───────────────┘  │   │
│                               │                     │   │
│                               └─────────────────────┘   │
│                                        │                │
└─────────────────────────────────────────────────────────┘
                                         │
                                         ▼
                                ┌─────────────────┐
                                │                 │
                                │ External Users  │
                                │ with Moonlight  │
                                │ Client          │
                                │                 │
                                └─────────────────┘
```

**Vanilla Kubernetes Deployment:**

```ascii
┌─────────────────────────────────────────────────────────┐
│ Kubernetes Cluster                                      │
│                                                         │
│  ┌─────────────────┐      ┌─────────────────────────┐   │
│  │                 │      │                         │   │
│  │  Control Plane  │      │  DeskuBe Operator       │   │
│  │                 │      │  Namespace              │   │
│  └─────────────────┘      │                         │   │
│                           │  ┌───────────────────┐  │   │
│                           │  │ Controller        │  │   │
│                           │  │ Manager           │  │   │
│                           │  └───────────────────┘  │   │
│                           │                         │   │
│                           └─────────────────────────┘   │
│                                        │                │
│                                        ▼                │
│  ┌────────────────────────┐   ┌─────────────────────┐   │
│  │                        │   │                     │   │
│  │  Application           │   │  VdiInstance        │   │
│  │  Namespace             │   │  Namespace          │   │
│  │                        │   │                     │   │
│  │  ┌──────────────────┐  │   │  ┌───────────────┐  │   │
│  │  │                  │  │   │  │ KubeVirt VMs  │  │   │
│  │  │ Business         │  │   │  │               │  │   │
│  │  │ Applications     │  │   │  └───────────────┘  │   │
│  │  │                  │  │   │         │           │   │
│  │  └──────────────────┘  │   │         ▼           │   │
│  │                        │   │  ┌───────────────┐  │   │
│  └────────────────────────┘   │  │ Sunshine Pods │  │   │
│                               │  │               │  │   │
│                               │  └───────────────┘  │   │
│                               │         │           │   │
│                               │         ▼           │   │
│                               │  ┌───────────────┐  │   │
│                               │  │ Kubernetes    │  │   │
│                               │  │ Ingress       │  │   │
│                               │  └───────────────┘  │   │
│                               │                     │   │
│                               └─────────────────────┘   │
│                                        │                │
└─────────────────────────────────────────────────────────┘
                                         │
                                         ▼
                                ┌─────────────────┐
                                │                 │
                                │ External Users  │
                                │ with Browser or │
                                │ Moonlight       │
                                │                 │
                                └─────────────────┘
```

**Direct Service Exposure (Gaming Optimized):**

```ascii
┌─────────────────────────────────────────────────────────┐
│ Kubernetes/OpenShift Cluster                            │
│                                                         │
│  ┌─────────────────────────┐                            │
│  │                         │                            │
│  │  DeskuBe Controller     │                            │
│  │                         │                            │
│  └─────────────────────────┘                            │
│              │                                          │
│              ▼                                          │
│  ┌─────────────────────────┐                            │
│  │                         │                            │
│  │  Gaming VDI Instance    │                            │
│  │  Namespace              │                            │
│  │                         │                            │
│  │  ┌───────────────────┐  │                            │
│  │  │ Gaming KubeVirt   │  │                            │
│  │  │ VM with GPU       │◄─┼────────────┐               │
│  │  │                   │  │            │               │
│  │  └───────────────────┘  │            │               │
│  │            │            │     Low-latency            │
│  │            ▼            │     connection             │
│  │  ┌───────────────────┐  │            │               │
│  │  │ High Performance  │  │            │               │
│  │  │ Sunshine Server   │  │            │               │
│  │  │                   │  │            │               │
│  │  └───────────────────┘  │            │               │
│  │            │            │            │               │
│  │            ▼            │            │               │
│  │  ┌───────────────────┐  │            │               │
│  │  │ LoadBalancer      │──┼────────────┘               │
│  │  │ Service           │  │                            │
│  │  │                   │  │                            │
│  │  └───────────────────┘  │                            │
│  │                         │                            │
│  └─────────────────────────┘                            │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### Key Features

- **KubeVirt VM Integration**: Connect to existing KubeVirt virtual machines
- **High-Performance Streaming**: Low-latency streaming using Sunshine technology
- **Multiple Access Methods**: Support for direct, ingress, or OpenShift route access
- **User Access Control**: Fine-grained access control for VDI instances
- **Resource Management**: Control computing and network resource allocation
- **Custom Configuration**: Extensive customization of streaming parameters

## Getting Started

### Prerequisites

- go version v1.22.0+
- docker version 17.03+
- kubectl version v1.11.3+
- Access to a Kubernetes v1.11.3+ cluster or OpenShift 4.x+
- KubeVirt v0.54.0+ installed on your cluster
- Operator SDK (for development)

### Required Components

1. **KubeVirt**: Must be installed on your cluster (see [KubeVirt installation guide](https://kubevirt.io/user-guide/#/installation/installation))
   - DeskuBe is compatible with KubeVirt v0.54.0 and higher
   - Requires KubeVirt VM API with `.status.ready` and `.status.printableStatus` fields
   - VM resources must be pre-created and properly configured before being referenced in a VdiInstance
2. **Sunshine and Moonlight**: The operator will deploy these components as needed

## Quick Start

```sh
# Build the operator
cd /home/epheo/dev/desku.be
make build

# Deploy the CRDs
make install

# Deploy the operator
make deploy

# Apply a sample VDI instance
kubectl apply -f config/samples/vdi_v1alpha1_vdiinstance.yaml
```

### To Deploy on the cluster

#### Build and push your image to the location specified by `IMG`

```sh
make docker-build docker-push IMG=<some-registry>/deskube:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

#### Install the CRDs into the cluster

```sh
make install
```

#### Deploy the Manager to the cluster with the image specified by `IMG`

```sh
make deploy IMG=<some-registry>/deskube:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

#### Create VDI instances

You can create VDI instances by applying the provided samples or creating your own custom resources:

```sh
kubectl apply -k config/samples/
```

Alternatively, create a custom VDI instance by defining a YAML file:

```yaml
apiVersion: vdi.desku.be/v1alpha1
kind: VdiInstance
metadata:
  name: desktop-instance
spec:
  # Reference to a KubeVirt VirtualMachine
  kubeVirtVMRef:
    name: windows10-vm  # Name of your existing KubeVirt VM
    namespace: default  # Namespace where the VM is located
  
  # Configuration for Sunshine streaming server
  sunshineConfig:
    image: lizardbyte/sunshine:latest
    fps: 60
    resolution: 1920x1080
    bitrate: 50  # Mbps
    additionalOptions:
      audio: true
  
  # Network configuration
  networkConfig:
    serviceType: ClusterIP
    exposureMethod: Route  # Use 'Ingress' for vanilla Kubernetes
    tls: true
```

Apply your custom VDI instance:

```sh
kubectl apply -f your-vdi-instance.yaml
```

After creation, the operator will:

1. Connect to the referenced KubeVirt VM
2. Deploy the Sunshine streaming server
3. Configure networking for access
4. Update the VDI instance status with connection details

### To Uninstall

**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## VDI Instance Configuration

The `VdiInstance` custom resource allows for extensive configuration options to tailor your VDI deployment to specific needs.

### KubeVirt VM Reference

```yaml
kubeVirtVMRef:
  name: windows10-vm     # Required: Name of the KubeVirt VM
  namespace: default     # Optional: Namespace of the VM (defaults to VdiInstance namespace)
```

### Sunshine Streaming Configuration

```yaml
sunshineConfig:
  image: lizardbyte/sunshine:latest   # Streaming server image
  fps: 60                             # Frames per second for streaming
  resolution: 1920x1080               # Streaming resolution
  bitrate: 50                         # Streaming bitrate in Mbps
  additionalOptions:                  # Optional additional Sunshine configuration
    audio: true
    gamepad_support: true
    min_bitrate: 10
    max_bitrate: 100
```

### Moonlight Client Configuration

```yaml
moonlightConfig:
  image: lizardbyte/moonlight:latest  # Client image
  clientOptions:                      # Client-specific options
    fullscreen: true
    audio_enabled: true
    input_method: all
```

### User Access Control

```yaml
userAccess:
- username: admin                     # Individual user access
  authType: basic
  permissions:
  - read
  - write
- groups:                             # Group-based access
  - developers
  authType: oauth
  permissions:
  - read
```

### Resource Requirements

```yaml
resources:
  sunshine:                           # Resource limits for Sunshine server
    limits:
      memory: 2Gi
      cpu: 2
    requests:
      memory: 1Gi
      cpu: 1
  moonlight:                          # Resource limits for Moonlight client
    limits:
      memory: 1Gi
      cpu: 1
```

### Network Configuration

```yaml
networkConfig:
  serviceType: ClusterIP              # Kubernetes service type
  exposureMethod: Route              # How to expose the VDI (Route, Ingress, direct)
  hostname: vdi.example.com          # Hostname for access
  port: 47989                        # Port for VDI service
  tls: true                          # Whether to enable TLS
```

## Accessing VDI Instances

Once a VDI instance is created and running, you can access it using several methods:

### 1. Getting Connection Information

```sh
kubectl get vdiinstance desktop-instance -o jsonpath='{.status.connectionDetails}'
```

This will provide the access URL, port, and other connection parameters.

### 2. Using Moonlight Client

For the best experience, use the Moonlight client to connect to your VDI instance:

```sh
# Example using Moonlight client (installed locally)
moonlight client --url https://desktop-instance-vdi.apps.example.com --user admin
```

### 3. Web Access

If configured with a public hostname and TLS, you can access the VDI instance directly from a web browser:

```text
https://desktop-instance-vdi.apps.example.com
```

## Advanced Usage Scenarios

### Multi-User VDI Environment

For environments where multiple users need access to different VDI instances:

1. Create a KubeVirt VM template for your desktop OS
2. Create a script to provision new VMs from the template
3. Create VdiInstance resources pointing to each VM
4. Configure user access permissions per VdiInstance

### High-Performance Gaming VDI

For gaming workloads that require maximum performance:

```yaml
apiVersion: vdi.desku.be/v1alpha1
kind: VdiInstance
metadata:
  name: gaming-desktop
spec:
  kubeVirtVMRef:
    name: gaming-vm
  sunshineConfig:
    fps: 120
    resolution: 2560x1440
    bitrate: 100
    additionalOptions:
      min_bitrate: 50
      gamepad_support: true
      hdr: true
  networkConfig:
    serviceType: LoadBalancer  # Direct network path for lowest latency
    exposureMethod: direct
```

### Enterprise Desktop Environment

For enterprise environments with security requirements:

```yaml
apiVersion: vdi.desku.be/v1alpha1
kind: VdiInstance
metadata:
  name: secure-workstation
spec:
  kubeVirtVMRef:
    name: enterprise-desktop
  sunshineConfig:
    fps: 30  # Lower for business applications
    resolution: 1920x1080
  userAccess:
  - groups:
    - accounting
    authType: ldap
    permissions:
    - read
    - write
  networkConfig:
    exposureMethod: Route
    tls: true
    hostname: desktop.internal.company.com
```

## Status and Monitoring

Monitor the status of your VDI instances:

```sh
kubectl get vdiinstances
```

Example output:

```text
NAME                  PHASE     VM-STATUS   CONNECTION-URL
desktop-instance      Running   Ready       https://desktop-instance-vdi.apps.example.com
gaming-desktop        Running   Ready       gaming-desktop-sunshine.default.svc:47989
secure-workstation    Pending   NotReady    N/A
```

Get detailed information about a specific VDI instance:

```sh
kubectl describe vdiinstance desktop-instance
```

### Kubernetes Conditions

DeskuBe implements standard Kubernetes conditions to provide detailed status information about VDI instances:

```sh
kubectl get vdiinstance desktop-instance -o jsonpath='{range .status.conditions[*]}{.type}{"\t"}{.status}{"\t"}{.message}{"\n"}{end}'
```

Example output:

```text
Ready         True    VDI instance is ready for use
Available     True    VDI instance is available for connection
Progressing   False   VDI instance is now ready
VMReady       True    KubeVirt VM is ready
StreamingReady True   Streaming components are ready
NetworkReady  True    Network resources successfully configured
RBACReady     True    RBAC resources successfully configured
```

These conditions help you understand the status of different components of your VDI instance and troubleshoot any issues. See [detailed documentation on conditions](docs/conditions.md).

## Troubleshooting

### Common Issues

1. **VM Not Found**: Ensure the KubeVirt VM exists and is in the correct namespace
2. **Connection Issues**: Check network policies and firewall rules
3. **Performance Problems**: Adjust FPS, resolution, and bitrate settings
4. **Authentication Failures**: Verify user access permissions

### KubeVirt-Specific Troubleshooting

If you encounter issues with KubeVirt VM connectivity:

**VM Status Issues**: Check that the VM is running and in Ready state.

```sh
kubectl get virtualmachine -n <namespace> <vm-name> -o yaml
```

**Network Connectivity**: Verify the VM has valid IP addresses assigned.

```sh
kubectl get virtualmachineinstance -n <namespace> <vm-name> -o jsonpath='{.status.interfaces}'
```

**VM Ready Check**: This should return `true` for a VM that's ready to accept connections.

```sh
kubectl get virtualmachine -n <namespace> <vm-name> -o jsonpath='{.status.ready}'
```

**Enable Debug Logs**: To enable more verbose logging for KubeVirt VM connectivity troubleshooting, set the controller log level to debug when running the operator.

### Logs

Check the operator logs:

```sh
kubectl logs -f deployment/deskube-controller-manager -n deskube-system
```

Check the Sunshine pod logs:

```sh
kubectl logs -f $(kubectl get pods -l app=desktop-instance-sunshine -o name)
```

## Integration with OpenShift

DeskuBe works especially well with OpenShift, offering several advantages:

1. **Route Integration**: Automatically creates OpenShift Routes for external access
2. **Console Integration**: Custom resource views in the OpenShift console
3. **OAuth Integration**: Use OpenShift's built-in authentication system
4. **Operator Lifecycle**: Managed through the OpenShift Operator Lifecycle Manager (OLM)

## RBAC Integration

DeskuBe supports integration with Kubernetes Role-Based Access Control (RBAC) to manage access to VDI instances. This provides a powerful and standardized way to control who can view, connect to, and manage VDI instances.

### RBAC Access Control Configuration

```yaml
accessControl:
  enabled: true                     # Enable RBAC integration
  createRoles: true                 # Automatically create standard roles
  rolePrefix: vdi                   # Prefix for auto-generated roles
  defaultRoleRef:                   # Default role for users without specific roles
    kind: Role
    name: vdi-instance-viewer
```

### User Access with RBAC

```yaml
userAccess:
- type: user                        # User, Group, or ServiceAccount
  name: admin                       # Name of the user
  roleRef:                          # Reference to a Kubernetes Role
    kind: Role                      # Role or ClusterRole
    name: vdi-instance-admin        # Name of the role
- type: group                       # Group of users
  name: developers
  roleRef:
    kind: Role  
    name: vdi-instance-user
- type: serviceaccount              # Service account for automation
  name: cicd-pipeline
  roleRef:
    kind: ClusterRole
    name: vdi-viewer
```

### Standard Roles

When `createRoles` is enabled, DeskuBe creates the following standard roles for each VDI instance:

1. **Viewer Role**: Allows read-only access to view VDI instance details
   - Role name: `<rolePrefix>-<instance-name>-viewer`

2. **User Role**: Allows read access and the ability to connect to the VDI instance
   - Role name: `<rolePrefix>-<instance-name>-user`

3. **Admin Role**: Allows management of the VDI instance configuration
   - Role name: `<rolePrefix>-<instance-name>-admin`

### Custom Roles

You can also create custom roles with specific permissions and reference them in the `roleRef` field:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: vdi-power-user
  namespace: default
rules:
- apiGroups: ["vdi.desku.be"]
  resources: ["vdiinstances"]
  verbs: ["get", "list", "watch", "update"]
- apiGroups: ["vdi.desku.be"]
  resources: ["vdiinstances/status"]
  verbs: ["get"]
```

Then reference this role in your VDI instance:

```yaml
userAccess:
- type: user
  name: power-user
  roleRef:
    kind: Role
    name: vdi-power-user
```

## Development Guide

DeskuBe is built using the [Operator SDK](https://sdk.operatorframework.io/) and follows standard Kubernetes operator development patterns. This section is for developers who want to contribute to or customize DeskuBe.

### Project Structure

```text
├── api/                       # API definitions for CRDs
│   └── v1alpha1/              # v1alpha1 API version
│       ├── vdiinstance_types.go  # VdiInstance CRD definition
│       └── ...
├── config/                    # Kubernetes manifests
│   ├── crd/                   # Custom Resource Definitions
│   ├── rbac/                  # Role-based access control
│   ├── manager/               # Controller manager deployment
│   └── samples/               # Sample CR instances
├── internal/                  # Internal code packages
│   └── controller/            # Controller implementation
│       └── vdiinstance_controller.go  # Main reconciliation logic
└── ...
```

### Building the Operator Locally

1. Clone the repository:

   ```sh
   git clone https://github.com/deskube/deskube.git
   cd deskube
   ```

2. Install required dependencies:

   ```sh
   go mod download
   ```

3. Build the manager binary:

   ```sh
   make build
   ```

4. Run the controller locally (outside the cluster):

   ```sh
   make run
   ```

### Running Tests

Run unit tests:

```sh
make test
```

Run integration tests (requires a running Kubernetes cluster):

```sh
make test-integration
```

## Technical Deep Dive

### Controller Reconciliation Flow

The VdiInstance controller follows this reconciliation process:

1. **VM Verification**:
   - Retrieves the referenced KubeVirt VM
   - Verifies its status and availability

2. **Sunshine Deployment**:
   - Creates a Deployment running the Sunshine streaming server
   - Configures it based on VdiInstance spec settings
   - Creates necessary ConfigMaps for Sunshine configuration

3. **Network Configuration**:
   - Creates Services to expose the Sunshine server
   - Based on `exposureMethod`, creates one of:
     - OpenShift Route
     - Kubernetes Ingress
     - Direct Service exposure

4. **Status Updates**:
   - Updates VdiInstance status with current phase
   - Records connection details for users

### Custom Resource Status

The `VdiInstanceStatus` contains:

- `phase`: Current lifecycle phase (Pending, Running, Failed, etc.)
- `vmStatus`: Status of the underlying KubeVirt VM
- `sunshineStatus`: Status of the Sunshine streaming server
- `moonlightStatus`: Status of the Moonlight client (if used)
- `connectionDetails`: Information needed to connect to the VDI
- `conditions`: Standard Kubernetes conditions

### Resource Ownership and Garbage Collection

All resources created by the operator (Deployments, Services, etc.) have owner references set to the corresponding VdiInstance CR. This ensures proper garbage collection when a VdiInstance is deleted.

### Security Considerations

1. **Authentication**: The operator supports multiple authentication methods through the `userAccess` configuration
2. **Network Security**: TLS can be enabled for secure connections
3. **Resource Isolation**: Each VDI instance runs in its own Pod with defined resource limits
4. **RBAC**: The operator uses the principle of least privilege for its permissions

## Roadmap

Future development plans for DeskuBe:

- [ ] Support for GPU passthrough to VDI instances
- [ ] Integration with additional authentication providers
- [ ] Enhanced multi-user session management
- [ ] Metrics collection and dashboard integration
- [ ] Auto-scaling based on usage patterns
- [ ] Improved VM template management

## Contributing

Contributions to DeskuBe are welcome! Here's how you can contribute:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

Before submitting a PR, please ensure:

- Code follows the project's style guidelines
- All tests pass
- New functionality includes appropriate tests
- Documentation is updated

## Community

- **GitHub Discussions**: Ask questions and propose ideas
- **Issue Tracker**: Report bugs or request features
- **Slack Channel**: Join our community at #deskube on Kubernetes Slack

## KubeVirt Integration Details

DeskuBe integrates with KubeVirt to provide VDI capabilities on top of virtual machines running in Kubernetes. This section explains the technical details of this integration.

### KubeVirt Version Compatibility

- **Minimum Version**: KubeVirt v0.54.0+
- **API Dependencies**: DeskuBe relies on the following KubeVirt API features:
  - VirtualMachine custom resource with `.status.ready` field
  - `.status.printableStatus` field for VM state detection
  - Standard VM lifecycle management (start/stop/etc.)

### VM State Handling

DeskuBe monitors the following VM states and takes appropriate actions:

| VM Status | VDI Behavior |
|-----------|-------------|
| Ready     | Allows connections, reports as "Running" |
| Starting  | Waits for VM to become ready, reports as "Pending" |
| Running (not ready) | Waits for VM to become ready, reports as "Pending" |
| Stopped   | Reports VDI as "Stopped", no connections allowed |
| Error     | Reports error in status, attempts recovery |

### VM Connectivity Requirements

For successful connectivity between the Sunshine streaming server and the KubeVirt VM:

1. **Network Configuration**: The VM must be on a network that allows the Sunshine pod to connect to it
2. **Firewall Settings**: The VM's firewall must allow incoming connections from the Sunshine pod
3. **Guest Tools**: For optimal performance, install any required guest tools in the VM

### VM Preparation Guidelines

Before creating a VDI instance, prepare your KubeVirt VM as follows:

1. Install and configure the OS with appropriate graphics drivers
2. Install any necessary tools for remote desktop connections
3. Configure automatic login if needed
4. Pre-install any required applications
5. Create the VM with sufficient resources (CPU, memory, disk)

For more details on VM configuration, refer to the [KubeVirt documentation](https://kubevirt.io/docs/).

## References

- [KubeVirt Documentation](https://kubevirt.io/docs/)
- [Sunshine Streaming](https://github.com/LizardByte/Sunshine)
- [Moonlight Client](https://github.com/LizardByte/Moonlight)
- [Operator SDK](https://sdk.operatorframework.io/)
- [Kubernetes Operators](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)
- [OpenShift Routes](https://docs.openshift.com/container-platform/latest/networking/routes/route-configuration.html)

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

### Build the installer for the image built and published in the registry

```sh
make build-installer IMG=<some-registry>/deskube:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

### Using the installer

Users can just run `kubectl apply -f <URL for YAML BUNDLE>` to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/deskube/<tag or branch>/dist/install.yaml
```

## Additional Contribution Resources

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Licensed under the Apache License, Version 2.0 (the "License");
