#!/bin/sh
set -eu

# ============================================================
# BTXL Docker Entrypoint
# ------------------------------------------------------------
# 目标:
# 1. 同时兼容“文件挂载”和“目录挂载”两种配置方式
# 2. 在配置缺失时自动用示例配置初始化
# 3. 避免面板类编排工具将 config.yaml 自动创建为目录后导致容器无法启动
# ============================================================

APP_HOME="/opt/btxl"
APP_BIN="${APP_HOME}/btxl"
EXAMPLE_CONFIG="${APP_HOME}/config.example.yaml"

LEGACY_CONFIG_PATH="${APP_HOME}/config.yaml"
CONFIG_DIR_PATH="${APP_HOME}/config"

AUTH_DIR="/root/.btxl"
LOG_DIR="${APP_HOME}/logs"

resolve_config_file() {
  if [ -d "${LEGACY_CONFIG_PATH}" ]; then
    printf '%s\n' "${LEGACY_CONFIG_PATH}/config.yaml"
    return 0
  fi

  if [ -f "${LEGACY_CONFIG_PATH}" ]; then
    printf '%s\n' "${LEGACY_CONFIG_PATH}"
    return 0
  fi

  if [ -d "${CONFIG_DIR_PATH}" ]; then
    printf '%s\n' "${CONFIG_DIR_PATH}/config.yaml"
    return 0
  fi

  printf '%s\n' "${CONFIG_DIR_PATH}/config.yaml"
}

has_config_arg() {
  prev=""
  for arg in "$@"; do
    if [ "${arg}" = "-config" ] || [ "${prev}" = "-config" ]; then
      return 0
    fi
    prev="${arg}"
  done
  return 1
}

CONFIG_FILE="$(resolve_config_file)"
CONFIG_DIR="$(dirname "${CONFIG_FILE}")"

mkdir -p "${CONFIG_DIR}" "${AUTH_DIR}" "${LOG_DIR}"

if [ -d "${CONFIG_FILE}" ]; then
  echo "BTXL startup error: config file path is a directory: ${CONFIG_FILE}" >&2
  exit 1
fi

if [ ! -f "${CONFIG_FILE}" ]; then
  cp "${EXAMPLE_CONFIG}" "${CONFIG_FILE}"
  echo "BTXL: initialized missing config at ${CONFIG_FILE}" >&2
fi

if has_config_arg "$@"; then
  exec "${APP_BIN}" "$@"
fi

exec "${APP_BIN}" -config "${CONFIG_FILE}" "$@"
