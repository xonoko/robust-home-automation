#!/usr/bin/env bash
# Removes the development deployment from Docker Desktop's Kubernetes.
# Data volumes are kept unless --purge is given.
set -euo pipefail

CONTEXT=docker-desktop
NAMESPACE=home-hub
kubectl=(kubectl --context "$CONTEXT")
helm=(helm --kube-context "$CONTEXT")

if ! kubectl config get-contexts -o name | grep -qx "$CONTEXT"; then
  echo "The '$CONTEXT' context does not exist." >&2
  exit 1
fi

"${helm[@]}" uninstall hub --namespace "$NAMESPACE" --ignore-not-found
"${helm[@]}" uninstall nats --namespace "$NAMESPACE" --ignore-not-found

if [ "${1:-}" = "--purge" ]; then
  "${kubectl[@]}" delete namespace "$NAMESPACE" --ignore-not-found
  "${helm[@]}" uninstall cnpg --namespace cnpg-system --ignore-not-found
fi
