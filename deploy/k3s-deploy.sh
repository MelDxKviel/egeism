#!/usr/bin/env bash
# Run on the existing production k3s node with its local kubeconfig.
set -euo pipefail
umask 077

revision=${1:?tested commit SHA required}
chart=${2:?chart directory required}
[[ "$revision" =~ ^[0-9a-f]{40}$ ]] || { echo "Invalid revision"; exit 1; }
export KUBECONFIG=${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}
namespace=egeism
release=egeism
k3s kubectl -n "$namespace" get deployment api web >/dev/null

# The node did not previously have Helm. Use a pinned, verified temporary
# binary; do not change the system package manager or the cluster credentials.
work=$(mktemp -d /tmp/egeism-helm.XXXXXX)
trap 'rm -rf -- "$work"' EXIT
curl --fail --silent --show-error --location --retry 3 \
  https://get.helm.sh/helm-v3.19.0-linux-amd64.tar.gz -o "$work/helm.tar.gz"
echo "a7f81ce08007091b86d8bd696eb4d86b8d0f2e1b9f6c714be62f82f96a594496  $work/helm.tar.gz" | sha256sum -c -
tar -xzf "$work/helm.tar.gz" -C "$work"
helm="$work/linux-amd64/helm"
"$helm" status "$release" -n "$namespace" >/dev/null

# Keep a local database backup before the additive migrations. No pupil data
# or credentials are printed to the Actions log.
backup_dir=/var/backups/egeism
mkdir -p "$backup_dir"
k3s kubectl -n "$namespace" exec statefulset/postgres -- \
  pg_dump -U egeism -d egeism --format=custom > "$backup_dir/before-$revision.dump"

# Reuse the ACTUAL production values (TLS, storage, secret references, bots).
# Pre-upgrade hooks must complete before any new application pod is created.
if ! "$helm" upgrade "$release" "$chart" -n "$namespace" --reuse-values \
  --set-string global.imageTag="$revision" --atomic --wait-for-jobs --timeout 8m; then
  # Atomic rollback preserves failed hooks. Show their state without dumping
  # Secret/ConfigMap values so image pulls and startup errors are actionable.
  k3s kubectl -n "$namespace" get pods,jobs || true
  k3s kubectl -n "$namespace" get events --sort-by=.lastTimestamp | tail -60 || true
  for component in minio-init migrate; do
    job="$component-${revision:0:8}"
    k3s kubectl -n "$namespace" logs "job/$job" --all-containers --tail=60 --pod-running-timeout=10s || true
  done
  exit 1
fi

for component in api web fetcher worker bot; do
  k3s kubectl -n "$namespace" rollout status "deployment/$component" --timeout=180s
  actual=$(k3s kubectl -n "$namespace" get "deployment/$component" -o "jsonpath={.spec.template.spec.containers[0].image}")
  test "$actual" = "ghcr.io/meldxkviel/egeism-$component:$revision"
done

# Verify both the public frontend and API, not an unused Compose stack.
curl --fail --silent --show-error --retry 5 --retry-all-errors https://egeism.ru/health
curl --fail --silent --show-error https://egeism.ru/version.json | grep -F "\"$revision\""
curl --fail --silent --show-error https://egeism.ru/api/config | grep -F "\"$revision\""
echo "Public k3s release verified: $revision"
