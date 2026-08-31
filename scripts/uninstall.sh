#!/bin/sh
# Remove the NovelForge executable while preserving all user configuration and projects.
set -e

BIN="novelforge"
DEST="${NOVELFORGE_INSTALL_DIR:-${AINOVEL_INSTALL_DIR:-/usr/local/bin}}"
TARGET="$DEST/$BIN"
DRY_RUN=false

usage() {
	cat <<'USAGE'
Usage: uninstall.sh [--dry-run]

Removes only the NovelForge executable. The following are always preserved:
  ~/.novelforge
  ~/.ainovel
  project .novelforge / .ainovel directories

Set NOVELFORGE_INSTALL_DIR to uninstall from a custom binary directory.
USAGE
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--dry-run) DRY_RUN=true ;;
		-h|--help) usage; exit 0 ;;
		*) echo "未知参数：$1" >&2; usage >&2; exit 2 ;;
	esac
	shift
done

if [ ! -e "$TARGET" ] && [ ! -L "$TARGET" ]; then
	echo "NovelForge 未安装在 $TARGET；无需操作"
	exit 0
fi
if [ -d "$TARGET" ] && [ ! -L "$TARGET" ]; then
	echo "拒绝删除目录：$TARGET" >&2
	exit 1
fi
if [ "$DRY_RUN" = true ]; then
	echo "将删除：$TARGET"
	echo "配置与项目数据将保留"
	exit 0
fi

if [ -w "$DEST" ]; then
	rm -f "$TARGET"
else
	command -v sudo >/dev/null 2>&1 || {
		echo "没有权限删除 $TARGET，且系统未安装 sudo" >&2
		exit 1
	}
	echo "需要管理员权限删除 $TARGET"
	sudo rm -f "$TARGET"
fi

echo "✓ 已卸载：$TARGET"
echo "配置与项目数据已保留；如需清理，请人工确认后处理 ~/.novelforge 或 ~/.ainovel"
