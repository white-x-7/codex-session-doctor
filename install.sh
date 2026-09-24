#!/usr/bin/env bash
# 把 codex-session-doctor 编译并安装到本地 PATH。
#
# 用法：
#   bash install.sh               # 安装到 ~/.local/bin
#   bash install.sh --prefix DIR  # 安装到 DIR/bin
set -euo pipefail

BIN_NAME="codex-session-doctor"
PREFIX="${HOME}/.local"

while [ $# -gt 0 ]; do
    case "$1" in
        --prefix)
            if [ $# -lt 2 ]; then
                echo "错误：--prefix 需要一个目录参数" >&2
                exit 2
            fi
            PREFIX="$2"
            shift 2
            ;;
        -h|--help)
            sed -n '2,6p' "$0" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
        *)
            echo "错误：未知参数 $1" >&2
            exit 2
            ;;
    esac
done

if ! command -v go >/dev/null 2>&1; then
    echo "错误：没有找到 go 命令，请先安装 Go 1.25 或更高版本。" >&2
    exit 1
fi

REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "${REPO_DIR}"

echo "正在编译 ${BIN_NAME} ..."
go build -o "${BIN_NAME}" ./cmd/codex-session-doctor

mkdir -p "${PREFIX}/bin"
install -m 0755 "${BIN_NAME}" "${PREFIX}/bin/${BIN_NAME}"
echo "已安装到 ${PREFIX}/bin/${BIN_NAME}"

case ":${PATH}:" in
    *":${PREFIX}/bin:"*) ;;
    *)
        echo
        echo "提示：${PREFIX}/bin 不在 PATH 中，请把它加进去，例如："
        echo "  export PATH=\"${PREFIX}/bin:\$PATH\""
        ;;
esac
