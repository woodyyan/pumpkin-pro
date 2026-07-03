#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RELEASE_DIR="$ROOT_DIR/.release"
MANIFEST_DIR="$RELEASE_DIR/manifests"
TMP_DIR="$RELEASE_DIR/tmp"
CONFIG_FILE="$ROOT_DIR/ops/local/release.config.sh"

# shellcheck disable=SC1090
source "$CONFIG_FILE"

SERVICES=("${RELEASE_SUPPORTED_SERVICES[@]}")
DEFAULT_IMAGE_REGISTRY="${IMAGE_REGISTRY:-ccr.ccs.tencentyun.com}"
DEFAULT_IMAGE_NAMESPACE="${IMAGE_NAMESPACE:-pumpkin-pro}"
DEFAULT_IMAGE_PLATFORM="${IMAGE_PLATFORM:-linux/amd64}"
DEFAULT_TCR_USERNAME="${TCR_USERNAME:-}"
DEFAULT_TCR_PASSWORD="${TCR_PASSWORD:-}"
DEFAULT_TAG_PREFIX="${RELEASE_TAG_PREFIX:-release}"
DEFAULT_SHORT_SHA="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo nogit)"
DEFAULT_TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
DEFAULT_RELEASE_TAG="${DEFAULT_TAG_PREFIX}-${DEFAULT_TIMESTAMP}-${DEFAULT_SHORT_SHA}"
DEFAULT_BUILDER="${RELEASE_BUILDER:-$RELEASE_DEFAULT_BUILDER}"

TAG=""
SERVICES_ARG=""
MODE="build_push"
DRY_RUN=0
PARALLEL=1
PUSH_LATEST=0
VERIFY_LOCAL_IMAGE=1
BUILD_BASE_MODE="auto"
BUILDER="$DEFAULT_BUILDER"

usage() {
  cat <<'EOF'
Usage:
  sh ops/local/release.sh --tag <tag> --services backend,frontend
  sh ops/local/release.sh --tag <tag> --all
  sh ops/local/release.sh --services backend --build-only

Options:
  --tag <tag>              Release tag. Default: release-YYYYMMDD-HHMMSS-shortsha
  --services <list>        Comma-separated services: backend,frontend,quant
  --all                    Release all supported services
  --build-only             Build images locally only, do not push to TCR
  --push                   Build and push images to TCR (default)
  --dry-run                Print resolved plan without executing docker commands
  --parallel <n>           Reserved execution concurrency, current phase runs serially
  --builder <name>         Buildx builder to use. Default: release.config.sh -> default
  --build-base             Force building required base images before service images
  --skip-base-build        Skip auto-building base images even if not found locally
  --image-registry <host>  Override IMAGE_REGISTRY
  --image-namespace <ns>   Override IMAGE_NAMESPACE
  --image-platform <plat>  Override IMAGE_PLATFORM
  --tcr-username <name>    Override TCR username
  --tcr-password <secret>  Override TCR password
  --push-latest            Also push :latest for selected services
  --no-verify-local-image  Skip docker image inspect after build
  -h, --help               Show help

Examples:
  sh ops/local/release.sh --services backend --build-only
  sh ops/local/release.sh --build-base --services backend --build-only
  sh ops/local/release.sh --tag release-20260702-213300-1aa251e --services backend,quant
  IMAGE_NAMESPACE=my-team sh ops/local/release.sh --all --push
EOF
}

log() {
  printf '[release] %s\n' "$*"
}

warn() {
  printf '[release][warn] %s\n' "$*" >&2
}

die() {
  printf '[release][error] %s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "Missing required command: $1"
}

join_by() {
  local delimiter="$1"
  shift
  local first=1
  for item in "$@"; do
    if [ "$first" -eq 1 ]; then
      printf '%s' "$item"
      first=0
    else
      printf '%s%s' "$delimiter" "$item"
    fi
  done
}

contains_service() {
  local candidate="$1"
  local service
  for service in "${SERVICES[@]}"; do
    if [ "$service" = "$candidate" ]; then
      return 0
    fi
  done
  return 1
}

validate_tag() {
  local value="$1"
  if [ -z "$value" ]; then
    die "Tag cannot be empty"
  fi
  if ! printf '%s' "$value" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._-]{2,127}$'; then
    die "Invalid tag '$value'. Allowed: letters, digits, dot, underscore, dash; length 3-128"
  fi
}

parse_services() {
  local raw="$1"
  local seen="," token normalized=() old_ifs
  old_ifs="$IFS"
  IFS=','
  read -r -a token <<< "$raw"
  IFS="$old_ifs"

  local item trimmed
  for item in "${token[@]}"; do
    trimmed="$(printf '%s' "$item" | tr -d '[:space:]')"
    [ -z "$trimmed" ] && continue
    contains_service "$trimmed" || die "Unsupported service: $trimmed"
    case "$seen" in
      *",$trimmed,"*) ;;
      *)
        normalized+=("$trimmed")
        seen="${seen}${trimmed},"
        ;;
    esac
  done

  [ "${#normalized[@]}" -gt 0 ] || die "No valid services resolved"
  SELECTED_SERVICES=("${normalized[@]}")
}

resolve_service_repo() {
  local service_name="$1"
  local repo_name
  repo_name="$(release_service_repo "$service_name")" || die "Unsupported service repo: $service_name"
  printf '%s/%s/%s' "$IMAGE_REGISTRY" "$IMAGE_NAMESPACE" "$repo_name"
}

resolve_service_context() {
  local service_name="$1"
  local relative_path
  relative_path="$(release_service_context "$service_name")" || die "Unsupported service context: $service_name"
  printf '%s/%s' "$ROOT_DIR" "$relative_path"
}

resolve_service_dockerfile() {
  local service_name="$1"
  local relative_path
  relative_path="$(release_service_dockerfile "$service_name")" || die "Unsupported service dockerfile: $service_name"
  printf '%s/%s' "$ROOT_DIR" "$relative_path"
}

resolve_base_repo() {
  local service_name="$1"
  local repo_name
  repo_name="$(release_base_repo "$service_name")" || die "Unsupported base repo: $service_name"
  printf '%s/%s/%s' "$IMAGE_REGISTRY" "$IMAGE_NAMESPACE" "$repo_name"
}

resolve_base_context() {
  local service_name="$1"
  local relative_path
  relative_path="$(release_base_context "$service_name")" || die "Unsupported base context: $service_name"
  printf '%s/%s' "$ROOT_DIR" "$relative_path"
}

resolve_base_dockerfile() {
  local service_name="$1"
  local relative_path
  relative_path="$(release_base_dockerfile "$service_name")" || die "Unsupported base dockerfile: $service_name"
  printf '%s/%s' "$ROOT_DIR" "$relative_path"
}

resolve_base_tag() {
  local service_name="$1"
  release_base_tag "$service_name" || die "Unsupported base tag: $service_name"
}

ensure_dirs() {
  mkdir -p "$MANIFEST_DIR" "$TMP_DIR"
}

ensure_builder_selected() {
  if [ "$DRY_RUN" -eq 1 ]; then
    log "[dry-run] validating builder ${BUILDER}"
    return 0
  fi
  docker buildx inspect "$BUILDER" >/dev/null 2>&1 || die "Buildx builder not found: $BUILDER"
}

docker_login_if_needed() {
  [ "$MODE" = "build_push" ] || return 0
  if [ -n "$TCR_USERNAME" ] && [ -n "$TCR_PASSWORD" ]; then
    log "Logging in to $IMAGE_REGISTRY"
    if [ "$DRY_RUN" -eq 1 ]; then
      log "[dry-run] docker login $IMAGE_REGISTRY -u $TCR_USERNAME --password-stdin"
      return 0
    fi
    printf '%s' "$TCR_PASSWORD" | docker login "$IMAGE_REGISTRY" -u "$TCR_USERNAME" --password-stdin >/dev/null
    return 0
  fi

  if docker info >/dev/null 2>&1; then
    warn "TCR credentials not provided explicitly; assuming existing docker login session"
    return 0
  fi

  die "TCR credentials missing. Set TCR_USERNAME/TCR_PASSWORD or pass --tcr-username/--tcr-password"
}

write_manifest_header() {
  local manifest_path="$1"
  local services_json=""
  local service
  for service in "${SELECTED_SERVICES[@]}"; do
    if [ -n "$services_json" ]; then
      services_json+=" ,"
    fi
    services_json+="\"${service}\""
  done

  cat > "$manifest_path" <<EOF
{
  "release_id": "${TAG}",
  "tag": "${TAG}",
  "mode": "${MODE}",
  "builder": "${BUILDER}",
  "build_base_mode": "${BUILD_BASE_MODE}",
  "created_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "git": {
    "branch": "$(git -C "$ROOT_DIR" branch --show-current 2>/dev/null || echo unknown)",
    "commit": "$(git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null || echo unknown)",
    "short_sha": "$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo unknown)"
  },
  "image_registry": "${IMAGE_REGISTRY}",
  "image_namespace": "${IMAGE_NAMESPACE}",
  "image_platform": "${IMAGE_PLATFORM}",
  "requested_services": [${services_json}],
  "services": [
EOF
}

append_manifest_service() {
  local manifest_path="$1"
  local comma="$2"
  local service="$3"
  local repo="$4"
  local image_ref="$5"
  local latest_ref="$6"
  local pushed_latest="$7"
  local dockerfile="$8"
  local context_dir="$9"
  local build_status="${10}"
  local push_status="${11}"
  local image_id="${12}"

  cat >> "$manifest_path" <<EOF
${comma}    {
      "name": "${service}",
      "repo": "${repo}",
      "tag": "${TAG}",
      "image_ref": "${image_ref}",
      "latest_ref": "${latest_ref}",
      "pushed_latest": ${pushed_latest},
      "dockerfile": "${dockerfile}",
      "context": "${context_dir}",
      "build_status": "${build_status}",
      "push_status": "${push_status}",
      "image_id": "${image_id}"
    }
EOF
}

finalize_manifest() {
  local manifest_path="$1"
  cat >> "$manifest_path" <<EOF
  ]
}
EOF
}

base_image_local_exists() {
  local image_ref="$1"
  docker image inspect "$image_ref" >/dev/null 2>&1
}

build_base_image() {
  local service="$1"
  local base_repo base_tag base_ref base_dockerfile base_context
  base_repo="$(resolve_base_repo "$service")"
  base_tag="$(resolve_base_tag "$service")"
  base_ref="${base_repo}:${base_tag}"
  base_dockerfile="$(resolve_base_dockerfile "$service")"
  base_context="$(resolve_base_context "$service")"

  log "Building base image for ${service} -> ${base_ref}"
  local build_cmd=(docker buildx build --builder "$BUILDER" --load --platform "$IMAGE_PLATFORM" -f "$base_dockerfile" -t "$base_ref" "$base_context")

  if [ "$DRY_RUN" -eq 1 ]; then
    printf '[release][dry-run]'
    printf ' %q' "${build_cmd[@]}"
    printf '\n'
    return 0
  fi

  "${build_cmd[@]}"
}

ensure_required_base_images() {
  local service base_repo base_tag base_ref
  for service in "${SELECTED_SERVICES[@]}"; do
    if [ "$service" != "backend" ]; then
      continue
    fi

    base_repo="$(resolve_base_repo "$service")"
    base_tag="$(resolve_base_tag "$service")"
    base_ref="${base_repo}:${base_tag}"

    case "$BUILD_BASE_MODE" in
      always)
        build_base_image "$service"
        ;;
      skip)
        if [ "$DRY_RUN" -eq 1 ]; then
          log "[dry-run] skipping base image auto-build for ${service} (${base_ref})"
        elif ! base_image_local_exists "$base_ref"; then
          warn "Base image ${base_ref} not found locally; backend build may pull or fail depending on registry access"
        fi
        ;;
      auto)
        if [ "$DRY_RUN" -eq 1 ]; then
          log "[dry-run] auto-check base image ${base_ref}"
          continue
        fi
        if ! base_image_local_exists "$base_ref"; then
          build_base_image "$service"
        else
          log "Reusing local base image ${base_ref}"
        fi
        ;;
      *)
        die "Unsupported base build mode: $BUILD_BASE_MODE"
        ;;
    esac
  done
}

build_service() {
  local service="$1"
  local repo image_ref latest_ref dockerfile context_dir
  repo="$(resolve_service_repo "$service")"
  image_ref="${repo}:${TAG}"
  latest_ref="${repo}:latest"
  dockerfile="$(resolve_service_dockerfile "$service")"
  context_dir="$(resolve_service_context "$service")"

  log "Building ${service} -> ${image_ref}"

  local build_cmd=(docker buildx build --builder "$BUILDER" --load --platform "$IMAGE_PLATFORM" -f "$dockerfile" -t "$image_ref")
  if [ "$service" = "backend" ]; then
    build_cmd+=(--build-arg "BACKEND_BASE_IMAGE=$(resolve_base_repo backend):$(resolve_base_tag backend)")
  fi
  if [ "$PUSH_LATEST" -eq 1 ]; then
    build_cmd+=(-t "$latest_ref")
  fi
  build_cmd+=("$context_dir")

  if [ "$DRY_RUN" -eq 1 ]; then
    printf '[release][dry-run]'
    printf ' %q' "${build_cmd[@]}"
    printf '\n'
  else
    "${build_cmd[@]}"
  fi

  local image_id="skipped"
  if [ "$VERIFY_LOCAL_IMAGE" -eq 1 ]; then
    if [ "$DRY_RUN" -eq 1 ]; then
      image_id="dry-run"
      log "[dry-run] docker image inspect ${image_ref}"
    else
      image_id="$(docker image inspect "$image_ref" --format '{{.Id}}')"
      [ -n "$image_id" ] || die "docker image inspect returned empty id for $image_ref"
    fi
  fi

  local push_status="skipped"
  if [ "$MODE" = "build_push" ]; then
    push_status="pushed"
    if [ "$DRY_RUN" -eq 1 ]; then
      log "[dry-run] docker push ${image_ref}"
      if [ "$PUSH_LATEST" -eq 1 ]; then
        log "[dry-run] docker push ${latest_ref}"
      fi
    else
      docker push "$image_ref"
      if [ "$PUSH_LATEST" -eq 1 ]; then
        docker push "$latest_ref"
      fi
    fi
  fi

  LAST_SERVICE_REPO="$repo"
  LAST_SERVICE_IMAGE_REF="$image_ref"
  LAST_SERVICE_LATEST_REF="$latest_ref"
  LAST_SERVICE_BUILD_STATUS="built"
  LAST_SERVICE_PUSH_STATUS="$push_status"
  LAST_SERVICE_IMAGE_ID="$image_id"
  LAST_SERVICE_CONTEXT="$context_dir"
  LAST_SERVICE_DOCKERFILE="$dockerfile"
}

print_summary() {
  local manifest_path="$1"
  log "Release manifest written to ${manifest_path}"
  log "Tag: ${TAG}"
  log "Mode: ${MODE}"
  log "Builder: ${BUILDER}"
  log "Base image mode: ${BUILD_BASE_MODE}"
  log "Services: $(join_by ',' "${SELECTED_SERVICES[@]}")"
}

IMAGE_REGISTRY="$DEFAULT_IMAGE_REGISTRY"
IMAGE_NAMESPACE="$DEFAULT_IMAGE_NAMESPACE"
IMAGE_PLATFORM="$DEFAULT_IMAGE_PLATFORM"
TCR_USERNAME="$DEFAULT_TCR_USERNAME"
TCR_PASSWORD="$DEFAULT_TCR_PASSWORD"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --tag)
      [ "$#" -ge 2 ] || die "--tag requires a value"
      TAG="$2"
      shift 2
      ;;
    --services)
      [ "$#" -ge 2 ] || die "--services requires a value"
      SERVICES_ARG="$2"
      shift 2
      ;;
    --all)
      SERVICES_ARG="$(join_by ',' "${SERVICES[@]}")"
      shift
      ;;
    --build-only)
      MODE="build_only"
      shift
      ;;
    --push)
      MODE="build_push"
      shift
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    --parallel)
      [ "$#" -ge 2 ] || die "--parallel requires a value"
      PARALLEL="$2"
      shift 2
      ;;
    --builder)
      [ "$#" -ge 2 ] || die "--builder requires a value"
      BUILDER="$2"
      shift 2
      ;;
    --build-base)
      BUILD_BASE_MODE="always"
      shift
      ;;
    --skip-base-build)
      BUILD_BASE_MODE="skip"
      shift
      ;;
    --image-registry)
      [ "$#" -ge 2 ] || die "--image-registry requires a value"
      IMAGE_REGISTRY="$2"
      shift 2
      ;;
    --image-namespace)
      [ "$#" -ge 2 ] || die "--image-namespace requires a value"
      IMAGE_NAMESPACE="$2"
      shift 2
      ;;
    --image-platform)
      [ "$#" -ge 2 ] || die "--image-platform requires a value"
      IMAGE_PLATFORM="$2"
      shift 2
      ;;
    --tcr-username)
      [ "$#" -ge 2 ] || die "--tcr-username requires a value"
      TCR_USERNAME="$2"
      shift 2
      ;;
    --tcr-password)
      [ "$#" -ge 2 ] || die "--tcr-password requires a value"
      TCR_PASSWORD="$2"
      shift 2
      ;;
    --push-latest)
      PUSH_LATEST=1
      shift
      ;;
    --no-verify-local-image)
      VERIFY_LOCAL_IMAGE=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "Unknown argument: $1"
      ;;
  esac
done

require_command docker
require_command git

[ -n "$TAG" ] || TAG="$DEFAULT_RELEASE_TAG"
validate_tag "$TAG"
[ -n "$SERVICES_ARG" ] || die "Choose --services <list> or --all"
parse_services "$SERVICES_ARG"

if ! printf '%s' "$PARALLEL" | grep -Eq '^[1-9][0-9]*$'; then
  die "--parallel must be a positive integer"
fi
if [ "$PARALLEL" -ne 1 ]; then
  warn "Phase 2 executes serially; --parallel=${PARALLEL} is accepted but not used yet"
fi

ensure_dirs
manifest_path="$MANIFEST_DIR/${TAG}.json"
[ ! -f "$manifest_path" ] || die "Manifest already exists: $manifest_path"

log "Resolved release plan"
log "  registry : $IMAGE_REGISTRY"
log "  namespace: $IMAGE_NAMESPACE"
log "  platform : $IMAGE_PLATFORM"
log "  builder  : $BUILDER"
log "  tag      : $TAG"
log "  mode     : $MODE"
log "  base     : $BUILD_BASE_MODE"
log "  services : $(join_by ',' "${SELECTED_SERVICES[@]}")"
log "  manifest : $manifest_path"

if [ "$DRY_RUN" -eq 1 ]; then
  log "Dry-run mode enabled"
fi

ensure_builder_selected
docker_login_if_needed
ensure_required_base_images
write_manifest_header "$manifest_path"

comma=""
for service in "${SELECTED_SERVICES[@]}"; do
  build_service "$service"
  append_manifest_service \
    "$manifest_path" \
    "$comma" \
    "$service" \
    "$LAST_SERVICE_REPO" \
    "$LAST_SERVICE_IMAGE_REF" \
    "$LAST_SERVICE_LATEST_REF" \
    "$([ "$PUSH_LATEST" -eq 1 ] && echo true || echo false)" \
    "$LAST_SERVICE_DOCKERFILE" \
    "$LAST_SERVICE_CONTEXT" \
    "$LAST_SERVICE_BUILD_STATUS" \
    "$LAST_SERVICE_PUSH_STATUS" \
    "$LAST_SERVICE_IMAGE_ID"
  comma=","
done

finalize_manifest "$manifest_path"
print_summary "$manifest_path"
