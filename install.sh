#!/usr/bin/env bash
set -euo pipefail

REPO="${REPO:-steiler/loganalyzer}"
BINARY_NAME="loganalyzer"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
API_URL="https://api.github.com/repos/${REPO}/releases/latest"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Error: required command '$1' not found" >&2
    exit 1
  fi
}

need_cmd curl
need_cmd uname
need_cmd mktemp

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "${os}" in
  linux)
    os="linux"
    ;;
  darwin)
    echo "Error: no macOS release artifacts are configured yet for ${REPO}." >&2
    exit 1
    ;;
  *)
    echo "Error: unsupported OS '${os}'" >&2
    exit 1
    ;;
esac

case "${arch}" in
  x86_64|amd64)
    arch="amd64"
    ;;
  aarch64|arm64)
    arch="arm64"
    ;;
  *)
    echo "Error: unsupported architecture '${arch}'" >&2
    exit 1
    ;;
esac

latest_json="$(curl -fsSL "${API_URL}")"
tag="$(printf '%s\n' "${latest_json}" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"

if [[ -z "${tag}" ]]; then
  echo "Error: could not determine latest release tag for ${REPO}." >&2
  echo "Hint: publish your first release before using this installer." >&2
  exit 1
fi

asset_url="$(printf '%s\n' "${latest_json}" \
  | grep -Eo '"browser_download_url"[[:space:]]*:[[:space:]]*"[^"]+"' \
  | cut -d '"' -f4 \
  | grep -E "/${BINARY_NAME}_[^/]*_${os}_${arch}\\.(tar\\.gz|zip)$" \
  | head -n1)"

if [[ -z "${asset_url}" ]]; then
  echo "Error: no release asset found for ${os}/${arch} in ${REPO} ${tag}." >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "${tmp_dir}"
}
trap cleanup EXIT
asset_file="$(basename "${asset_url}")"

curl -fL "${asset_url}" -o "${tmp_dir}/${asset_file}"

extracted_bin="${tmp_dir}/${BINARY_NAME}"

case "${asset_file}" in
  *.tar.gz)
    tar -xzf "${tmp_dir}/${asset_file}" -C "${tmp_dir}"
    ;;
  *.zip)
    need_cmd unzip
    unzip -q "${tmp_dir}/${asset_file}" -d "${tmp_dir}"
    ;;
  *)
    echo "Error: unsupported archive format '${asset_file}'" >&2
    exit 1
    ;;
esac

if [[ ! -f "${extracted_bin}" ]]; then
  found_bin="$(find "${tmp_dir}" -type f -name "${BINARY_NAME}" | head -n1 || true)"
  if [[ -z "${found_bin}" ]]; then
    echo "Error: '${BINARY_NAME}' binary was not found in downloaded archive." >&2
    exit 1
  fi
  extracted_bin="${found_bin}"
fi

if [[ -w "${INSTALL_DIR}" ]]; then
  install -m 0755 "${extracted_bin}" "${INSTALL_DIR}/${BINARY_NAME}"
else
  if ! command -v sudo >/dev/null 2>&1; then
    echo "Error: write permission denied for ${INSTALL_DIR} and 'sudo' is unavailable." >&2
    exit 1
  fi
  sudo install -m 0755 "${extracted_bin}" "${INSTALL_DIR}/${BINARY_NAME}"
fi

echo "Installed ${BINARY_NAME} ${tag} to ${INSTALL_DIR}/${BINARY_NAME}"
echo "Run: ${BINARY_NAME} -file <path-to-log-file>"
