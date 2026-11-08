#!/usr/bin/env bash
# Deploys the hub, NATS and Postgres to Docker Desktop's built-in Kubernetes.
#
# Prerequisite: Docker Desktop > Settings > Kubernetes > Enable Kubernetes,
# cluster type "kind", 3 nodes.
#
# Safety: every command is pinned to the docker-desktop context; this script
# never touches another cluster, whatever the current kubectl context is.
set -euo pipefail

CONTEXT=docker-desktop
NAMESPACE=home-hub
IMAGE=robust-home-automation/hub:dev
CNPG_CHART_VERSION=0.29.1
NATS_CHART_VERSION=2.15.2

root="$(cd "$(dirname "$0")/../.." && pwd)"
kubectl=(kubectl --context "$CONTEXT")
helm=(helm --kube-context "$CONTEXT")

if ! kubectl config get-contexts -o name | grep -qx "$CONTEXT"; then
  echo "The '$CONTEXT' context does not exist: enable Kubernetes in Docker Desktop (kind, 3 nodes)." >&2
  exit 1
fi

nodes=$("${kubectl[@]}" get nodes --no-headers | wc -l | tr -d ' ')
if [ "$nodes" -lt 3 ]; then
  echo "warning: $nodes node(s) found, 3 expected. Pods will share nodes and HA cannot be tested." >&2
fi

echo "==> Building $IMAGE"
docker build -t "$IMAGE" "$root"

# With the kind provisioner, nodes are containers with their own image store.
# Load the image into each of them when they are visible to the Docker CLI.
kind_nodes=$(docker ps --filter "label=io.x-k8s.kind.role" --format '{{.Names}}' || true)
if [ -n "$kind_nodes" ]; then
  for node in $kind_nodes; do
    echo "==> Loading $IMAGE into $node"
    docker save "$IMAGE" | docker exec -i "$node" ctr --namespace=k8s.io images import -
  done
else
  echo "==> No kind node containers visible; assuming Docker Desktop shares the image store with the cluster"
fi

echo "==> Installing the CloudNativePG operator"
"${helm[@]}" upgrade --install cnpg cloudnative-pg \
  --repo https://cloudnative-pg.github.io/charts --version "$CNPG_CHART_VERSION" \
  --namespace cnpg-system --create-namespace --wait

echo "==> Installing NATS"
"${helm[@]}" upgrade --install nats nats \
  --repo https://nats-io.github.io/k8s/helm/charts/ --version "$NATS_CHART_VERSION" \
  --namespace "$NAMESPACE" --create-namespace \
  -f "$root/deploy/dev/values-nats.yaml" --wait

echo "==> Installing the hub"
"${helm[@]}" upgrade --install hub "$root/deploy/helm/hub" \
  --namespace "$NAMESPACE" \
  -f "$root/deploy/dev/values-hub.yaml" --wait --timeout 10m

# Rebuilding the same :dev tag does not change the pod spec; restart to pick it up.
"${kubectl[@]}" -n "$NAMESPACE" rollout restart daemonset/hub
"${kubectl[@]}" -n "$NAMESPACE" rollout status daemonset/hub --timeout 5m

cat <<EOF

Deployed. Open the UI with:
  kubectl --context $CONTEXT -n $NAMESPACE port-forward svc/hub 8080:8080
  then http://localhost:8080
EOF
