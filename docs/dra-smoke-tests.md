# DRA Smoke Tests for Neuron Operator with KMM 2.7

## Prerequisites

- OpenShift 4.22+ / Kubernetes 1.35+ (DRA requires >= 1.34)
- KMM 2.7 installed (channel: `release-2.7`)
- NFD installed with NodeFeatureDiscovery CR
- Neuron operator deployed from branch `yevgeny/kmm27-dra-migration`
- trn1 or inf2 nodes with Neuron PCI devices

## Test Case 1: DRA DeviceConfig Creation

**Objective:** Verify that creating a DeviceConfig with `draDriverImage` triggers KMM to create a DRA DaemonSet, DeviceClass, and ResourceSlices.

### Steps

1. Create the DeviceConfig:

```yaml
apiVersion: k8s.aws/v1beta1
kind: DeviceConfig
metadata:
  name: neuron
  namespace: aws-neuron-operator
spec:
  driversImage: public.ecr.aws/os-partners/neuron-openshift/neuron-kernel-module:2.28.0.0
  draDriverImage: public.ecr.aws/neuron/neuron-dra-driver:1.0.1
  nodeMetricsImage: public.ecr.aws/neuron/neuron-monitor:1.10.0
  selector:
    feature.node.kubernetes.io/aws-neuron: "true"
```

2. Verify the KMM Module was created with `spec.dra`:

```bash
oc get module neuron -n aws-neuron-operator -o jsonpath='{.spec.dra}' | python3 -m json.tool
```

3. Verify the DRA DaemonSet was created by KMM and pods are running:

```bash
oc get ds -n aws-neuron-operator -l kmm.node.kubernetes.io/role=dra
oc get pods -n aws-neuron-operator -l kmm.node.kubernetes.io/role=dra
```

4. Verify the DeviceClass was created:

```bash
oc get deviceclass neuron.aws.com -o yaml
```

5. Verify ResourceSlices are published on each Neuron node:

```bash
oc get resourceslice
```

6. Verify Module status shows DRA availability:

```bash
oc get module neuron -n aws-neuron-operator -o jsonpath='{.status.dra}' | python3 -m json.tool
```

### Expected Results

- Module `spec.dra` contains: `driverName: neuron.aws.com`, `container.image: public.ecr.aws/neuron/neuron-dra-driver:1.0.1`, `container.command: [k8s-neuron-dra-driver]`, `serviceAccountName: awslabs-gpu-operator-dra-driver`, default DeviceClass `neuron.aws.com`
- DRA DaemonSet has 1 ready pod per Neuron node
- DeviceClass `neuron.aws.com` exists with CEL selector `device.driver == "neuron.aws.com"` and KMM ownership labels
- ResourceSlices list 16 `neuron-device-*` entries per trn1.32xlarge node
- Module `status.dra.availableNumber` equals the number of Neuron nodes

---

## Test Case 2: DRA Pod Spec Verification

**Objective:** Verify KMM auto-injects the required env vars, volumes, liveness probe, and hostNetwork into DRA driver pods.

### Steps

1. Check DRA pod environment variables:

```bash
oc get pod -n aws-neuron-operator -l kmm.node.kubernetes.io/role=dra \
  -o jsonpath='{range .items[0].spec.containers[0].env[*]}{.name}={.value}{"\n"}{end}'
```

2. Check DRA pod volumes:

```bash
oc get pod -n aws-neuron-operator -l kmm.node.kubernetes.io/role=dra \
  -o jsonpath='{range .items[0].spec.volumes[*]}{.name}{"\n"}{end}'
```

3. Check hostNetwork:

```bash
oc get pod -n aws-neuron-operator -l kmm.node.kubernetes.io/role=dra \
  -o jsonpath='{.items[0].spec.hostNetwork}'
```

4. Check liveness probe:

```bash
oc get pod -n aws-neuron-operator -l kmm.node.kubernetes.io/role=dra \
  -o jsonpath='{.items[0].spec.containers[0].livenessProbe}' | python3 -m json.tool
```

### Expected Results

- Env vars: `NODE_NAME` (from fieldRef), `POD_UID` (from fieldRef), `CDI_ROOT=/var/run/cdi`, `KUBELET_REGISTRAR_DIRECTORY_PATH=/var/lib/kubelet/plugins_registry/`, `KUBELET_PLUGINS_DIRECTORY_PATH=/var/lib/kubelet/plugins/`, `HEALTHCHECK_PORT=51515`
- Volumes: `kubelet-plugins`, `kubelet-plugins-registry`, `cdi`
- `hostNetwork: true`
- GRPC liveness probe on port 51515

---

## Test Case 3: Custom Scheduler Not Deployed in DRA Mode

**Objective:** Verify that the custom scheduler and scheduler extension deployments are NOT created when `draDriverImage` is set.

### Steps

1. List all deployments in the operator namespace:

```bash
oc get deployment -n aws-neuron-operator
```

2. Verify no scheduler-related deployments exist:

```bash
oc get deployment -n aws-neuron-operator | grep -c scheduler
```

### Expected Results

- Only `awslabs-gpu-operator-controller-manager` deployment exists
- No `*-custom-scheduler` or `*-custom-scheduler-extension` deployments
- Zero count from grep

---

## Test Case 4: DRA Device Allocation via ResourceClaim

**Objective:** Verify that a consumer pod can request and receive a Neuron device via DRA ResourceClaim.

### Steps

1. Create a ResourceClaimTemplate and consumer pod:

```yaml
apiVersion: resource.k8s.io/v1
kind: ResourceClaimTemplate
metadata:
  name: neuron-claim-template
  namespace: aws-neuron-operator
spec:
  spec:
    devices:
      requests:
      - name: neuron
        firstAvailable:
        - name: neuron-request
          deviceClassName: neuron.aws.com
          count: 1
---
apiVersion: v1
kind: Pod
metadata:
  name: dra-test-consumer
  namespace: aws-neuron-operator
spec:
  containers:
  - name: test
    image: registry.access.redhat.com/ubi9/ubi-minimal:9.7
    command: ["sh", "-c", "echo 'DRA device allocated successfully' && sleep 60"]
    resources:
      claims:
      - name: neuron-device
  resourceClaims:
  - name: neuron-device
    resourceClaimTemplateName: neuron-claim-template
  restartPolicy: Never
```

2. Verify the pod is Running:

```bash
oc get pod dra-test-consumer -n aws-neuron-operator
```

3. Verify the ResourceClaim was allocated:

```bash
oc get resourceclaim -n aws-neuron-operator
```

4. Verify the pod logs:

```bash
oc logs dra-test-consumer -n aws-neuron-operator
```

5. Cleanup:

```bash
oc delete pod dra-test-consumer -n aws-neuron-operator
oc delete resourceclaimtemplate neuron-claim-template -n aws-neuron-operator
```

### Expected Results

- Pod reaches `Running` state
- ResourceClaim shows `allocated,reserved` state
- Pod logs show: `DRA device allocated successfully`

---

## Test Case 5: Custom DeviceClasses

**Objective:** Verify that specifying custom DeviceClasses in the DeviceConfig propagates to the KMM Module and creates the correct DeviceClass resources.

### Steps

1. Patch the DeviceConfig with custom DeviceClasses:

```bash
oc patch deviceconfig neuron -n aws-neuron-operator --type=merge -p '{
  "spec": {
    "deviceClasses": [
      {
        "name": "neuron-training",
        "selectors": [{"cel": {"expression": "device.driver == \"neuron.aws.com\""}}]
      },
      {
        "name": "neuron-inference",
        "selectors": [{"cel": {"expression": "device.driver == \"neuron.aws.com\""}}]
      }
    ]
  }
}'
```

2. Verify DeviceClasses were created:

```bash
oc get deviceclass
```

3. Verify Module spec was updated:

```bash
oc get module neuron -n aws-neuron-operator \
  -o jsonpath='{range .spec.dra.deviceClasses[*]}{.name}{"\n"}{end}'
```

4. Verify old default DeviceClass was removed:

```bash
oc get deviceclass neuron.aws.com 2>&1
```

### Expected Results

- Two DeviceClasses exist: `neuron-training` and `neuron-inference`
- Module `spec.dra.deviceClasses` lists both custom classes
- Default `neuron.aws.com` DeviceClass no longer exists (KMM converges to declared state)

---

## Test Case 6: Revert to Default DeviceClass

**Objective:** Verify that removing custom DeviceClasses from the DeviceConfig reverts to the default `neuron.aws.com` DeviceClass.

### Steps

1. Remove the `deviceClasses` field:

```bash
oc patch deviceconfig neuron -n aws-neuron-operator --type=json \
  -p='[{"op":"remove","path":"/spec/deviceClasses"}]'
```

2. Wait for convergence and verify:

```bash
sleep 10
oc get deviceclass
oc get module neuron -n aws-neuron-operator \
  -o jsonpath='{range .spec.dra.deviceClasses[*]}{.name}{"\n"}{end}'
```

### Expected Results

- Only `neuron.aws.com` DeviceClass exists
- Module spec contains only the default DeviceClass with CEL selector `device.driver == "neuron.aws.com"`
- Custom DeviceClasses (`neuron-training`, `neuron-inference`) are deleted

---

## Test Case 7: DeviceConfig Deletion and Cascade Cleanup

**Objective:** Verify that deleting the DeviceConfig triggers cascade deletion of all DRA resources through KMM.

### Steps

1. Record pre-deletion state:

```bash
oc get ds -n aws-neuron-operator
oc get deviceclass
oc get module -n aws-neuron-operator
oc get resourceslice
```

2. Delete the DeviceConfig:

```bash
oc delete deviceconfig neuron -n aws-neuron-operator
```

3. Wait and verify all resources are cleaned up:

```bash
sleep 20
oc get ds -n aws-neuron-operator
oc get deviceclass
oc get module -n aws-neuron-operator
oc get resourceslice
oc get pods -n aws-neuron-operator
```

4. Verify no errors in operator logs:

```bash
oc logs deployment/awslabs-gpu-operator-controller-manager -n aws-neuron-operator | grep -i "error"
```

### Expected Results

- All DaemonSets deleted (DRA and node-metrics)
- All DeviceClasses deleted
- KMM Module deleted
- ResourceSlices cleaned up (may take a few extra seconds)
- Only the operator controller-manager pod remains
- No errors in operator logs

---

## Test Case 8: Re-create After Deletion

**Objective:** Verify the operator can create all DRA resources from scratch after a full cleanup.

### Steps

1. Re-apply the DeviceConfig (same YAML as Test Case 1)
2. Verify all resources are created:

```bash
sleep 15
oc get module -n aws-neuron-operator
oc get ds -n aws-neuron-operator
oc get deviceclass
oc get resourceslice
oc get pods -n aws-neuron-operator -l kmm.node.kubernetes.io/role=dra
oc get module neuron -n aws-neuron-operator -o jsonpath='{.status.dra}' | python3 -m json.tool
```

### Expected Results

- All resources recreated identically to Test Case 1
- DRA pods running on all Neuron nodes
- ResourceSlices published
- Module status shows full availability

---

## Test Case 9: Operator Error-Free Operation

**Objective:** Verify the operator produces no errors throughout the entire test lifecycle.

### Steps

1. Check operator logs for any errors:

```bash
oc logs deployment/awslabs-gpu-operator-controller-manager -n aws-neuron-operator | grep -ci "error\|panic"
```

2. Check operator pod restart count:

```bash
oc get pod -n aws-neuron-operator -l app.kubernetes.io/name=aws-neuron \
  -o jsonpath='{.items[0].status.containerStatuses[0].restartCount}'
```

### Expected Results

- Zero error/panic lines in logs
- Zero restarts

---

## Test Results Summary

| Test Case | Description | Result |
|-----------|-------------|--------|
| TC-1 | DRA DeviceConfig Creation | |
| TC-2 | DRA Pod Spec Verification | |
| TC-3 | Custom Scheduler Not Deployed | |
| TC-4 | DRA Device Allocation via ResourceClaim | |
| TC-5 | Custom DeviceClasses | |
| TC-6 | Revert to Default DeviceClass | |
| TC-7 | DeviceConfig Deletion Cascade | |
| TC-8 | Re-create After Deletion | |
| TC-9 | Error-Free Operation | |

## Environment

- **Cluster:** OpenShift 4.22 / Kubernetes 1.35
- **KMM Version:** 2.7.0
- **Operator Image:** `quay.io/rh-ee-ybrodsky/neuron-operator:kmm27-dra`
- **Node Type:** trn1.32xlarge (16 Neuron devices per node)
- **NFD Version:** 4.22
