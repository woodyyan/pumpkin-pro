#!/usr/bin/env bash
# shellcheck shell=bash

RELEASE_SUPPORTED_SERVICES=(backend frontend quant)
RELEASE_DEFAULT_BUILDER="default"
RELEASE_BACKEND_BASE_SERVICE="backend-base"
RELEASE_BACKEND_BASE_REPO="pumpkin-base"
RELEASE_BACKEND_BASE_TAG="1.0"

release_service_repo() {
  case "$1" in
    backend) printf '%s' 'pumpkin-pro-backend' ;;
    frontend) printf '%s' 'pumpkin-pro-frontend' ;;
    quant) printf '%s' 'pumpkin-pro-quant' ;;
    *) return 1 ;;
  esac
}

release_service_context() {
  case "$1" in
    backend) printf '%s' 'backend' ;;
    frontend) printf '%s' 'frontend' ;;
    quant) printf '%s' 'quant' ;;
    *) return 1 ;;
  esac
}

release_service_dockerfile() {
  case "$1" in
    backend) printf '%s' 'backend/Dockerfile' ;;
    frontend) printf '%s' 'frontend/Dockerfile' ;;
    quant) printf '%s' 'quant/Dockerfile' ;;
    *) return 1 ;;
  esac
}

release_base_repo() {
  case "$1" in
    backend) printf '%s' "$RELEASE_BACKEND_BASE_REPO" ;;
    *) return 1 ;;
  esac
}

release_base_context() {
  case "$1" in
    backend) printf '%s' 'backend' ;;
    *) return 1 ;;
  esac
}

release_base_dockerfile() {
  case "$1" in
    backend) printf '%s' 'backend/Dockerfile.base' ;;
    *) return 1 ;;
  esac
}

release_base_tag() {
  case "$1" in
    backend) printf '%s' "$RELEASE_BACKEND_BASE_TAG" ;;
    *) return 1 ;;
  esac
}
