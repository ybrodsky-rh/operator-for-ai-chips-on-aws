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

## Test Case 10: vLLM Inference Workload with DRA

**Objective:** Verify that a full vLLM inference workload can deploy, load a model, and serve requests using DRA-allocated Neuron devices (replacing the device-plugin `aws.amazon.com/neuron` resource model).

### Prerequisites

- HuggingFace token with access to `meta-llama/Llama-3.1-8B-Instruct`
- `gp3-csi` StorageClass available for model cache PVC
- Red Hat registry pull secret (for `registry.redhat.io/rhaiis/vllm-neuron-rhel9:3`)

### Steps

1. Create the namespace and HuggingFace token secret:

```bash
oc create namespace neuron-inference
oc create secret generic hf-token -n neuron-inference \
  --from-literal=HF_TOKEN=<your-hf-token>
```

2. Create the model cache PVC:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: model-cache
  namespace: neuron-inference
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 50Gi
  storageClassName: gp3-csi
```

3. Create the ResourceClaimTemplate for DRA device allocation:

```yaml
apiVersion: resource.k8s.io/v1
kind: ResourceClaimTemplate
metadata:
  name: neuron-vllm-claim
  namespace: neuron-inference
spec:
  spec:
    devices:
      requests:
      - name: neuron
        firstAvailable:
        - name: neuron-request
          deviceClassName: neuron.aws.com
          count: 1
```

4. Create the vLLM Deployment (DRA mode — no `schedulerName`, no `aws.amazon.com/neuron` resources):

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: neuron-vllm
  namespace: neuron-inference
  labels:
    app: neuron-vllm
spec:
  replicas: 1
  selector:
    matchLabels:
      app: neuron-vllm
  template:
    metadata:
      labels:
        app: neuron-vllm
    spec:
      volumes:
        - name: model-volume
          persistentVolumeClaim:
            claimName: model-cache
        - name: shm
          emptyDir:
            medium: Memory
            sizeLimit: "2Gi"
      initContainers:
        - name: fetch-model
          image: python:3.11-slim
          env:
            - name: HF_HOME
              value: /model
            - name: HF_TOKEN
              valueFrom:
                secretKeyRef:
                  name: hf-token
                  key: HF_TOKEN
          command: ["/bin/sh","-c"]
          args:
            - |
             set -ex
             if [ ! -f "/model/config.json" ]; then
              export PYTHONUSERBASE="/tmp/pip"
              export PATH="$PYTHONUSERBASE/bin:$PATH"
              pip install --no-cache-dir --user "huggingface_hub>=1.0"
              $PYTHONUSERBASE/bin/hf download meta-llama/Llama-3.1-8B-Instruct --local-dir /model
             else
              echo "Model already present, skipping pull"
             fi
          volumeMounts:
            - name: model-volume
              mountPath: /model
      containers:
        - name: vllm-neuron
          image: registry.redhat.io/rhaiis/vllm-neuron-rhel9:3
          imagePullPolicy: IfNotPresent
          workingDir: /model
          env:
            - name: NEURON_CACHE_URL
              value: "/model/neuron_cache"
          command:
            - python
            - '-m'
            - vllm.entrypoints.openai.api_server
          args:
            - '--port=8000'
            - '--model=/model'
            - '--served-model-name=meta-llama/Llama-3.1-8B-Instruct'
            - '--tensor-parallel-size=2'
            - '--max-num-seqs=4'
            - '--max-model-len=4096'
            - '--block-size=16'
            - '--no-enable-prefix-caching'
            - '--no-enable-chunked-prefill'
          livenessProbe:
            httpGet:
              path: /health
              port: 8000
            initialDelaySeconds: 900
            periodSeconds: 10
            failureThreshold: 3
          readinessProbe:
            httpGet:
              path: /health
              port: 8000
            initialDelaySeconds: 900
            periodSeconds: 10
            failureThreshold: 3
          resources:
            limits:
              memory: "100Gi"
            requests:
              memory: "10Gi"
            claims:
            - name: neuron-device
          volumeMounts:
            - name: model-volume
              mountPath: /model
            - name: shm
              mountPath: /dev/shm
      resourceClaims:
      - name: neuron-device
        resourceClaimTemplateName: neuron-vllm-claim
      restartPolicy: Always
```

5. Create the Service and Route:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: neuron-vllm
  namespace: neuron-inference
spec:
  selector:
    app: neuron-vllm
  ports:
    - name: vllm-port
      protocol: TCP
      port: 80
      targetPort: 8000
---
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: neuron-vllm
  namespace: neuron-inference
spec:
  to:
    kind: Service
    name: neuron-vllm
  port:
    targetPort: vllm-port
  tls:
    termination: edge
    insecureEdgeTerminationPolicy: Redirect
```

6. Wait for model download and Neuron compilation (15-30 min on first run):

```bash
oc logs -n neuron-inference -l app=neuron-vllm -c fetch-model -f
oc logs -n neuron-inference -l app=neuron-vllm -c vllm-neuron -f
```

7. Verify the ResourceClaim was allocated via DRA:

```bash
oc get resourceclaim -n neuron-inference -o wide
```

8. Verify pod was scheduled by `default-scheduler` (not `neuron-scheduler`):

```bash
oc get events -n neuron-inference --field-selector reason=Scheduled
```

9. Test the vLLM models endpoint:

```bash
oc port-forward -n neuron-inference deployment/neuron-vllm 8000:8000 &
curl -s http://localhost:8000/v1/models | python3 -m json.tool
```

10. Test chat completion inference:

```bash
curl -s http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "meta-llama/Llama-3.1-8B-Instruct",
    "messages": [{"role": "user", "content": "What is Dynamic Resource Allocation in Kubernetes? Answer in one sentence."}],
    "max_tokens": 100,
    "temperature": 0.7
  }' | python3 -m json.tool
```

11. Test text completion inference:

```bash
curl -s http://localhost:8000/v1/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "meta-llama/Llama-3.1-8B-Instruct",
    "prompt": "OpenShift is",
    "max_tokens": 50,
    "temperature": 0.5
  }' | python3 -m json.tool
```

12. Cleanup:

```bash
oc delete namespace neuron-inference
```

### Key DRA Differences from Device-Plugin Mode

| Aspect | Device-Plugin Mode | DRA Mode |
|--------|-------------------|----------|
| Scheduler | `schedulerName: neuron-scheduler` | Default scheduler |
| Device request | `resources.limits.aws.amazon.com/neuron: 1` | `resourceClaims` + `ResourceClaimTemplate` |
| Device allocation | Device plugin API (integer count) | ResourceClaim (structured parameters) |
| Device class | N/A | `neuron.aws.com` DeviceClass |

### Expected Results

- ResourceClaimTemplate `neuron-vllm-claim` created
- ResourceClaim shows `allocated,reserved` state with `neuron.aws.com` driver
- Pod scheduled by `default-scheduler` to a Neuron node
- Init container downloads model successfully
- vLLM starts, compiles model for Neuron, and serves on port 8000
- `/v1/models` returns `meta-llama/Llama-3.1-8B-Instruct`
- `/v1/chat/completions` returns valid chat response
- `/v1/completions` returns valid text completion
- No `schedulerName` used — DRA replaces custom scheduler need

---

## Test Results Summary

| Test Case | Description | Result |
|-----------|-------------|--------|
| TC-1 | DRA DeviceConfig Creation | PASS |
| TC-2 | DRA Pod Spec Verification | PASS |
| TC-3 | Custom Scheduler Not Deployed | PASS |
| TC-4 | DRA Device Allocation via ResourceClaim | PASS |
| TC-5 | Custom DeviceClasses | PASS |
| TC-6 | Revert to Default DeviceClass | PASS |
| TC-7 | DeviceConfig Deletion Cascade | PASS |
| TC-8 | Re-create After Deletion | PASS |
| TC-9 | Error-Free Operation | PASS |
| TC-10 | vLLM Inference Workload with DRA | PASS |

## Test Run Results (2026-08-13)

### TC-10 vLLM DRA Workload — Observed Output

**Pod status:**
```
NAME                           READY   STATUS    RESTARTS   AGE
neuron-vllm-5d48f6b54c-l8gzf   1/1     Running   0          166m
```

**ResourceClaim (DRA allocation):**
```
NAME                                               STATE                AGE
neuron-vllm-5d48f6b54c-l8gzf-neuron-device-48zxp   allocated,reserved   166m

Driver: neuron.aws.com
Pool:   ip-10-0-2-31.us-west-2.compute.internal
```

**Scheduler event (default-scheduler, no custom neuron-scheduler):**
```
Normal  Scheduled  default-scheduler  Successfully assigned neuron-inference/neuron-vllm-5d48f6b54c-l8gzf to ip-10-0-2-31.us-west-2.compute.internal
```

**GET /v1/models:**
```json
{
    "object": "list",
    "data": [
        {
            "id": "meta-llama/Llama-3.1-8B-Instruct",
            "object": "model",
            "owned_by": "vllm",
            "root": "/model",
            "max_model_len": 4096
        }
    ]
}
```

**POST /v1/chat/completions:**
```json
{
    "id": "chatcmpl-632d79bdf6764a6dae6eceffcfb09180",
    "model": "meta-llama/Llama-3.1-8B-Instruct",
    "choices": [
        {
            "message": {
                "role": "assistant",
                "content": "Dynamic Resource Allocation in Kubernetes is a feature that automatically adjusts the resources (such as CPU and memory) allocated to pods based on their actual needs, ensuring efficient use of cluster resources and minimizing waste."
            },
            "finish_reason": "stop"
        }
    ],
    "usage": {
        "prompt_tokens": 48,
        "completion_tokens": 40,
        "total_tokens": 88
    }
}
```

**POST /v1/completions:**
```json
{
    "id": "cmpl-44bfe67321564a7b83ec0ed050da7866",
    "model": "meta-llama/Llama-3.1-8B-Instruct",
    "choices": [
        {
            "text": " a container application platform that provides a scalable and secure way to deploy and manage applications. It is built on top of Kubernetes and provides a lot of additional features and tools to make it easier to deploy and manage applications. In this tutorial, we will go",
            "finish_reason": "length"
        }
    ],
    "usage": {
        "prompt_tokens": 4,
        "completion_tokens": 50,
        "total_tokens": 54
    }
}
```

## Environment

- **Cluster:** OpenShift 4.22 / Kubernetes 1.35 (ramat-aviv)
- **KMM Version:** 2.7.0
- **Operator Image:** `quay.io/rh-ee-ybrodsky/neuron-operator:kmm27-dra`
- **Node Type:** trn1.32xlarge (16 Neuron devices per node)
- **NFD Version:** 4.22
- **Nodes:** `ip-10-0-1-198.us-west-2.compute.internal`, `ip-10-0-2-31.us-west-2.compute.internal`
- **vLLM Image:** `registry.redhat.io/rhaiis/vllm-neuron-rhel9:3`
- **Model:** `meta-llama/Llama-3.1-8B-Instruct` (tensor-parallel-size=2, max-model-len=4096)
