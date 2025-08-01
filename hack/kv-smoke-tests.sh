#!/usr/bin/env bash

set -euxo pipefail

INSTALLED_NAMESPACE=${INSTALLED_NAMESPACE:-"kubevirt-hyperconverged"}
OUTPUT_DIR=${ARTIFACT_DIR:-"$(pwd)/_out"}
SERVICE_ACCOUNT_NAME=${SERVICE_ACCOUNT_NAME:-"kubevirt-testing"}

source hack/common.sh
source cluster/kubevirtci.sh

echo "downloading the test binary"
BIN_DIR="$(pwd)/_out" && mkdir -p "${BIN_DIR}"
export BIN_DIR

TESTS_BINARY="$BIN_DIR/kv_smoke_tests.test"

curl -Lo "$TESTS_BINARY" "https://github.com/kubevirt/kubevirt/releases/download/${KUBEVIRT_VERSION}/tests.test"
chmod +x "$TESTS_BINARY"

echo "create testing infrastructure"

cat <<EOF | ${CMD} apply -f -
apiVersion: v1
kind: PersistentVolume
metadata:
  name: host-path-disk-alpine
  labels:
    kubevirt.io: ""
    os: "alpine"
spec:
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteOnce
  hostPath:
    path: /tmp/hostImages/alpine
EOF

cat <<EOF | ${CMD} apply -f -
apiVersion: v1
kind: PersistentVolume
metadata:
  name: host-path-disk-custom
  labels:
    kubevirt.io: ""
    os: "custom"
spec:
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteOnce
  hostPath:
    path: /tmp/hostImages/custom
EOF

cat <<EOF | ${CMD} apply -f -
apiVersion: v1
kind: Service
metadata:
  name: cdi-http-import-server
  namespace: ${INSTALLED_NAMESPACE}
  labels:
    kubevirt.io: "cdi-http-import-server"
spec:
  ports:
    - port: 80
      targetPort: 80
      protocol: TCP
  selector:
    kubevirt.io: cdi-http-import-server
EOF

cat <<EOF | ${CMD} apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cdi-http-import-server
  namespace: ${INSTALLED_NAMESPACE}
  labels:
    kubevirt.io: "cdi-http-import-server"
spec:
  selector:
    matchLabels:
      kubevirt.io: "cdi-http-import-server"
  replicas: 1
  template:
    metadata:
      labels:
        kubevirt.io: cdi-http-import-server
    spec:
      securityContext:
        runAsUser: 0
      serviceAccountName: ${SERVICE_ACCOUNT_NAME}
      containers:
        - name: cdi-http-import-server
          image: quay.io/kubevirt/cdi-http-import-server:latest
          imagePullPolicy: Always
          ports:
            - containerPort: 80
              name: "http"
              protocol: "TCP"
          readinessProbe:
            tcpSocket:
              port: 80
            initialDelaySeconds: 5
            periodSeconds: 10
EOF

cat <<EOF | ${CMD} apply -f -
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: disks-images-provider
  namespace: ${INSTALLED_NAMESPACE}
  labels:
    kubevirt.io: "disks-images-provider"
spec:
  selector:
    matchLabels:
      kubevirt.io: "disks-images-provider"
  template:
    metadata:
      labels:
        name: disks-images-provider
        kubevirt.io: disks-images-provider
      name: disks-images-provider
    spec:
      tolerations:
        - key: CriticalAddonsOnly
          operator: Exists
      serviceAccountName: ${SERVICE_ACCOUNT_NAME}
      containers:
        - name: target
          image: quay.io/kubevirt/disks-images-provider:${KUBEVIRT_VERSION}
          imagePullPolicy: Always
          env:
          - name: NUM_TEST_IMAGE_REPLICAS
            value: "3"
          volumeMounts:
          - name: images
            mountPath: /hostImages
          - name: local-storage
            mountPath: /local-storage
          - name: host-dir
            mountPath: /host
            mountPropagation: Bidirectional
          securityContext:
            privileged: true
            readOnlyRootFilesystem: false
          readinessProbe:
            exec:
              command:
              - cat
              - /ready
            initialDelaySeconds: 10
            periodSeconds: 5
      volumes:
        - name: images
          hostPath:
            path: /tmp/hostImages
            type: DirectoryOrCreate
        - name: local-storage
          hostPath:
            path: /mnt/local-storage
            type: DirectoryOrCreate
        - name: host-dir
          hostPath:
            path: /
EOF

cat <<EOF | ${CMD} apply -f -
apiVersion: v1
kind: PersistentVolume
metadata:
  name: local-block-storage-cirros
  labels:
    kubevirt.io: ""
    blockstorage: "cirros"
spec:
  accessModes:
  - ReadWriteOnce
  capacity:
    storage: 1Gi
  local:
    path: /mnt/local-storage/cirros-block-device
  nodeAffinity:
    required:
      nodeSelectorTerms:
      - matchExpressions:
        - key: kubernetes.io/hostname
          operator: In
          values:
          - node01
  persistentVolumeReclaimPolicy: Retain
  storageClassName: local-block
  volumeMode: Block
EOF

cat <<EOF | ${CMD} apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ${SERVICE_ACCOUNT_NAME}
  namespace: ${INSTALLED_NAMESPACE}
  labels:
    kubevirt.io: ""
EOF

oc adm policy add-scc-to-user hostaccess -z ${SERVICE_ACCOUNT_NAME} -n ${INSTALLED_NAMESPACE}

cat <<EOF | ${CMD} apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: ${SERVICE_ACCOUNT_NAME}-cluster-admin
  labels:
    kubevirt.io: ""
roleRef:
  kind: ClusterRole
  name: cluster-admin
  apiGroup: rbac.authorization.k8s.io
subjects:
  - kind: ServiceAccount
    name: ${SERVICE_ACCOUNT_NAME}
    namespace: ${INSTALLED_NAMESPACE}
EOF

CLUSTER_KIND=$(${CMD} get infrastructure cluster -o jsonpath='{.status.platformStatus.type}' || true)

if [ "${CLUSTER_KIND}" == "GCP" ]; then
    echo "Configuring the test environment for a GCP cluster"
    CLUSTER_KIND_SUFFIX=".gcp"
elif [ "${CLUSTER_KIND}" == "Azure" ]; then
    echo "Configuring the test environment for an Azure cluster"
    CLUSTER_KIND_SUFFIX=".azure"
else
    echo "Unable to identify cluster kind, consuming a generic test configuration"
    CLUSTER_KIND_SUFFIX=""
fi

${CMD} create configmap -n ${INSTALLED_NAMESPACE} kubevirt-test-config --from-file="hack/test-config.json${CLUSTER_KIND_SUFFIX}" --dry-run=client -o yaml | ${CMD} apply -f -

echo "waiting for testing infrastructure to be ready"
${CMD} wait deployment cdi-http-import-server -n "${INSTALLED_NAMESPACE}" --for condition=Available --timeout=10m
${CMD} wait pods -l "kubevirt.io=disks-images-provider" -n "${INSTALLED_NAMESPACE}" --for condition=Ready --timeout=10m

# TODO: remove once https://github.com/kubevirt/kubevirt/pull/12073 will be merged
# and we will be able to consume a new release with a fix
KVPR12073='(test_id:6867)'

# these failures introduced when bumping to KubeVirt 1.6. We can't run in HCO controlled environment, so
# because they requires FG we don't support in HCO. Skipping them
KVV1_6FAILURES='(when guest crashes)|(rfe_id:151.*IgnitionData)'

echo "starting tests"
${TESTS_BINARY} \
    -cdi-namespace="$INSTALLED_NAMESPACE" \
    -config="hack/test-config.json${CLUSTER_KIND_SUFFIX}" \
    -installed-namespace="$INSTALLED_NAMESPACE" \
    -junit-output="${OUTPUT_DIR}/junit_kv_smoke_tests.xml" \
    -kubeconfig="$KUBECONFIG" \
    -ginkgo.focus='(rfe_id:1177)|(rfe_id:273)|(rfe_id:151)' \
    -ginkgo.no-color \
    -ginkgo.seed=0 \
    -ginkgo.skip="(Slirp Networking)|(with CPU spec)|(with TX offload disabled)|(with cni flannel and ptp plugin interface)|(with ovs-cni plugin)|(test_id:1752)|(SRIOV)|(with EFI)|(Operator)|(GPU)|(DataVolume Integration)|(when virt-handler is not responsive)|(with default cpu model)|(should set the default MachineType when created without explicit value)|(should fail to start when a volume is backed by PVC created by DataVolume instead of the DataVolume itself)|(test_id:3468)|(test_id:3466)|(test_id:1015)|(rfe_id:393)|(test_id:4646)|(test_id:4647)|(test_id:4648)|(test_id:4649)|(test_id:4650)|(test_id:4651)|(test_id:4652)|(test_id:4654)|(test_id:4655)|(test_id:4656)|(test_id:4657)|(test_id:4658)|(test_id:4659)|(test_id:7679)|(should obey the disk verification limits in the KubeVirt CR)|${KVPR12073}|${KVV1_6FAILURES}" \
    -ginkgo.slow-spec-threshold=60s \
    -ginkgo.succinct \
    -ginkgo.flake-attempts=3 \
    -kubectl-path="$(which oc)" \
    -container-tag="${KUBEVIRT_VERSION}" \
    -utility-container-prefix=quay.io/kubevirt \
    -utility-container-tag="${KUBEVIRT_VERSION}" \
    -test.timeout=3h \
    -ginkgo.timeout=3h \
    -ginkgo.label-filter='!software-emulation' \
    -artifacts=${ARTIFACT_DIR}/kubevirt_dump
